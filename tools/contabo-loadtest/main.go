// Command loadtest is a careful, phased load generator for the Aequitas
// WAL fastpath, meant to run directly on a node (default target
// localhost:8080) rather than over the public internet -- built for
// Contabo2, SCALING_ARCHITECTURE.md's real-hardware validation gap.
//
// Three phases, run in order via -phase:
//
//  1. fund   -- a small number of seed accounts (funded by a human, real
//     AEQ) distribute a small amount to every test account via ordinary
//     eth_sendRawTransaction calls, one at a time, no concurrency. Not a
//     stress test -- this phase exists purely to get value into disposable
//     test accounts without needing the operator to fund hundreds of
//     addresses individually.
//  2. warmup -- one transfer per test pair, sequential, so every account is
//     "warm" in cs.accounts before the timed phase starts. Matches this
//     project's own sandbox benchmark methodology (see
//     TestSimulateMaxTPS_WarmSteadyState's own comment, x/humanity/keeper):
//     a cold first touch always falls to the slower batcher path, which
//     would understate the fastpath's real number.
//  3. run    -- the actual timed load: each pair's own goroutine sends
//     back-and-forth transfers continuously. Ramps concurrency up over the
//     first -ramp seconds instead of starting at full concurrency
//     immediately, and polls /api/status throughout -- if height stops
//     advancing for 15s, the run aborts early rather than continuing to
//     hammer a node that stopped producing blocks.
//
// Account private keys are read from a CSV (index,address,privkey_hex,
// header row required) that is NEVER committed to this repository --
// generate it with the accompanying gen_accounts.go, keep it out of git,
// and delete it from any server it's copied to once a test run is done.
// This binary itself never prints a private key, only addresses and tx
// hashes.
//
// Every phase logs progress and a per-phase failure count; fund and
// warmup abort the remaining phases on ANY failure rather than continuing
// into a state that was never verified.
package main

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const chainID = 1926 // matches evm_rpc.go's own eth_chainId ("0x786")

type account struct {
	index   int
	address string // lowercase, 0x-prefixed
	priv    *ecdsa.PrivateKey
	nonce   uint64 // next nonce to use, refreshed from chain before each phase
}

func loadAccounts(path string) []*account {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	must(err)
	var accs []*account
	for _, row := range rows[1:] { // skip header
		privBytes, err := hex.DecodeString(row[2])
		must(err)
		priv, err := crypto.ToECDSA(privBytes)
		must(err)
		addr := strings.ToLower(crypto.PubkeyToAddress(priv.PublicKey).Hex())
		var idx int
		fmt.Sscanf(row[0], "%d", &idx)
		accs = append(accs, &account{index: idx, address: addr, priv: priv})
	}
	sort.Slice(accs, func(i, j int) bool { return accs[i].index < accs[j].index })
	return accs
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// ---- JSON-RPC client -------------------------------------------------

type rpcClient struct {
	url string
	hc  *http.Client
}

type rpcReq struct {
	Jsonrpc string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type rpcResp struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *rpcClient) call(method string, params interface{}) (json.RawMessage, error) {
	body, _ := json.Marshal(rpcReq{Jsonrpc: "2.0", Method: method, Params: params, ID: 1})
	req, _ := http.NewRequest("POST", c.url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var rr rpcResp
	if err := json.Unmarshal(b, &rr); err != nil {
		return nil, fmt.Errorf("bad response %q: %w", string(b), err)
	}
	if rr.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rr.Error.Code, rr.Error.Message)
	}
	return rr.Result, nil
}

func (c *rpcClient) nonce(addr string) uint64 {
	res, err := c.call("eth_getTransactionCount", []string{addr, "latest"})
	must(err)
	var hexStr string
	must(json.Unmarshal(res, &hexStr))
	var n uint64
	fmt.Sscanf(strings.TrimPrefix(hexStr, "0x"), "%x", &n)
	return n
}

func (c *rpcClient) sendValue(from *account, toAddr string, amountWei *big.Int) (string, error) {
	tx := types.NewTransaction(from.nonce, addrFromHex(toAddr), amountWei, 21000, big.NewInt(0), nil)
	signer := types.NewEIP155Signer(big.NewInt(chainID))
	signedTx, err := types.SignTx(tx, signer, from.priv)
	if err != nil {
		return "", err
	}
	from.nonce++
	raw, err := signedTx.MarshalBinary()
	if err != nil {
		return "", err
	}
	rawHex := "0x" + hex.EncodeToString(raw)
	res, err := c.call("eth_sendRawTransaction", []string{rawHex})
	if err != nil {
		return "", err
	}
	var txHash string
	json.Unmarshal(res, &txHash)
	return txHash, nil
}

// addrFromHex returns a plain [20]byte, assignable to go-ethereum's
// common.Address (defined as exactly that underlying type) without an
// explicit conversion, per Go's assignability rules for unnamed types.
func addrFromHex(s string) (a [20]byte) {
	b, _ := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	copy(a[:], b)
	return a
}

// ---- status polling for safety ---------------------------------------

type statusResp struct {
	Height int64 `json:"height"`
}

// pollStatus watches /api/status during the timed run phase and signals
// abort if height stops advancing for 15s -- a real node that has stopped
// producing blocks should never be hammered with more load while whoever
// is watching this run figures out why.
func pollStatus(c *rpcClient, statusURL string, stopCh <-chan struct{}, abort chan<- string) {
	lastHeight := int64(-1)
	stallSince := time.Time{}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			resp, err := http.Get(statusURL)
			if err != nil {
				fmt.Printf("[monitor] /api/status unreachable: %v\n", err)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var st statusResp
			if json.Unmarshal(b, &st) != nil {
				continue
			}
			if st.Height == lastHeight {
				if stallSince.IsZero() {
					stallSince = time.Now()
				} else if time.Since(stallSince) > 15*time.Second {
					select {
					case abort <- fmt.Sprintf("height stalled at %d for >15s", st.Height):
					default:
					}
					return
				}
			} else {
				stallSince = time.Time{}
			}
			fmt.Printf("[monitor] height=%d\n", st.Height)
			lastHeight = st.Height
		}
	}
}

// ---- main ---------------------------------------------------------------

func main() {
	rpcURL := flag.String("rpc", "http://localhost:8080/rpc", "EVM RPC endpoint")
	statusURL := flag.String("status", "http://localhost:8080/api/status", "status endpoint")
	csvPath := flag.String("accounts", "accounts.csv", "account CSV path")
	phase := flag.String("phase", "fund,warmup,run", "comma-separated phases to run")
	numSeeds := flag.Int("seeds", 5, "number of seed accounts (first N rows)")
	fundAmount := flag.String("fund-amount-wei", "1000000000000000", "wei sent from a seed to each test account (default 0.001 AEQ)")
	transferAmount := flag.String("transfer-amount-wei", "10000000000000", "wei per load-test transfer (default 0.00001 AEQ)")
	runDuration := flag.Duration("duration", 20*time.Second, "timed load phase duration")
	rampSeconds := flag.Int("ramp", 5, "seconds to ramp concurrency up over")
	flag.Parse()

	accs := loadAccounts(*csvPath)
	if len(accs) <= *numSeeds {
		panic("not enough accounts for the requested seed count")
	}
	seeds := accs[:*numSeeds]
	testAccs := accs[*numSeeds:]
	numPairs := len(testAccs) / 2
	senders := testAccs[:numPairs]
	recipients := testAccs[numPairs : 2*numPairs]

	client := &rpcClient{url: *rpcURL, hc: &http.Client{Timeout: 10 * time.Second}}
	fundWei, ok := new(big.Int).SetString(*fundAmount, 10)
	if !ok {
		panic("bad fund-amount-wei")
	}
	transferWei, ok := new(big.Int).SetString(*transferAmount, 10)
	if !ok {
		panic("bad transfer-amount-wei")
	}

	phases := strings.Split(*phase, ",")
	runPhase := func(name string) bool {
		for _, p := range phases {
			if strings.TrimSpace(p) == name {
				return true
			}
		}
		return false
	}

	if runPhase("fund") {
		fmt.Printf("=== PHASE fund: %d seeds -> %d test accounts, %s wei each ===\n", len(seeds), len(testAccs), fundWei)
		for i, s := range seeds {
			s.nonce = client.nonce(s.address)
			fmt.Printf("seed %d (%s) starting nonce=%d\n", i, s.address, s.nonce)
		}
		var failed int
		for i, ta := range testAccs {
			seed := seeds[i%len(seeds)]
			hash, err := client.sendValue(seed, ta.address, fundWei)
			if err != nil {
				failed++
				fmt.Printf("fund #%d (%s) FAILED: %v\n", i, ta.address, err)
				continue
			}
			if i%20 == 0 {
				fmt.Printf("fund #%d/%d -> %s tx=%s\n", i, len(testAccs), ta.address, hash)
			}
			time.Sleep(20 * time.Millisecond) // gentle pace, this is not the stress test
		}
		fmt.Printf("=== fund phase done: %d failed of %d ===\n", failed, len(testAccs))
		if failed > 0 {
			fmt.Println("ABORTING remaining phases due to funding failures -- inspect before retrying.")
			return
		}
		fmt.Println("Waiting 15s for funding transfers to settle before continuing...")
		time.Sleep(15 * time.Second)
	}

	if runPhase("warmup") {
		fmt.Printf("=== PHASE warmup: %d pairs, one transfer each ===\n", numPairs)
		for _, s := range senders {
			s.nonce = client.nonce(s.address)
		}
		var failed int
		for i := 0; i < numPairs; i++ {
			_, err := client.sendValue(senders[i], recipients[i].address, transferWei)
			if err != nil {
				failed++
				fmt.Printf("warmup pair %d FAILED: %v\n", i, err)
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Printf("=== warmup phase done: %d failed of %d pairs ===\n", failed, numPairs)
		if failed > 0 {
			fmt.Println("ABORTING run phase due to warmup failures -- inspect before retrying.")
			return
		}
		fmt.Println("Waiting 5s before the timed run...")
		time.Sleep(5 * time.Second)
	}

	if runPhase("run") {
		fmt.Printf("=== PHASE run: %d pairs, ramping over %ds, then %s timed ===\n", numPairs, *rampSeconds, *runDuration)
		for _, s := range senders {
			s.nonce = client.nonce(s.address)
		}

		var succeeded, failed int64
		stopCh := make(chan struct{})
		abortCh := make(chan string, 1)
		go pollStatus(client, *statusURL, stopCh, abortCh)

		var wg sync.WaitGroup
		start := time.Now()
		rampDur := time.Duration(*rampSeconds) * time.Second
		for i := 0; i < numPairs; i++ {
			wg.Add(1)
			go func(pairIdx int) {
				defer wg.Done()
				// Ramp: this pair doesn't start sending until its own slice of
				// the ramp window has elapsed, spreading pair start times evenly
				// across rampSeconds instead of all numPairs goroutines hammering
				// the RPC from t=0.
				delay := time.Duration(float64(pairIdx) / float64(numPairs) * float64(rampDur))
				select {
				case <-time.After(delay):
				case <-stopCh:
					return
				}
				from, to := senders[pairIdx], recipients[pairIdx]
				for {
					select {
					case <-stopCh:
						return
					default:
					}
					if _, err := client.sendValue(from, to.address, transferWei); err != nil {
						atomic.AddInt64(&failed, 1)
						continue
					}
					atomic.AddInt64(&succeeded, 1)
				}
			}(i)
		}

		select {
		case reason := <-abortCh:
			fmt.Printf("!!! ABORT: %s !!!\n", reason)
		case <-time.After(rampDur + *runDuration):
		}
		close(stopCh)
		wg.Wait()
		elapsed := time.Since(start)

		fmt.Printf("=== run phase done ===\n")
		fmt.Printf("elapsed: %s  succeeded: %d  failed: %d\n", elapsed, succeeded, failed)
		fmt.Printf("TPS (succeeded/elapsed): %.1f\n", float64(succeeded)/elapsed.Seconds())
	}
}
