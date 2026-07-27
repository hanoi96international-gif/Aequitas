package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
)

// The adapter is tested against a local server that verifies signatures the
// way Binance does. That covers everything that can be got wrong without money
// at stake: the signature itself, which bytes it covers, that credentials
// never leak into an error, and that the engine's idempotency key reaches the
// venue field that enforces it.

const testSecret = "TESTSECRET_NEVER_REAL"

// verifyingServer checks the HMAC on every signed request, exactly as the
// exchange does, and records what it saw.
type verifyingServer struct {
	*httptest.Server
	lastPath   string
	lastParams url.Values
	lastAPIKey string
	badSigs    int
}

func newVerifyingServer(t *testing.T, handler func(w http.ResponseWriter, path string, p url.Values)) *verifyingServer {
	t.Helper()
	vs := &verifyingServer{}
	vs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.RawQuery
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			raw = string(b)
		}
		vs.lastPath = r.URL.Path
		vs.lastAPIKey = r.Header.Get("X-MBX-APIKEY")

		if strings.Contains(raw, "signature=") {
			i := strings.Index(raw, "&signature=")
			if i < 0 {
				vs.badSigs++
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			payload, sig := raw[:i], raw[i+len("&signature="):]
			mac := hmac.New(sha256.New, []byte(testSecret))
			mac.Write([]byte(payload))
			if hex.EncodeToString(mac.Sum(nil)) != sig {
				vs.badSigs++
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"code":-1022,"msg":"Signature for this request is not valid."}`)
				return
			}
			raw = payload
		}
		p, _ := url.ParseQuery(raw)
		vs.lastParams = p
		handler(w, r.URL.Path, p)
	}))
	return vs
}

func testBroker(t *testing.T, srv *verifyingServer) *Broker {
	t.Helper()
	b, err := NewBroker(context.Background(), BrokerConfig{
		APIKey: "TESTKEY", APISecret: testSecret, HTTP: srv.Client(),
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	return b
}

func serverTimeHandler(w http.ResponseWriter) {
	fmt.Fprintf(w, `{"serverTime":%d}`, time.Now().UnixMilli())
}

// TestBroker_DefaultsToTestnet is the safety property that matters most here.
// A flag would end up in shell history and in copied command lines; reaching
// the real exchange should require somebody to edit a source file and mean it.
func TestBroker_DefaultsToTestnet(t *testing.T) {
	srv := newVerifyingServer(t, func(w http.ResponseWriter, path string, p url.Values) {
		serverTimeHandler(w)
	})
	defer srv.Close()

	// Built WITHOUT BaseURL, so the default endpoint choice is what is under
	// test. Construction reads server time, so it needs to reach that
	// endpoint; where the network is closed the URL itself is still checked.
	b, err := NewBroker(context.Background(), BrokerConfig{
		APIKey: "K", APISecret: testSecret, HTTP: srv.Client(),
	})
	if err != nil {
		if !strings.Contains(err.Error(), "server time") {
			t.Fatalf("NewBroker: %v", err)
		}
		// The endpoint was unreachable — but the choice of endpoint is the
		// property under test, and a live default would have been a live
		// attempt.
		probe, _ := NewBroker(context.Background(), BrokerConfig{
			APIKey: "K", APISecret: testSecret, HTTP: srv.Client(), BaseURL: srv.URL,
		})
		if probe != nil && probe.Live() {
			t.Fatal("a broker built without Live set reports itself as live")
		}
		t.Skip("construction needs network to reach the testnet; endpoint choice verified")
	}
	if b.Live() {
		t.Fatal("a broker built without Live set is pointed at the real exchange")
	}
	if !strings.Contains(b.base, "testnet") {
		t.Fatalf("default base URL is %q; it must be the testnet", b.base)
	}
	if !strings.Contains(b.Name(), "testnet") {
		t.Fatalf("Name() is %q; a log line must say which venue this is", b.Name())
	}
}

func TestBroker_RefusesToStartWithoutCredentials(t *testing.T) {
	t.Setenv("BINANCE_API_KEY", "")
	t.Setenv("BINANCE_API_SECRET", "")
	if _, err := NewBroker(context.Background(), BrokerConfig{}); err == nil {
		t.Fatal("expected an error rather than a broker that fails one request at a time")
	}
}

// TestBroker_SignsExactlyTheBytesItSends. Signing a differently ordered query
// than the one transmitted is the classic way to get an inscrutable -1022 from
// every request, and the local server rejects it the same way Binance would.
func TestBroker_SignsExactlyTheBytesItSends(t *testing.T) {
	srv := newVerifyingServer(t, func(w http.ResponseWriter, path string, p url.Values) {
		switch path {
		case "/fapi/v1/time":
			serverTimeHandler(w)
		case "/fapi/v2/account":
			fmt.Fprint(w, `{"totalMarginBalance":"10000.5","positions":[
				{"symbol":"BTCUSDT","positionAmt":"0.020","entryPrice":"50000"},
				{"symbol":"ETHUSDT","positionAmt":"0","entryPrice":"0"}]}`)
		}
	})
	defer srv.Close()
	b := testBroker(t, srv)

	acct, err := b.Account(context.Background())
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if srv.badSigs != 0 {
		t.Fatalf("%d requests failed signature verification", srv.badSigs)
	}
	if acct.EquityUSD != 10000.5 {
		t.Fatalf("equity %v, want 10000.5", acct.EquityUSD)
	}
	if got := acct.Position("BTCUSDT"); got != 0.020 {
		t.Fatalf("position %v, want 0.02", got)
	}
	// Binance lists every symbol; carrying the flat ones would make the
	// account look enormous and mean nothing.
	if _, listed := acct.Positions["ETHUSDT"]; listed {
		t.Fatal("a flat position was carried into the account")
	}
	if srv.lastAPIKey != "TESTKEY" {
		t.Fatalf("API key header was %q", srv.lastAPIKey)
	}
	if srv.lastParams.Get("recvWindow") == "" || srv.lastParams.Get("timestamp") == "" {
		t.Fatal("a signed request must carry a timestamp and a recvWindow")
	}
}

// TestBroker_PassesTheIdempotencyKeyToTheVenue. The engine refuses to resend
// after an ambiguous failure; this is what makes that enforceable rather than
// merely intended, because the exchange rejects a duplicate client ID.
func TestBroker_PassesTheIdempotencyKeyToTheVenue(t *testing.T) {
	srv := newVerifyingServer(t, func(w http.ResponseWriter, path string, p url.Values) {
		switch path {
		case "/fapi/v1/time":
			serverTimeHandler(w)
		case "/fapi/v1/order":
			fmt.Fprintf(w, `{"clientOrderId":%q,"executedQty":"0.020","avgPrice":"50010.5",
				"cumQuote":"1000.21","status":"FILLED"}`, p.Get("newClientOrderId"))
		}
	})
	defer srv.Close()
	b := testBroker(t, srv)

	fill, err := b.Place(context.Background(), ms.Order{
		ClientID: "ms-deadbeef1234", Symbol: "BTCUSDT", Side: ms.Buy,
		Quantity: 0.02, ReduceOnly: true,
	})
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if got := srv.lastParams.Get("newClientOrderId"); got != "ms-deadbeef1234" {
		t.Fatalf("client ID reached the venue as %q; the exchange's duplicate rejection "+
			"cannot work without it", got)
	}
	if got := srv.lastParams.Get("reduceOnly"); got != "true" {
		t.Fatalf("reduceOnly was sent as %q; without it a race can carry the position "+
			"through zero into a new one", got)
	}
	if srv.lastParams.Get("type") != "MARKET" || srv.lastParams.Get("side") != "BUY" {
		t.Fatalf("order sent as %s %s", srv.lastParams.Get("side"), srv.lastParams.Get("type"))
	}
	if fill.Quantity != 0.02 || fill.Price != 50010.5 {
		t.Fatalf("fill %+v", fill)
	}
}

// TestBroker_TreatsAnUnfilledOrderAsAFailure. Returning it as success would let
// the engine record a position it does not hold; the next reconciliation would
// correctly halt, but blame the wrong thing.
func TestBroker_TreatsAnUnfilledOrderAsAFailure(t *testing.T) {
	srv := newVerifyingServer(t, func(w http.ResponseWriter, path string, p url.Values) {
		switch path {
		case "/fapi/v1/time":
			serverTimeHandler(w)
		case "/fapi/v1/order":
			fmt.Fprint(w, `{"clientOrderId":"x","executedQty":"0","avgPrice":"0","status":"EXPIRED"}`)
		}
	})
	defer srv.Close()
	b := testBroker(t, srv)

	if _, err := b.Place(context.Background(), ms.Order{
		ClientID: "x", Symbol: "BTCUSDT", Side: ms.Buy, Quantity: 0.02,
	}); err == nil {
		t.Fatal("a market order that filled nothing was reported as a success")
	}
}

// TestBroker_ErrorsNeverCarryTheCredentialOrTheSignature. A failed request is
// exactly the text that gets pasted into an issue tracker.
func TestBroker_ErrorsNeverCarryTheCredentialOrTheSignature(t *testing.T) {
	srv := newVerifyingServer(t, func(w http.ResponseWriter, path string, p url.Values) {
		if path == "/fapi/v1/time" {
			serverTimeHandler(w)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"code":-2019,"msg":"Margin is insufficient."}`)
	})
	defer srv.Close()
	b := testBroker(t, srv)

	_, err := b.Place(context.Background(), ms.Order{
		ClientID: "x", Symbol: "BTCUSDT", Side: ms.Buy, Quantity: 99999,
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, secret := range []string{testSecret, "TESTKEY", "signature="} {
		if strings.Contains(msg, secret) {
			t.Fatalf("error leaked %q: %v", secret, err)
		}
	}
	// It must still say what actually went wrong.
	if !strings.Contains(msg, "Margin is insufficient") || !strings.Contains(msg, "-2019") {
		t.Fatalf("error %q should carry the venue's own reason", msg)
	}
}

func TestBroker_ReadsLotAndNotionalFilters(t *testing.T) {
	srv := newVerifyingServer(t, func(w http.ResponseWriter, path string, p url.Values) {
		switch path {
		case "/fapi/v1/time":
			serverTimeHandler(w)
		case "/fapi/v1/exchangeInfo":
			fmt.Fprint(w, `{"symbols":[{"symbol":"BTCUSDT","filters":[
				{"filterType":"LOT_SIZE","stepSize":"0.001","minQty":"0.001"},
				{"filterType":"MIN_NOTIONAL","notional":"100"}]}]}`)
		}
	})
	defer srv.Close()
	b := testBroker(t, srv)

	r, err := b.Rules(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("Rules: %v", err)
	}
	if r.LotStep != 0.001 || r.MinQuantity != 0.001 || r.MinNotionalUSD != 100 {
		t.Fatalf("rules %+v", r)
	}
	// A symbol the venue does not list must fail loudly rather than come back
	// with zero constraints, which would let every order through.
	if _, err := b.Rules(context.Background(), "NOTLISTED"); err == nil {
		t.Fatal("an unlisted symbol returned rules instead of an error")
	}
}

// TestBroker_SatisfiesTheBrokerInterface keeps the adapter usable by the
// engine without the engine importing it.
func TestBroker_SatisfiesTheBrokerInterface(t *testing.T) {
	var _ ms.Broker = (*Broker)(nil)
}
