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
	// Die erste Zeile nur ueberspringen, wenn sie WIRKLICH eine Kopfzeile ist.
	// Vorher stand hier rows[1:] ohne Pruefung -- und accounts-funded.csv hat
	// keine Kopfzeile, das erste Konto verschwand also bei jedem Lauf still.
	datenAb := 0
	if len(rows) > 0 && len(rows[0]) > 2 {
		if _, err := hex.DecodeString(rows[0][2]); err != nil {
			datenAb = 1 // dritte Spalte ist kein Schluessel -> Kopfzeile
		}
	}
	for _, row := range rows[datenAb:] {
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

// nonceMaxAttempts / nonceRetryBackoff bound the retry loop in nonce below.
//
// FIX (2026-07-24 — this killed a whole run in its first second): nonce used
// to call must(err) on the RPC result, i.e. panic on ANY error. The run phase
// ramps every pair up at once and each one fetches its nonce first, so that
// burst reliably trips the node's own per-IP RPC rate limiter
// (rpcRateLimited/evm_rpc.go, "-32005: rate limited: too many requests, try
// again shortly"). A single such response — a deliberately TRANSIENT,
// retry-after-a-moment signal — took down the entire process:
//
//	=== fund phase done:   0 failed of 145 ===
//	=== warmup phase done: 0 failed of 72 pairs ===
//	=== PHASE run: 72 pairs, ramping over 5s, then 20s timed ===
//	panic: rpc error -32005: rate limited: too many requests, try again shortly
//	  main.(*rpcClient).nonce(...) main.go:142
//
// Funding and warmup had just proven the chain accepts transfers fine; the
// measurement never started. The run-phase SENDS already got exactly this
// treatment (see sendRetryBackoff and the run loop's own comment); the nonce
// fetch that precedes them was simply missed.
//
// Retries are bounded rather than infinite so a genuinely down node still
// fails fast and loudly instead of hanging the workflow until its timeout,
// and the backoff grows so a rate limiter gets a real chance to drain rather
// than being hammered at a fixed interval. A non-transient error (bad
// address, malformed response) is not worth retrying, but distinguishing it
// costs more than it saves here: the attempt cap already bounds the damage,
// and the final error is reported verbatim.
const (
	nonceMaxAttempts  = 8
	nonceRetryBackoff = 100 * time.Millisecond
)

// tryNonce reads addr's next expected nonce without ever aborting the process.
//
// This is what the RUN phase needs. nonce() below calls must() when it finally
// gives up, which is right for setup (a node that cannot answer at all should
// fail the workflow loudly) and catastrophic inside the timed loop, where 72
// goroutines are mid-measurement and a panic takes the whole run with it.
//
// eth_getTransactionCount returns s.nonces[addr] — the very same in-memory
// counter sendRawTransaction compares against (evm_rpc.go: storedNonce). So
// this reads exactly what the node will expect next, with no pending-vs-latest
// discrepancy to reason about, which is what makes a mid-run resync sound.
// lastNonceErr carries WHY the most recent tryNonce failed. Without it the
// retry loop reports "giving up after 8 attempts" and nothing about the cause,
// so a rate limit, a transport timeout and a malformed reply all look alike.
var lastNonceErr error

// balanceWei liest den Kontostand eines Kontos.
func (c *rpcClient) balanceWei(addr string) (*big.Int, bool) {
	res, err := c.call("eth_getBalance", []string{addr, "latest"})
	if err != nil {
		return nil, false
	}
	var hexStr string
	if err := json.Unmarshal(res, &hexStr); err != nil {
		return nil, false
	}
	n, ok := new(big.Int).SetString(strings.TrimPrefix(hexStr, "0x"), 16)
	return n, ok
}

// aussortieren entfernt Konten, die die geplanten Ueberweisungen nicht
// bezahlen koennen.
//
// WARUM. Ein leerer Absender laesst nicht nur seine eigene Ueberweisung
// scheitern, sondern das ganze JSON-RPC-Buendel, in dem er sitzt. Am
// 29.08.2026 waren 118 von 623 Konten leer; der Lauf meldete 20.400
// Fehlschlaege neben 28.500 Erfolgen, und die daraus errechneten 299 TPS
// sagten ueber die Kette nichts aus. Das Werkzeug beschreibt diese Falle in
// seinem eigenen Kommentar zur ring-Topologie ("this measured the CSV\'s
// wealth distribution, not the topology") und zog daraus keine Folgerung.
//
// Eine fehlgeschlagene Abfrage sortiert NICHT aus: sonst wuerde ein
// Netzwackler den Lauf schrumpfen lassen, und das saehe aus wie ein Ergebnis.
func aussortieren(c *rpcClient, accs []*account, mindest *big.Int) []*account {
	if mindest == nil || mindest.Sign() <= 0 {
		return accs
	}
	type ergebnis struct {
		a  *account
		ok bool
	}
	aus := make(chan ergebnis, len(accs))
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	for _, a := range accs {
		wg.Add(1)
		go func(a *account) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			b, ok := c.balanceWei(a.address)
			aus <- ergebnis{a, !ok || b.Cmp(mindest) >= 0}
		}(a)
	}
	wg.Wait()
	close(aus)
	behalten := make([]*account, 0, len(accs))
	verworfen := 0
	for e := range aus {
		if e.ok {
			behalten = append(behalten, e.a)
		} else {
			verworfen++
		}
	}
	sort.Slice(behalten, func(i, j int) bool { return behalten[i].index < behalten[j].index })
	if verworfen > 0 {
		fmt.Printf("aussortiert: %d von %d Konten koennen die geplanten Ueberweisungen nicht "+
			"bezahlen (unter %s wei) -- sie haetten die Buendel mitgerissen, in denen sie sitzen\n",
			verworfen, len(accs), mindest.String())
	}
	return behalten
}

func (c *rpcClient) tryNonce(addr string) (uint64, bool) {
	res, err := c.call("eth_getTransactionCount", []string{addr, "latest"})
	if err != nil {
		lastNonceErr = err
		return 0, false
	}
	var hexStr string
	if err := json.Unmarshal(res, &hexStr); err != nil {
		return 0, false
	}
	var n uint64
	if _, err := fmt.Sscanf(strings.TrimPrefix(hexStr, "0x"), "%x", &n); err != nil {
		return 0, false
	}
	return n, true
}

// tryNonceRetrying is nonce() without the panic: it reports failure instead of
// ending the process.
//
// WHY THAT MATTERS. On 2026-08-22 three separate runs produced no measurement
// at all. The generator was aborting in warmup, on ONE account out of 617,
// before a single transfer had been sent:
//
//	nonce(0x621d...): giving up after 8 attempts
//	panic: could not read nonce for 0x621d... after 8 attempts
//
// One unreadable account is not a reason to throw away a run across the other
// 616. It is a reason to drop that account and say so.
func (c *rpcClient) tryNonceRetrying(addr string) (uint64, bool) {
	for attempt := 1; attempt <= nonceMaxAttempts; attempt++ {
		if n, ok := c.tryNonce(addr); ok {
			return n, true
		}
		if attempt == nonceMaxAttempts {
			fmt.Printf("nonce(%s): giving up after %d attempts (last error: %v)\n",
				addr, nonceMaxAttempts, lastNonceErr)
			return 0, false
		}
		time.Sleep(time.Duration(attempt) * nonceRetryBackoff)
	}
	return 0, false
}

// nonce keeps the hard-failure behaviour for the one caller where a single
// account genuinely IS the run: the fund phase seeds, which pay for the rest.
func (c *rpcClient) nonce(addr string) uint64 {
	n, ok := c.tryNonceRetrying(addr)
	if !ok {
		must(fmt.Errorf("could not read nonce for %s after %d attempts (last error: %v)",
			addr, nonceMaxAttempts, lastNonceErr))
	}
	return n
}

// prefetchNonces reads every pair's two nonces and keeps only the pairs where
// BOTH sides answered. A pair with one unreadable side cannot transact in
// either direction, so keeping it would only add failures to the throughput
// number it is supposed to contribute to.
func prefetchNonces(c *rpcClient, senders, recipients []*account) ([]*account, []*account) {
	keptS := make([]*account, 0, len(senders))
	keptR := make([]*account, 0, len(recipients))
	dropped := 0
	for i := range senders {
		sn, sok := c.tryNonceRetrying(senders[i].address)
		rn, rok := c.tryNonceRetrying(recipients[i].address)
		if !sok || !rok {
			dropped++
			continue
		}
		senders[i].nonce = sn
		recipients[i].nonce = rn
		keptS = append(keptS, senders[i])
		keptR = append(keptR, recipients[i])
	}
	if dropped > 0 {
		fmt.Printf("dropped %d of %d pair(s) whose nonce could not be read; running with %d\n",
			dropped, len(senders), len(keptS))
	}
	return keptS, keptR
}

func (c *rpcClient) sendValue(from *account, toAddr string, amountWei *big.Int) (string, error) {
	tx := types.NewTransaction(from.nonce, addrFromHex(toAddr), amountWei, 21000, big.NewInt(0), nil)
	signer := types.NewEIP155Signer(big.NewInt(chainID))
	signedTx, err := types.SignTx(tx, signer, from.priv)
	if err != nil {
		return "", err
	}
	raw, err := signedTx.MarshalBinary()
	if err != nil {
		return "", err
	}
	rawHex := "0x" + hex.EncodeToString(raw)
	res, err := c.call("eth_sendRawTransaction", []string{rawHex})
	if err != nil {
		return "", err
	}
	// Only advance past this nonce once the node has actually accepted it --
	// incrementing unconditionally (even on rejection) would permanently
	// desync this account's local nonce from the chain for the rest of the
	// run, since nothing re-syncs it mid-run.
	from.nonce++
	var txHash string
	json.Unmarshal(res, &txHash)
	return txHash, nil
}

// batchSize is how many signed transfers one pair packs into a single
// JSON-RPC batch request during the run phase.
//
// FIX (2026-07-24 — the run phase could not physically approach the 50k
// target, regardless of how well the chain performed): sendValue does ONE
// HTTP round trip per transfer and waits for the reply. That caps a pair at
// 1/latency transfers per second — at the ~40ms observed against Contabo2
// that is ~25/s, so even 72 pairs top out around 1,800/s. Filling a block in
// one BLOCK_TIME needed 50,000/s arriving back then; maxTxsPerBlock steht seit
// dem 21.08.2026 auf 10.000, die Rechnung hier bleibt aber als Beleg stehen,
// warum der Generator und nicht der Knoten die Grenze war. The
// generator, not the node, was the binding constraint, and no chain-side fix
// could ever have shown up in the number.
//
// Batching collapses ONE of the two limits, not both. The node supports
// JSON-RPC batches (evm_rpc.go's handleRPC: `body[0] == '['`, up to
// maxBatchSize=100 per request), so 100 transfers per request means 100x
// fewer round trips. That part still holds and is why batchSize exists.
//
// KORREKTUR (2026-08-29): der zweite Teil stimmte nicht mehr. Hier stand,
// der Begrenzer werde EINMAL JE ANFRAGE geprueft ("one tick, whatever the
// batch holds"), ein Buendel koste also 100x weniger Budget, und eine Quelle
// komme auf 2.000/s statt 20/s.
//
// Der P1-Fix vom 2026-07-21 (nach main am 2026-08-14) bucht JE POSTEN ab --
// genau um dieses Schlupfloch zu schliessen. Buendeln spart seither KEIN
// Budget mehr:
//
//	200 Posten je 10-s-Fenster = 20 Ueberweisungen/s je IP, egal wie gebuendelt
//
// Wer den Begrenzer nicht hochsetzt, misst also 20/s und nicht 2.000/s. Am
// 29.08.2026 kostete genau dieser Absatz einen Nachmittag: die Laeufe kamen
// auf 13-15 TPS, und die Zahl sah nach einem Knotenproblem aus.
//
// Vor einem Lastlauf gilt: Zielrate x 10 (Fensterlaenge) ist der Mindestwert
// fuer AEQUITAS_RPC_RATE_LIMIT_MAX. 10.000 TPS brauchen >= 100.000.
// Dafuer gibt es .github/workflows/set-rpc-rate-limit-contabo2.yml.
//
// Deliberately exactly maxBatchSize: the node rejects a larger batch outright
// ("batch too large"), and a smaller one would leave measured throughput on
// the table for no benefit.
const batchSize = 100

// sendValueBatch signs batchSize sequential transfers from `from` and submits
// them as ONE JSON-RPC batch. Returns how many the node accepted.
//
// Nonces are assigned locally and sequentially (from.nonce, +1, +2, ...)
// rather than re-fetched per transfer: the node processes a batch's entries
// in order (handleSingle in a loop), so a contiguous run from one sender is
// exactly what it expects. from.nonce advances only by the number actually
// accepted, preserving sendValue's own invariant — never advance past a
// nonce the node did not take, or this account desyncs for the rest of the
// run with nothing to re-sync it.
// toAddrs carries one recipient per transfer, so the same batch path serves
// both callers: the run phase passes batchSize copies of its pair's single
// recipient, while the fund phase passes a chunk of DISTINCT accounts. Before
// this, funding was a sequential loop with a 20ms pace ("gentle pace, this is
// not the stress test") — fine for 150 accounts, but 40+ seconds for the
// thousands of senders a 50,000/s run actually needs.
func (c *rpcClient) sendValueBatch(from *account, toAddrs []string, amountWei *big.Int) (int, error) {
	n := len(toAddrs)
	if n == 0 {
		return 0, nil
	}
	signer := types.NewEIP155Signer(big.NewInt(chainID))
	reqs := make([]rpcReq, 0, n)
	for i := 0; i < n; i++ {
		tx := types.NewTransaction(from.nonce+uint64(i), addrFromHex(toAddrs[i]), amountWei, 21000, big.NewInt(0), nil)
		signedTx, err := types.SignTx(tx, signer, from.priv)
		if err != nil {
			return 0, err
		}
		raw, err := signedTx.MarshalBinary()
		if err != nil {
			return 0, err
		}
		reqs = append(reqs, rpcReq{
			Jsonrpc: "2.0",
			Method:  "eth_sendRawTransaction",
			Params:  []string{"0x" + hex.EncodeToString(raw)},
			ID:      i + 1,
		})
	}
	body, err := json.Marshal(reqs)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequest("POST", c.url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	// A rate-limited or malformed batch comes back as a single error object,
	// not an array — surface it verbatim so the run summary can tally it the
	// same way it tallies single-send failures.
	var single rpcResp
	if json.Unmarshal(b, &single) == nil && single.Error != nil {
		return 0, fmt.Errorf("rpc error %d: %s", single.Error.Code, single.Error.Message)
	}
	var results []rpcResp
	if err := json.Unmarshal(b, &results); err != nil {
		return 0, fmt.Errorf("bad batch response %q: %w", string(b), err)
	}
	accepted := 0
	var firstErr error
	for _, r := range results {
		if r.Error != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("rpc error %d: %s", r.Error.Code, r.Error.Message)
			}
			continue
		}
		accepted++
	}
	// Only advance by what was actually taken. A partially-accepted batch is
	// reported as an error too, so the caller re-syncs rather than assuming.
	from.nonce += uint64(accepted)
	if accepted < len(results) || len(results) < n {
		if firstErr == nil {
			firstErr = fmt.Errorf("batch partially accepted: %d of %d", accepted, n)
		}
		return accepted, firstErr
	}
	return accepted, nil
}

// sendRetryBackoff is how long a run-phase pair waits after a failed send
// before retrying. See the run loop's own comment for why this exists and
// why it is deliberately flat rather than exponential.
const sendRetryBackoff = 50 * time.Millisecond

// normalizeErrForTally collapses the variable parts of an error string
// (hashes, addresses, nonces, hex blobs) so that N occurrences of the same
// underlying cause tally as one entry instead of N distinct ones. Without
// this, an error like "nonce too low: address 0xabc... want 42" would
// produce a unique key per account per nonce, which is exactly as useless
// as the bare count it replaces.
func normalizeErrForTally(s string) string {
	var b strings.Builder
	for _, f := range strings.Fields(s) {
		// Satzzeichen abtrennen und danach wieder anfuegen. Ohne das griff die
		// Adresspruefung bei "(0xabc..." nicht -- im Lauf vom 29.08.2026 blieb
		// deshalb die erste Adresse eines Paares im Klartext stehen und die
		// zweite wurde ersetzt, also zaehlte jede Adresse als eigene Ursache.
		vorn := strings.TrimLeft(f, "(\"'")
		links := f[:len(f)-len(vorn)]
		kern := strings.TrimRight(vorn, ".,;:)\"'")
		rechts := vorn[len(kern):]
		f = kern
		switch {
		case strings.HasPrefix(f, "0x") && len(f) > 6:
			f = "0x<hex>"
		case isAllDigits(f):
			f = "<n>"
		case isFraction(f):
			// "batch member 9/26" ist keine eigene Ursache. Ohne diesen Fall
			// zerfiel EINE Ursache in 23 scheinbar verschiedene, jede mit
			// kleiner Zahl -- und die haeufigste war nicht mehr erkennbar.
			f = "<i>/<n>"
		}
		f = links + f + rechts
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(f)
	}
	out := b.String()
	if len(out) > 200 {
		out = out[:200] + "..."
	}
	return out
}

// isFraction erkennt Positionsangaben wie "9/26".
func isFraction(s string) bool {
	i := strings.IndexByte(s, '/')
	if i <= 0 || i == len(s)-1 {
		return false
	}
	return isAllDigits(s[:i]) && isAllDigits(s[i+1:])
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// printErrTally prints the distinct failure causes seen during the run
// phase, most frequent first. This is the difference between "37,327
// failures" (unactionable) and "37,327 failures, all of them <one specific
// node-side rejection>" (immediately actionable).
func printErrTally(tally map[string]int64) {
	if len(tally) == 0 {
		return
	}
	type row struct {
		msg string
		n   int64
	}
	rows := make([]row, 0, len(tally))
	for m, n := range tally {
		rows = append(rows, row{m, n})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].n > rows[j].n })
	fmt.Printf("=== run-phase failure causes (%d distinct) ===\n", len(rows))
	for i, r := range rows {
		if i >= 10 {
			fmt.Printf("  ... and %d more distinct cause(s)\n", len(rows)-i)
			break
		}
		fmt.Printf("  %8d  %s\n", r.n, r.msg)
	}
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
	maxPairs := flag.Int("pairs", 0, "cap on concurrent sender pairs (0 = every pair in the CSV). Lower values find the rate the network SUSTAINS, as opposed to the peak it briefly reaches.")
	topology := flag.String("topology", "pairs", "pairs = disjoint A<->B couples, half as many senders as accounts; ring = every account sends to the next, twice the concurrency but neighbouring senders share an account")
	fundAmount := flag.String("fund-amount-wei", "1000000000000000", "wei sent from a seed to each test account (default 0.001 AEQ)")
	transferAmount := flag.String("transfer-amount-wei", "10000000000000", "wei per load-test transfer (default 0.00001 AEQ)")
	runDuration := flag.Duration("duration", 20*time.Second, "timed load phase duration")
	// Vorgabe: das Hundertfache einer Ueberweisung. Wer weniger haelt, haelt
	// den Lauf nicht durch und reisst nur Buendel mit. "0" schaltet es ab.
	minBalanceWei := flag.String("min-balance-wei", "1000000000000000",
		"Konten unter diesem Stand vor dem Lauf aussortieren (0 = nicht aussortieren)")
	rampSeconds := flag.Int("ramp", 5, "seconds to ramp concurrency up over")
	// Both of these were fixed constants until the 2026-07-25 run showed the
	// working point has to be FOUND, not assumed: 288 requests exceeded the
	// 10s client timeout, meaning the node needed longer than that to answer a
	// 100-transfer batch. Whether the fix is more patience or smaller batches
	// is an empirical question, and it cannot be asked without turning both
	// into inputs. Defaults keep the batch size exactly as before.
	httpTimeout := flag.Duration("http-timeout", 30*time.Second, "per-request HTTP timeout; must exceed how long the node takes to answer one batch, but stay well under -duration so a stuck sender still recovers within the run")
	batchSizeFlag := flag.Int("batch-size", batchSize, "transfers per JSON-RPC batch (clamped to 1..100; the node rejects anything above its own maxBatchSize=100)")
	flag.Parse()

	// Clamp rather than reject: the node refuses a batch above maxBatchSize=100
	// outright, and a run that dies on argument validation after the operator
	// waited for a deploy window is a worse outcome than one that quietly uses
	// the largest legal value and says so.
	effBatchSize := *batchSizeFlag
	if effBatchSize < 1 {
		effBatchSize = 1
	}
	if effBatchSize > batchSize {
		effBatchSize = batchSize
	}
	if effBatchSize != *batchSizeFlag {
		fmt.Printf("batch-size %d out of range — using %d\n", *batchSizeFlag, effBatchSize)
	}

	accs := loadAccounts(*csvPath)
	if len(accs) <= *numSeeds {
		panic("not enough accounts for the requested seed count")
	}
	seeds := accs[:*numSeeds]
	testAccs := accs[*numSeeds:]
	// Zahlungsunfaehige Konten VOR der Paarbildung entfernen -- siehe
	// aussortieren(). Eigener kleiner Client, damit hier nichts an der
	// bestehenden Reihenfolge verschoben werden muss.
	if *minBalanceWei != "" && *minBalanceWei != "0" {
		if mindest, ok := new(big.Int).SetString(*minBalanceWei, 10); !ok {
			fmt.Printf("-min-balance-wei %q ist keine Zahl -- es wird nicht aussortiert\n", *minBalanceWei)
		} else {
			vorpruefer := &rpcClient{url: *rpcURL, hc: &http.Client{Timeout: 30 * time.Second}}
			testAccs = aussortieren(vorpruefer, testAccs, mindest)
			if len(testAccs) < 2 {
				fmt.Println("nach dem Aussortieren bleiben weniger als zwei Konten -- nichts zu messen")
				return
			}
		}
	}
	// TOPOLOGY. Throughput here is senders divided by per-transfer latency,
	// because one JSON-RPC batch carries transfers from a single sender and the
	// node applies a batch in order. So the sender count IS the concurrency the
	// node ever sees, and with "pairs" it is only half the accounts:
	//
	//	623 funded accounts -> 311 pairs -> ~3,400-3,800 TPS measured
	//
	// "ring" makes every account a sender (i sends to i+1), doubling that for
	// free. The cost is that neighbouring senders share an account: goroutine i
	// touches accounts i and i+1, goroutine i+1 touches i+1 and i+2.
	//
	// That overlap is exactly what the run loop's own comment below warns a
	// ring would break, and the warning was right WHEN IT WAS WRITTEN: a
	// contended shard used to make a transfer BLOCK, because
	// transferConcurrentWAL's eligibility checks called shardedAccounts.Get,
	// which locks. That was removed on 2026-08-22 after it measured 46ms of a
	// 67ms transfer, and a contended shard now bails cleanly to the batcher
	// instead -- which amortises commits across many transfers and is the
	// path TryLockAddrs was always designed to fall through to.
	//
	// So the trade changed and is worth measuring rather than assuming. Default
	// stays "pairs": this flag exists to produce a number, not to be believed
	// in advance.
	//
	// MEASURED 2026-08-22, and the result does NOT settle the question:
	//
	//	pairs  308 senders   2,616 TPS    1,631 failures
	//	ring   617 senders   1,965 TPS  216,306 failures
	//
	// Every one of those 216,306 was "insufficient balance", not contention.
	// accounts-funded.csv is written richest-first, so in the pairs topology
	// the senders are the RICH half and the recipients the poor half. The ring
	// makes the poor half send too, and they run dry within the first minute.
	//
	// So this measured the CSV's wealth distribution, not the topology. A fair
	// comparison needs evenly funded accounts (loadtest-widen-senders.yml),
	// and until that exists the ring number should not be quoted as evidence
	// against the ring.
	numPairs := len(testAccs) / 2
	ring := *topology == "ring"
	if ring {
		numPairs = len(testAccs)
	} else if *topology != "pairs" {
		fmt.Printf("unknown -topology %q, using pairs\n", *topology)
	}
	// Capped deliberately. Peak throughput turned out to be the less useful
	// number: every run at ~7-8k/s drove the node into an orphan backlog it
	// needed half an hour to clear, because blocks were produced faster than
	// the network could merge them. What matters is the rate it holds without
	// falling behind, and finding that needs a knob.
	if *maxPairs > 0 && *maxPairs < numPairs {
		numPairs = *maxPairs
	}
	var senders, recipients []*account
	if ring {
		senders = testAccs[:numPairs]
		recipients = make([]*account, numPairs)
		for i := range recipients {
			recipients[i] = testAccs[(i+1)%numPairs]
		}
	} else {
		senders = testAccs[:numPairs]
		recipients = testAccs[numPairs : 2*numPairs]
	}
	fmt.Printf("topology %s: %d concurrent senders over %d accounts\n",
		*topology, numPairs, len(testAccs))

	// The connection pool is load-bearing here, and leaving it at the default
	// made this tool measure ITSELF rather than the node.
	//
	// With Transport nil, Go uses http.DefaultTransport, whose
	// MaxIdleConnsPerHost is 2. The run phase starts one goroutine per pair
	// (72 for the standard 150 accounts), so 72 senders were sharing two
	// reusable connections: every request beyond the second had to open a
	// fresh TCP connection and discard it afterwards, because the idle pool
	// was already full.
	//
	// Measured on Contabo2, 2026-07-26: goroutine dumps taken during confirmed
	// load (the node reporting 423 transfers/s at that very instant) showed
	// 72-74 goroutines against 65 idle — only 7 to 9 concurrent requests
	// actually reaching the node, from 72 client goroutines. None of them were
	// blocked on a lock, which is what ruled out the node's own serialisation
	// (cs.mu, roadmap step 5) and pointed here instead.
	//
	// This also explains why the WAL group commit never batched: it can only
	// bundle Appends that are in flight at the same moment, and with a handful
	// of requests arriving at a time there was nothing to bundle. The disk
	// measured 133-169 fsyncs/s carrying up to MaxBatchSize=500 records each;
	// at 423 transfers/s it was carrying roughly one.
	//
	// Sized far above any realistic pair count so the pool cannot become the
	// constraint again. MaxConnsPerHost stays 0 (unlimited) deliberately: a cap
	// here would silently reintroduce the very queueing this removes.
	client := &rpcClient{url: *rpcURL, hc: &http.Client{
		Timeout: *httpTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        1024,
			MaxIdleConnsPerHost: 1024,
			MaxConnsPerHost:     0,
			IdleConnTimeout:     90 * time.Second,
		},
	}}
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
		// Batched, per seed. The previous version sent one transfer per HTTP
		// round trip with a fixed 20ms pace ("gentle pace, this is not the
		// stress test") — correct and fast enough for 150 accounts, but a
		// 50,000/s run needs thousands of distinct senders, and at 20ms each
		// that funding alone would take minutes and burn the per-IP rate
		// limit budget the run phase needs.
		//
		// Grouping by SEED rather than walking testAccs in order is what makes
		// batching possible at all: every transfer in one batch must come from
		// the same sender, because sendValueBatch assigns that sender's nonces
		// sequentially. The old i%len(seeds) round-robin interleaved seeds on
		// purpose (spreading load); here the same spreading is achieved across
		// batches instead, one seed at a time.
		var failed int
		perSeed := make([][]string, len(seeds))
		for i, ta := range testAccs {
			si := i % len(seeds)
			perSeed[si] = append(perSeed[si], ta.address)
		}
		funded := 0
		for si, targets := range perSeed {
			seed := seeds[si]
			for off := 0; off < len(targets); off += effBatchSize {
				end := off + effBatchSize
				if end > len(targets) {
					end = len(targets)
				}
				chunk := targets[off:end]
				accepted, err := client.sendValueBatch(seed, chunk, fundWei)
				funded += accepted
				if err != nil {
					failed += len(chunk) - accepted
					fmt.Printf("fund seed %d batch [%d:%d] accepted %d/%d, error: %v\n",
						si, off, end, accepted, len(chunk), err)
					continue
				}
				fmt.Printf("fund seed %d: %d/%d funded (batch of %d) — total %d/%d\n",
					si, end, len(targets), len(chunk), funded, len(testAccs))
			}
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
		// Tolerant: one account whose nonce cannot be read drops its pair
		// rather than aborting the whole run. See tryNonceRetrying.
		senders, recipients = prefetchNonces(client, senders, recipients)
		numPairs = len(senders)
		if numPairs == 0 {
			fmt.Println("no pair had both nonces readable — nothing to measure")
			return
		}
		var failed int
		for i := 0; i < numPairs; i++ {
			_, err := client.sendValue(senders[i], recipients[i].address, transferWei)
			// Whichever side of the pair holds the money sends. A previous run
			// ends with every sender empty and every recipient holding what it
			// sent them -- so a fresh run in the fixed A->B direction fails on
			// all 597 pairs with "insufficient balance", which is exactly what
			// happened and is what stopped the measurement. Trying the reverse
			// makes the harness self-correcting instead of needing the accounts
			// re-funded from outside every time.
			if err != nil && strings.Contains(err.Error(), "insufficient balance") {
				_, err = client.sendValue(recipients[i], senders[i].address, transferWei)
			}
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
		// Recipients send too in the pairs topology (the direction-alternating
		// loop below), so their nonces have to be current for the same reason
		// the senders' are. Tolerant for the same reason as warmup.
		senders, recipients = prefetchNonces(client, senders, recipients)
		numPairs = len(senders)
		if numPairs == 0 {
			fmt.Println("no pair had both nonces readable — nothing to measure")
			return
		}

		var succeeded, failed int64
		// errTally records WHY sends failed, not just how many. The 2026-07-24
		// Contabo2 run reported "succeeded: 128 failed: 37327" with no way at
		// all to tell whether that was nonce desync, a node-side rejection, an
		// RPC transport error, or the node simply not producing blocks -- an
		// unusable measurement for the one question this tool exists to answer
		// (SCALING_ARCHITECTURE.md's real-hardware validation gap). Keyed by a
		// normalized error string so a handful of distinct causes don't turn
		// into tens of thousands of unique map entries.
		var errMu sync.Mutex
		errTally := map[string]int64{}
		recordErr := func(err error) {
			key := normalizeErrForTally(err.Error())
			errMu.Lock()
			errTally[key]++
			errMu.Unlock()
		}
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
				// Built once per pair, not per batch: every transfer in this
				// pair's batches goes to the same recipient, and reallocating
				// a batchSize slice inside the hot loop would add allocation
				// pressure to the very process whose throughput is measured.
				//
				// Two of them, one per direction. The pair alternates which side
				// sends: with a fixed direction every sender's balance walks
				// monotonically to zero and the working set is single-use, which
				// is why a run that had just measured 60,004 transfers could not
				// be repeated -- all 1,195 accounts read 0 and the seeds were
				// empty too, so there was nothing left to fund them from.
				// Alternating makes the balance oscillate by one batch instead of
				// draining, so the same accounts carry an unlimited number of
				// runs. The pairs stay DISJOINT (no account appears in two pairs),
				// which is the property that keeps concurrent transfers from
				// contending on the same account shard -- a ring topology would
				// have broken exactly that.
				batchTargets := make([]string, effBatchSize)
				for i := range batchTargets {
					batchTargets[i] = to.address
				}
				reverseTargets := make([]string, effBatchSize)
				for i := range reverseTargets {
					reverseTargets[i] = from.address
				}
				// Direction alternation is a PAIRS-only device, and it would
				// be a correctness bug in a ring: goroutine i sending from
				// acc[i+1] would use the very account goroutine i+1 is already
				// sending from, and two goroutines assigning one account's
				// nonces desynchronise both for the rest of the run.
				//
				// It is also unnecessary there. Alternation exists because a
				// fixed direction walks every sender's balance to zero; in a
				// ring each account sends one batch and receives one batch per
				// round, so balances stay level on their own.
				forward := true
				for {
					select {
					case <-stopCh:
						return
					default:
					}
					// Batched: ONE HTTP round trip carries batchSize transfers.
					// See batchSize's own comment for why single sends made
					// the generator — not the node — the binding constraint,
					// and why this also collapses the per-IP rate limit by the
					// same factor. succeeded/failed still count TRANSFERS, not
					// requests, so the reported throughput stays comparable
					// with every earlier run.
					sender, targets := from, batchTargets
					if !forward {
						sender, targets = to, reverseTargets
					}
					accepted, err := client.sendValueBatch(sender, targets, transferWei)
					atomic.AddInt64(&succeeded, int64(accepted))
					// Flip for the next batch, so this pair's two balances
					// oscillate around their starting point rather than one
					// walking to zero. Flipped unconditionally, including after a
					// failure: an "insufficient balance" means this side is the
					// empty one, and the other side is then holding everything
					// this side ever sent it -- so the very next batch is the one
					// that can succeed.
					if !ring {
						forward = !forward
					}
					if err != nil {
						atomic.AddInt64(&failed, int64(effBatchSize-accepted))
						recordErr(err)
						// FIX (2026-07-25): re-read this sender's nonce from
						// the node before retrying. sendValueBatch's own
						// comment already named this failure mode -- "never
						// advance past a nonce the node did not take, or this
						// account desyncs for the rest of the run with nothing
						// to re-sync it" -- but the guard is one-directional:
						// it prevents advancing too FAR and has no answer for
						// advancing too LITTLE. This is that answer.
						//
						// Measured, run of 2026-07-25 06:30 (valid: the node
						// did not restart): succeeded 0, failed 122900, and in
						// every one of the 63 distinct causes `expected` was
						// exactly tx+100 -- one batchSize:
						//
						//     288  context deadline exceeded (10s client timeout)
						//     264  nonce too low: tx=11 expected=111
						//     139  nonce too low: tx=12 expected=112
						//     101  nonce too low: tx=14 expected=114
						//
						// The batch reached the node and was applied in full;
						// the RESPONSE missed the client's 10s timeout, so
						// hc.Do returned an error and sendValueBatch returned
						// (0, err) before `from.nonce += accepted` could run.
						// From then on that sender was permanently 100 behind,
						// and since the node requires exact equality (too high
						// is refused as well as too low) nothing could ever
						// bring it back. 288 timeouts each killed a sender for
						// good; by the end all 72 were dead, which is why the
						// run scored exactly zero.
						//
						// A timeout leaves the outcome genuinely unknown to
						// the client, so guessing either way is wrong -- ask
						// the node instead. tryNonce reads the same counter
						// sendRawTransaction checks, so the answer is
						// authoritative. If the node is still mid-apply the
						// read may briefly return the pre-batch value; the
						// next failure simply re-syncs again, so it converges
						// instead of latching. Failure to read is left alone
						// rather than guessed at.
						// Re-sync the account that actually sent this batch, not
						// always `from` -- with alternating directions the sender
						// is `to` on every other pass, and resyncing the wrong
						// side would leave the real one permanently desynced,
						// which is the exact failure this whole block exists to
						// prevent.
						if n, ok := client.tryNonce(sender.address); ok {
							sender.nonce = n
						}
						// Back off before retrying. Without this, a persistently
						// failing pair spins this loop as fast as the RPC can
						// reject, which (a) inflates the failure count into a
						// meaningless number that says more about retry speed
						// than about the node, and (b) burns CPU on the very box
						// whose throughput is being measured -- both seen in the
						// 2026-07-24 run (37,327 "failures" in 25s from 72 pairs).
						// Deliberately a flat, short sleep rather than exponential
						// backoff: the run phase is a fixed-duration throughput
						// measurement, not a client that needs to survive an
						// outage, and a growing delay would silently stop
						// generating load partway through the timed window.
						select {
						case <-time.After(sendRetryBackoff):
						case <-stopCh:
							return
						}
						continue
					}
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
		printErrTally(errTally)
	}
}
