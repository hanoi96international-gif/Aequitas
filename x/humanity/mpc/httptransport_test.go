package mpc

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// twoValidators stands up two independent HTTP servers, one per party, and
// returns a transport for each.
//
// This is as close to the deployment as a test can get in one process: the
// parties reach each other only through the network, so anything that would
// require reading a peer's memory fails here rather than in production.
func twoValidators(t *testing.T, token string) (a, b Transport, bytesOnWire *int64) {
	t.Helper()

	mailA, err := NewMailbox(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	mailB, err := NewMailbox(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	hA, err := mailA.Handler(token)
	if err != nil {
		t.Fatal(err)
	}
	hB, err := mailB.Handler(token)
	if err != nil {
		t.Fatal(err)
	}

	var counted int64
	count := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counted, r.ContentLength)
			next.ServeHTTP(w, r)
		})
	}

	muxA := http.NewServeMux()
	muxA.Handle(ExchangePath, count(hA))
	muxB := http.NewServeMux()
	muxB.Handle(ExchangePath, count(hB))

	srvA := httptest.NewServer(muxA)
	srvB := httptest.NewServer(muxB)
	t.Cleanup(srvA.Close)
	t.Cleanup(srvB.Close)

	peers := []string{srvA.URL, srvB.URL}
	ta, err := NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: peers, Session: "test-session", Token: token,
		Mailbox: mailA, AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	tb, err := NewHTTPTransport(HTTPConfig{
		Index: 1, Peers: peers, Session: "test-session", Token: token,
		Mailbox: mailB, AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ta, tb, &counted
}

// TestTwoValidatorsMatchOverHTTP is the deployment claim end to end: two
// separate servers, neither holding a whole template, reaching the decision the
// plaintext distance implies.
func TestTwoValidatorsMatchOverHTTP(t *testing.T) {
	const length = 32
	ta, tb, wire := twoValidators(t, "shared-peer-token")

	rng := rand.New(rand.NewSource(77))
	cand := make([]uint8, length)
	for i := range cand {
		cand[i] = uint8(rng.Intn(2))
	}
	same := make([]uint8, length)
	copy(same, cand)
	same[3] ^= 1 // a returning person: one bit of capture noise
	stranger := make([]uint8, length)
	for i := range stranger {
		stranger[i] = uint8(rng.Intn(2))
	}
	enrolled := [][]uint8{same, stranger}

	candRows, err := SplitTemplateForParties(cand, 2)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([][]PartyTemplate, len(enrolled))
	for i, e := range enrolled {
		if rows[i], err = SplitTemplateForParties(e, 2); err != nil {
			t.Fatal(err)
		}
	}

	triples, err := GenerateTriples(TriplesForManyComparison(length, len(enrolled))+4096, 2)
	if err != nil {
		t.Fatal(err)
	}

	const threshold = 4
	transports := []Transport{ta, tb}
	results := make([][]MatchResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for p := 0; p < 2; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			mine := make([]PartyTemplate, len(rows))
			for i := range rows {
				mine[i] = rows[i][p]
			}
			m := &DistributedMatcher{
				Session:   NewSession(transports[p], NewTripleStore(triples[p])),
				Threshold: threshold,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			results[p], errs[p] = m.CompareMany(ctx, candRows[p], mine)
		}(p)
	}
	wg.Wait()

	for p, err := range errs {
		if err != nil {
			t.Fatalf("validator %d: %v", p, err)
		}
	}
	for k := range results[0] {
		if results[0][k].Similar != results[1][k].Similar {
			t.Fatalf("the two validators disagree on candidate %d: %v vs %v — a split decision "+
				"about whether someone is already registered is the worst possible outcome",
				k, results[0][k].Similar, results[1][k].Similar)
		}
	}

	if !results[0][0].Similar {
		t.Error("the returning person (1 bit of noise, threshold 4) was not recognised — " +
			"they would register a second time")
	}
	if results[0][1].Similar {
		t.Error("a stranger was flagged as already registered — they would be locked out")
	}
	t.Logf("bytes crossing the network for %d candidates x %d features: %d",
		len(enrolled), length, atomic.LoadInt64(wire))
}

// TestWrongTokenIsRejected: the endpoint decides who is registered. An
// unauthenticated peer could force "no match" and mint a second account.
func TestWrongTokenIsRejected(t *testing.T) {
	mail, err := NewMailbox(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	h, err := mail.Handler("the-real-token")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(ExchangePath, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tr, err := NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: []string{srv.URL, srv.URL}, Session: "s", Token: "the-wrong-token",
		Mailbox: mail, AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := tr.Exchange(ctx, 0, []Element{1, 2, 3}); err == nil {
		t.Fatal("a peer with the wrong token was accepted")
	}
}

func TestHandlerRefusesToServeWithoutAToken(t *testing.T) {
	mail, err := NewMailbox(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mail.Handler(""); err == nil {
		t.Error("an empty token must be refused — it authenticates everyone while looking protected")
	}
}

// TestPlaintextPeerIsRefused: the wire carries no templates, but it does carry
// the values that decide whether someone counts as new.
func TestPlaintextPeerIsRefused(t *testing.T) {
	mail, _ := NewMailbox(2, time.Minute)
	_, err := NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: []string{"http://a", "http://b"}, Session: "s", Token: "tok",
		Mailbox: mail,
	})
	if err == nil {
		t.Error("a plaintext http:// peer was accepted without AllowInsecure")
	}
}

func TestSinglePartyConfigurationIsRefused(t *testing.T) {
	mail, _ := NewMailbox(2, time.Minute)
	_, err := NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: []string{"https://only-me"}, Session: "s", Token: "tok", Mailbox: mail,
	})
	if err == nil {
		t.Error("a one-party deployment was accepted — that party can reconstruct every template, " +
			"which is exactly what this package exists to prevent")
	}
}

// TestDuplicateContributionIsRejected: a second contribution to the same round
// must not overwrite the first, or a peer could revise its answer after seeing
// ours.
func TestDuplicateContributionIsRejected(t *testing.T) {
	mail, _ := NewMailbox(2, time.Minute)
	if err := mail.Deliver("s", 0, 1, []Element{5}); err != nil {
		t.Fatal(err)
	}
	if err := mail.Deliver("s", 0, 1, []Element{9}); err == nil {
		t.Fatal("a party was allowed to contribute to the same round twice")
	}
	// And the round is still incomplete: the rejected duplicate did not stand
	// in for the missing party. A short deadline, because a Background context
	// here would wait forever — which is precisely the failure this rejection
	// is meant to make visible.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := mail.Await(ctx, "s", 0); err == nil {
		t.Fatal("Await returned before every party contributed")
	}
}

// TestOutOfFieldValueIsRejected guards against a peer on a different field:
// silently wrong distances are worse than a refused registration.
func TestOutOfFieldValueIsRejected(t *testing.T) {
	buf := make([]byte, 8)
	for i := range buf {
		buf[i] = 0xFF // far above 2^61-1
	}
	if _, err := DecodeElements(buf); err == nil {
		t.Error("a value outside the field was accepted")
	}
	if _, err := DecodeElements([]byte{1, 2, 3}); err == nil {
		t.Error("a payload that is not a whole number of elements was accepted")
	}
}

func TestElementEncodingRoundTrips(t *testing.T) {
	want := []Element{0, 1, 2, Element(Prime - 1), 123456789}
	got, err := DecodeElements(EncodeElements(want))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d elements, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("element %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

// TestAwaitGivesUpRatherThanHanging: if the other validator is down, the
// person in front of the phone must get an answer, not an endless spinner.
func TestAwaitGivesUpRatherThanHanging(t *testing.T) {
	mail, _ := NewMailbox(2, time.Minute)
	if err := mail.Deliver("s", 0, 0, []Element{1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := mail.Await(ctx, "s", 0)
	if err == nil {
		t.Fatal("Await succeeded with only one of two parties")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Await took %v to give up", elapsed)
	}
	t.Logf("gave up as expected: %v", err)
}

// TestAbandonedRoundsDoNotAccumulate: failed registrations are normal, and
// their unclaimed rounds must not pin memory for the life of the process.
func TestAbandonedRoundsDoNotAccumulate(t *testing.T) {
	mail, err := NewMailbox(2, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if err := mail.Deliver(fmt.Sprintf("abandoned-%d", i), 0, 0, []Element{1}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(60 * time.Millisecond)
	// Any operation sweeps expired entries.
	_ = mail.slot("trigger-sweep", 0)

	mail.mu.Lock()
	remaining := len(mail.rounds)
	mail.mu.Unlock()
	if remaining > 5 {
		t.Errorf("%d abandoned rounds still held after expiry — the mailbox grows without bound "+
			"under normal failure traffic", remaining)
	}
}
