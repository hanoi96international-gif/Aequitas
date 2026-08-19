package mpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTPTransport is the deployment transport: the two validators, each holding
// one row of every template, exchanging blinded values over the network.
//
// This is the file that makes the privacy claim real. Everywhere else in this
// package the shares happen to sit in one process, and a process holding every
// share can reconstruct every biometric. Here party 0 runs on one box and party
// 1 on another, and neither can reconstruct anything alone. That is the whole
// point, and it is why the local transport is documented as a test fixture
// rather than as an option.
//
// WHAT AN ATTACKER ON THE WIRE CAN AND CANNOT DO
//
// Cannot: learn a template. Only Beaver-blinded differences travel, and they
// are uniformly random and independent of the inputs (see transport.go).
//
// Can, if unauthenticated: inject values, and thereby steer a comparison into
// saying "no match" for someone already registered. That is a Sybil hole rather
// than a privacy hole, and it is why every round is signed by the party that
// produced it (auth.go) and why plaintext HTTP is refused outside an explicit
// local harness. Integrity is what protects one-human-one-account here;
// confidentiality is the cheap part.
//
// Can, always: count rounds and measure payload sizes, and so learn how many
// candidates a registration was compared against. Batching reduces that to one
// figure per registration instead of a per-feature trace.

const (
	// maxExchangeBytes caps a single round's payload. A 512-bit sketch against
	// a few hundred candidates is a couple of megabytes, so the cap is generous
	// against real traffic while still bounded — an uncapped body is a way to
	// exhaust a validator's memory with one request.
	maxExchangeBytes = 64 << 20

	// ExchangePath is where a party receives its peers' contributions.
	ExchangePath = "/mpc/exchange"
)

// Mailbox receives peers' round contributions and hands them to the local
// computation. One per process, shared across sessions.
type Mailbox struct {
	mu      sync.Mutex
	parties int
	rounds  map[string]*localRound
	seen    map[string]time.Time
	ttl     time.Duration
}

// NewMailbox creates a mailbox for a fixed number of parties.
//
// ttl bounds how long an abandoned round occupies memory. Rounds are abandoned
// routinely — a registration that fails midway leaves its later rounds
// unclaimed — so without expiry this map only grows.
func NewMailbox(parties int, ttl time.Duration) (*Mailbox, error) {
	if parties < 2 {
		return nil, fmt.Errorf("mpc: %d parties cannot hide anything", parties)
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &Mailbox{
		parties: parties,
		rounds:  map[string]*localRound{},
		seen:    map[string]time.Time{},
		ttl:     ttl,
	}, nil
}

func mailboxKey(session string, round int) string {
	return session + "|" + strconv.Itoa(round)
}

// slot returns the round entry, creating it if a peer got here first.
func (m *Mailbox) slot(session string, round int) *localRound {
	key := mailboxKey(session, round)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	r, ok := m.rounds[key]
	if !ok {
		r = &localRound{
			slots: make([][]Element, m.parties),
			done:  make(chan struct{}),
		}
		m.rounds[key] = r
		m.seen[key] = time.Now()
	}
	return r
}

func (m *Mailbox) expireLocked() {
	cutoff := time.Now().Add(-m.ttl)
	for k, at := range m.seen {
		if at.Before(cutoff) {
			delete(m.seen, k)
			delete(m.rounds, k)
		}
	}
}

// Deliver records one party's contribution to one round.
func (m *Mailbox) Deliver(session string, round, party int, values []Element) error {
	if party < 0 || party >= m.parties {
		return fmt.Errorf("mpc: party index %d is outside 0..%d", party, m.parties-1)
	}
	r := m.slot(session, round)

	m.mu.Lock()
	defer m.mu.Unlock()
	if r.slots[party] != nil {
		return fmt.Errorf("mpc: party %d already contributed to round %d of session %q — a "+
			"repeat is either a retry whose response was lost or an attempt to overwrite a "+
			"contribution, and neither may silently replace the first", party, round, session)
	}
	r.slots[party] = values
	r.have++
	if r.have == m.parties {
		close(r.done)
	}
	return nil
}

// Await blocks until every party has contributed to this round.
func (m *Mailbox) Await(ctx context.Context, session string, round int) ([][]Element, error) {
	r := m.slot(session, round)
	select {
	case <-r.done:
		return r.slots, nil
	case <-ctx.Done():
		m.mu.Lock()
		var missing []int
		for i, s := range r.slots {
			if s == nil {
				missing = append(missing, i)
			}
		}
		m.mu.Unlock()
		return nil, fmt.Errorf("mpc: session %q round %d: no contribution from parties %v: %w",
			session, round, missing, ctx.Err())
	}
}

// Handler serves the endpoint peers post their contributions to.
//
// auth verifies that each contribution was signed by the party it claims to be
// from. There is no shared secret: a peer proves who it is with its own key, so
// no validator can speak for another and onboarding a new one never means
// handing anybody a secret.
func (m *Mailbox) Handler(auth Authenticator) (http.Handler, error) {
	if auth == nil {
		return nil, fmt.Errorf("mpc: refusing to serve the exchange endpoint without an " +
			"authenticator — an unauthenticated peer can steer a duplicate check into saying " +
			"'no match' and so mint a second account for the same person")
	}
	if auth.Parties() != m.parties {
		return nil, fmt.Errorf("mpc: the authenticator knows %d parties, the mailbox expects %d",
			auth.Parties(), m.parties)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}

		session := req.Header.Get("X-Mpc-Session")
		if session == "" || len(session) > 128 {
			http.Error(w, "bad session", http.StatusBadRequest)
			return
		}
		round, err := strconv.Atoi(req.Header.Get("X-Mpc-Round"))
		if err != nil || round < 0 {
			http.Error(w, "bad round", http.StatusBadRequest)
			return
		}
		party, err := strconv.Atoi(req.Header.Get("X-Mpc-Party"))
		if err != nil {
			http.Error(w, "bad party", http.StatusBadRequest)
			return
		}
		sig, err := hex.DecodeString(strings.TrimPrefix(req.Header.Get("X-Mpc-Signature"), "0x"))
		if err != nil || len(sig) == 0 {
			http.Error(w, "missing or malformed signature", http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxExchangeBytes))
		if err != nil {
			http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
			return
		}

		// Verify BEFORE decoding: an unauthenticated caller must not be able to
		// spend this node's CPU on parsing megabytes of attacker-chosen input.
		// The digest binds the payload to this session, round and party, so a
		// contribution cannot be replayed into a different one.
		if err := auth.VerifyParty(party, RoundDigest(session, round, party, body), sig); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		values, err := DecodeElements(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := m.Deliver(session, round, party, values); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}), nil
}

// EncodeElements packs field elements as big-endian uint64.
//
// Deliberately not JSON: a round carries hundreds of thousands of elements, and
// a decimal encoding would roughly triple the bytes on the wire for a
// readability nobody needs — these are uniform random values by construction.
func EncodeElements(vals []Element) []byte {
	buf := make([]byte, 8*len(vals))
	for i, v := range vals {
		binary.BigEndian.PutUint64(buf[8*i:], uint64(v))
	}
	return buf
}

// DecodeElements is the inverse, rejecting anything outside the field.
func DecodeElements(buf []byte) ([]Element, error) {
	if len(buf)%8 != 0 {
		return nil, fmt.Errorf("mpc: payload of %d bytes is not a whole number of field elements",
			len(buf))
	}
	out := make([]Element, len(buf)/8)
	for i := range out {
		v := binary.BigEndian.Uint64(buf[8*i:])
		if v >= Prime {
			return nil, fmt.Errorf("mpc: element %d is %d, outside the field — a peer running a "+
				"different field would produce silently wrong distances", i, v)
		}
		out[i] = Element(v)
	}
	return out, nil
}

// HTTPTransport implements Transport across the network.
type HTTPTransport struct {
	index   int
	parties int
	peers   []string // peer base URLs by party index; own entry unused
	session string
	auth    Authenticator
	mail    *Mailbox
	client  *http.Client
}

// HTTPConfig configures one party's view of one computation.
type HTTPConfig struct {
	Index   int      // which party this process is
	Peers   []string // base URL per party; the entry at Index is ignored
	Session string   // unique per registration; peers must agree on it
	Mailbox *Mailbox
	Client  *http.Client

	// Auth signs this party's contributions and verifies its peers'. Per-party
	// keys, never a shared secret: see auth.go for why that distinction
	// decides whether the validator set can grow.
	Auth Authenticator

	// AllowInsecure permits http:// peer URLs. For a local harness only; a
	// production deployment leaves this false so an unencrypted, forgeable
	// channel cannot be configured by accident.
	AllowInsecure bool
}

// NewHTTPTransport validates a configuration and returns the party's transport.
func NewHTTPTransport(cfg HTTPConfig) (*HTTPTransport, error) {
	parties := len(cfg.Peers)
	if parties < 2 {
		return nil, fmt.Errorf("mpc: %d parties cannot hide anything — with one party the "+
			"biometric is reconstructible by that party alone", parties)
	}
	if cfg.Index < 0 || cfg.Index >= parties {
		return nil, fmt.Errorf("mpc: index %d is outside 0..%d", cfg.Index, parties-1)
	}
	if cfg.Session == "" {
		return nil, fmt.Errorf("mpc: empty session id — two concurrent registrations would share " +
			"round numbers and read each other's contributions")
	}
	if cfg.Auth == nil {
		return nil, fmt.Errorf("mpc: no authenticator — peers would be unauthenticated, and " +
			"anyone able to reach the endpoint could decide that a returning person is new")
	}
	if cfg.Auth.Parties() != parties {
		return nil, fmt.Errorf("mpc: the authenticator knows %d parties but %d peers are "+
			"configured; a party with no key would be unverifiable",
			cfg.Auth.Parties(), parties)
	}
	if cfg.Mailbox == nil {
		return nil, fmt.Errorf("mpc: no mailbox — this party has nowhere to receive peers' values")
	}
	if cfg.Mailbox.parties != parties {
		return nil, fmt.Errorf("mpc: mailbox is built for %d parties, this computation has %d",
			cfg.Mailbox.parties, parties)
	}
	for i, p := range cfg.Peers {
		if i == cfg.Index {
			continue
		}
		if p == "" {
			return nil, fmt.Errorf("mpc: no address for party %d", i)
		}
		if !strings.HasPrefix(p, "https://") && !cfg.AllowInsecure {
			return nil, fmt.Errorf("mpc: peer %d is %q — plaintext HTTP lets anyone on the path "+
				"forge a contribution and decide that a returning person is new; set AllowInsecure "+
				"only in a local harness", i, p)
		}
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &HTTPTransport{
		index:   cfg.Index,
		parties: parties,
		peers:   append([]string(nil), cfg.Peers...),
		session: cfg.Session,
		auth:    cfg.Auth,
		mail:    cfg.Mailbox,
		client:  client,
	}, nil
}

// Index implements Transport.
func (t *HTTPTransport) Index() int { return t.index }

// Parties implements Transport.
func (t *HTTPTransport) Parties() int { return t.parties }

// Exchange implements Transport: publish to every peer, then wait for all.
func (t *HTTPTransport) Exchange(ctx context.Context, round int, mine []Element) ([][]Element, error) {
	// Deliver locally first, so a peer already waiting on us can proceed even
	// if our outbound calls are slow.
	if err := t.mail.Deliver(t.session, round, t.index, append([]Element(nil), mine...)); err != nil {
		return nil, err
	}

	body := EncodeElements(mine)
	errs := make([]error, t.parties)
	var wg sync.WaitGroup
	for i := range t.peers {
		if i == t.index {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = t.post(ctx, t.peers[i], round, body)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("mpc: sending round %d to party %d: %w", round, i, err)
		}
	}
	return t.mail.Await(ctx, t.session, round)
}

// post delivers one round to one peer, retrying transient failures.
//
// A dropped connection must not cost someone their registration, so retries
// exist; they are bounded because a peer that is genuinely down should surface
// as a failure quickly rather than holding a person in a spinner.
func (t *HTTPTransport) post(ctx context.Context, base string, round int, body []byte) error {
	url := strings.TrimRight(base, "/") + ExchangePath

	// One signature per round, not per element: 19 rounds per registration
	// makes the cost irrelevant, and the digest covers the whole payload.
	sig, err := t.auth.Sign(RoundDigest(t.session, round, t.index, body))
	if err != nil {
		return fmt.Errorf("signing round %d: %w", round, err)
	}
	sigHex := hex.EncodeToString(sig)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("X-Mpc-Signature", sigHex)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Mpc-Session", t.session)
		req.Header.Set("X-Mpc-Round", strconv.Itoa(round))
		req.Header.Set("X-Mpc-Party", strconv.Itoa(t.index))

		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusNoContent:
			return nil
		case resp.StatusCode == http.StatusConflict:
			// The peer already holds this contribution: an earlier attempt
			// arrived and only its response was lost. Retrying cannot help, and
			// the round is in fact delivered.
			return nil
		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("peer returned %d: %s",
				resp.StatusCode, strings.TrimSpace(string(msg)))
		default:
			// A 4xx other than conflict is our own fault; retrying repeats it.
			return fmt.Errorf("peer rejected the round with %d: %s",
				resp.StatusCode, strings.TrimSpace(string(msg)))
		}
	}
	return lastErr
}
