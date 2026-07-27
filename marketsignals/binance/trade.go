package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	ms "github.com/hanoi96international-gif/marketsignals"
)

// The trading adapter.
//
// This is the one file in the module that can move money, and it is built
// accordingly.
//
// TESTNET IS THE DEFAULT. NewBroker points at Binance's futures testnet unless
// Live is set explicitly in code. A flag would not do: flags end up in shell
// history, in copied command lines, and in a systemd unit somebody wrote a
// year ago. Reaching the real exchange should require somebody to edit a
// source file and mean it.
//
// CREDENTIALS COME FROM THE ENVIRONMENT and are never written anywhere. They
// are not in the config file, not in a log line, and not in an error string —
// a failed request is exactly the text that gets pasted into an issue tracker,
// so this file's errors carry status codes and Binance's own message and
// nothing else.
//
// IDEMPOTENCY IS THE VENUE'S. Every order carries the Engine's deterministic
// client ID as newClientOrderId, which is Binance's own duplicate-rejection
// mechanism. That is what makes the engine's "never resend after an ambiguous
// failure" rule enforceable rather than merely intended: if a retry did happen,
// the exchange refuses it.

// BrokerConfig configures the trading adapter.
type BrokerConfig struct {
	// APIKey and APISecret. Empty means read BINANCE_API_KEY and
	// BINANCE_API_SECRET from the environment.
	APIKey    string
	APISecret string

	// Live points the adapter at the real exchange. It is deliberately a
	// struct field rather than a flag, so that trading real money requires
	// editing code rather than remembering an argument.
	Live bool

	// BaseURL overrides the endpoint entirely — for a regional Binance
	// domain, for a corporate egress proxy, or for a local server in tests.
	// It takes precedence over Live, so a BaseURL pointing somewhere harmless
	// stays harmless.
	BaseURL string

	// RecvWindow is how long Binance will accept a signed request after its
	// timestamp. Signed requests fail on clock skew, and the failure looks
	// like a permissions problem, so the adapter measures the server's offset
	// once at construction rather than leaving somebody to debug it.
	RecvWindow time.Duration

	HTTP *http.Client
}

// Broker implements marketsignals.Broker against Binance USDⓈ-M futures.
type Broker struct {
	key, secret string
	base        string
	live        bool
	recvWindow  time.Duration
	http        *http.Client

	mu        sync.Mutex
	timeDelta time.Duration
	rules     map[string]ms.SymbolRules
}

// NewBroker builds the adapter. It fails rather than starting in a state where
// requests would be rejected one at a time for a reason nobody can see.
func NewBroker(ctx context.Context, cfg BrokerConfig) (*Broker, error) {
	key, secret := cfg.APIKey, cfg.APISecret
	if key == "" {
		key = os.Getenv("BINANCE_API_KEY")
	}
	if secret == "" {
		secret = os.Getenv("BINANCE_API_SECRET")
	}
	if key == "" || secret == "" {
		return nil, fmt.Errorf("no API credentials: set BINANCE_API_KEY and " +
			"BINANCE_API_SECRET in the environment")
	}

	base := "https://testnet.binancefuture.com"
	if cfg.Live {
		base = "https://fapi.binance.com"
	}
	if cfg.BaseURL != "" {
		base = strings.TrimSuffix(cfg.BaseURL, "/")
	}
	recv := cfg.RecvWindow
	if recv <= 0 {
		recv = 5 * time.Second
	}
	client := cfg.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	b := &Broker{
		key: key, secret: secret, base: base, live: cfg.Live,
		recvWindow: recv, http: client, rules: map[string]ms.SymbolRules{},
	}
	if err := b.syncTime(ctx); err != nil {
		return nil, fmt.Errorf("could not read server time: %w", err)
	}
	return b, nil
}

// Name identifies the broker without revealing anything about the account.
func (b *Broker) Name() string {
	if b.live {
		return "binance-futures-LIVE"
	}
	return "binance-futures-testnet"
}

// Live reports whether this adapter is pointed at the real exchange.
func (b *Broker) Live() bool { return b.live }

// syncTime measures the offset between the local clock and Binance's.
func (b *Broker) syncTime(ctx context.Context) error {
	body, err := b.get(ctx, "/fapi/v1/time", nil, false)
	if err != nil {
		return err
	}
	var resp struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	b.mu.Lock()
	b.timeDelta = time.Until(time.UnixMilli(resp.ServerTime))
	b.mu.Unlock()
	return nil
}

// Account reads the venue's record of the account.
func (b *Broker) Account(ctx context.Context) (ms.Account, error) {
	body, err := b.get(ctx, "/fapi/v2/account", nil, true)
	if err != nil {
		return ms.Account{}, err
	}
	var resp struct {
		TotalMarginBalance string `json:"totalMarginBalance"`
		Positions          []struct {
			Symbol      string `json:"symbol"`
			PositionAmt string `json:"positionAmt"`
			EntryPrice  string `json:"entryPrice"`
		} `json:"positions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ms.Account{}, fmt.Errorf("account: %w", err)
	}

	equity, err := strconv.ParseFloat(resp.TotalMarginBalance, 64)
	if err != nil {
		return ms.Account{}, fmt.Errorf("account balance %q: %w", resp.TotalMarginBalance, err)
	}

	out := ms.Account{EquityUSD: equity, Positions: map[string]ms.Position{}}
	for _, p := range resp.Positions {
		qty, err := strconv.ParseFloat(p.PositionAmt, 64)
		if err != nil {
			return ms.Account{}, fmt.Errorf("position %s: %w", p.Symbol, err)
		}
		if qty == 0 {
			// Binance lists every symbol; carrying the flat ones through would
			// make the account look enormous and mean nothing.
			continue
		}
		entry, _ := strconv.ParseFloat(p.EntryPrice, 64)
		out.Positions[p.Symbol] = ms.Position{Symbol: p.Symbol, Quantity: qty, Entry: entry}
	}
	return out, nil
}

// Rules reports the venue's constraints, cached after the first read.
func (b *Broker) Rules(ctx context.Context, symbol string) (ms.SymbolRules, error) {
	b.mu.Lock()
	if r, ok := b.rules[symbol]; ok {
		b.mu.Unlock()
		return r, nil
	}
	b.mu.Unlock()

	body, err := b.get(ctx, "/fapi/v1/exchangeInfo", nil, false)
	if err != nil {
		return ms.SymbolRules{}, err
	}
	var resp struct {
		Symbols []struct {
			Symbol  string `json:"symbol"`
			Filters []struct {
				FilterType string `json:"filterType"`
				StepSize   string `json:"stepSize"`
				MinQty     string `json:"minQty"`
				Notional   string `json:"notional"`
			} `json:"filters"`
		} `json:"symbols"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ms.SymbolRules{}, fmt.Errorf("exchangeInfo: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range resp.Symbols {
		var r ms.SymbolRules
		for _, f := range s.Filters {
			switch f.FilterType {
			case "LOT_SIZE", "MARKET_LOT_SIZE":
				if v, err := strconv.ParseFloat(f.StepSize, 64); err == nil && v > 0 {
					r.LotStep = v
				}
				if v, err := strconv.ParseFloat(f.MinQty, 64); err == nil && v > 0 {
					r.MinQuantity = v
				}
			case "MIN_NOTIONAL":
				if v, err := strconv.ParseFloat(f.Notional, 64); err == nil {
					r.MinNotionalUSD = v
				}
			}
		}
		b.rules[s.Symbol] = r
	}
	r, ok := b.rules[symbol]
	if !ok {
		return ms.SymbolRules{}, fmt.Errorf("%s is not listed on this venue", symbol)
	}
	return r, nil
}

// Place submits a market order.
//
// The order's ClientID is passed as newClientOrderId, which is Binance's own
// duplicate rejection. That is what turns the engine's "never resend after an
// ambiguous outcome" rule from an intention into something the venue enforces.
func (b *Broker) Place(ctx context.Context, o ms.Order) (ms.Fill, error) {
	params := url.Values{}
	params.Set("symbol", o.Symbol)
	params.Set("side", strings.ToUpper(string(o.Side)))
	params.Set("type", "MARKET")
	params.Set("quantity", strconv.FormatFloat(o.Quantity, 'f', -1, 64))
	params.Set("newClientOrderId", o.ClientID)
	if o.ReduceOnly {
		params.Set("reduceOnly", "true")
	}

	body, err := b.post(ctx, "/fapi/v1/order", params)
	if err != nil {
		return ms.Fill{}, err
	}
	var resp struct {
		ClientOrderID string `json:"clientOrderId"`
		ExecutedQty   string `json:"executedQty"`
		AvgPrice      string `json:"avgPrice"`
		CumQuote      string `json:"cumQuote"`
		Status        string `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ms.Fill{}, fmt.Errorf("order response: %w", err)
	}

	qty, err := strconv.ParseFloat(resp.ExecutedQty, 64)
	if err != nil {
		return ms.Fill{}, fmt.Errorf("executed quantity %q: %w", resp.ExecutedQty, err)
	}
	price, _ := strconv.ParseFloat(resp.AvgPrice, 64)

	// A market order that filled nothing is not a success. Returning it as one
	// would let the engine record a position it does not hold, which its next
	// reconciliation would correctly halt on — but the halt would blame the
	// wrong thing.
	if qty == 0 {
		return ms.Fill{}, fmt.Errorf("order %s returned status %q with nothing filled",
			resp.ClientOrderID, resp.Status)
	}
	return ms.Fill{ClientID: resp.ClientOrderID, Quantity: qty, Price: price}, nil
}

// ── signing ──────────────────────────────────────────────────────────────

func (b *Broker) sign(params url.Values) string {
	b.mu.Lock()
	delta := b.timeDelta
	b.mu.Unlock()

	params.Set("timestamp", strconv.FormatInt(time.Now().Add(delta).UnixMilli(), 10))
	params.Set("recvWindow", strconv.FormatInt(b.recvWindow.Milliseconds(), 10))

	// Encode() sorts keys, and the signature must cover exactly the bytes that
	// are sent — signing a differently ordered string is the classic way to
	// get an inscrutable -1022 from every request.
	query := params.Encode()
	mac := hmac.New(sha256.New, []byte(b.secret))
	mac.Write([]byte(query))
	return query + "&signature=" + hex.EncodeToString(mac.Sum(nil))
}

func (b *Broker) get(ctx context.Context, path string, params url.Values, signed bool) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	query := params.Encode()
	if signed {
		query = b.sign(params)
	}
	u := b.base + path
	if query != "" {
		u += "?" + query
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return b.do(req, signed)
}

func (b *Broker) post(ctx context.Context, path string, params url.Values) ([]byte, error) {
	body := b.sign(params)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+path,
		strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return b.do(req, true)
}

func (b *Broker) do(req *http.Request, signed bool) ([]byte, error) {
	if signed {
		req.Header.Set("X-MBX-APIKEY", b.key)
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		// The URL is deliberately absent: a signed request carries the
		// signature and the timestamp in its query string, and an error line
		// is exactly the text that ends up in an issue tracker. Binance's own
		// code and message say what went wrong without any of that.
		var e struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal(body, &e) == nil && e.Msg != "" {
			return nil, fmt.Errorf("%s returned %d: %s (code %d)",
				req.URL.Path, resp.StatusCode, e.Msg, e.Code)
		}
		return nil, fmt.Errorf("%s returned %d", req.URL.Path, resp.StatusCode)
	}
	return body, nil
}
