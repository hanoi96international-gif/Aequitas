package mpc

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
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
func twoValidators(t *testing.T, _ string) (a, b Transport, bytesOnWire *int64) {
	trs, wire := validators(t, 2, nil)
	return trs[0], trs[1], wire
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

// TestForeignKeyIsRejected: a signature from a key nobody listed must not be
// accepted. The endpoint decides who counts as registered.
func TestForeignKeyIsRejected(t *testing.T) {
	privs, pubs := newPartyKeys(t, 2)
	good, err := NewEd25519Authenticator(0, privs[0], pubs)
	if err != nil {
		t.Fatal(err)
	}

	mail, err := NewMailbox(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	h, err := mail.Handler(good)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(ExchangePath, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// An outsider with its own keypair, listed nowhere.
	outsiderPubs, outsiderPrivs := make([]ed25519.PublicKey, 2), make([]ed25519.PrivateKey, 2)
	for i := range outsiderPubs {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		outsiderPubs[i], outsiderPrivs[i] = pub, priv
	}
	outsider, err := NewEd25519Authenticator(0, outsiderPrivs[0], outsiderPubs)
	if err != nil {
		t.Fatal(err)
	}

	tr, err := NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: []string{srv.URL, srv.URL}, Session: "s",
		Mailbox: mail, Auth: outsider, AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := tr.Exchange(ctx, 0, []Element{1, 2, 3}); err == nil {
		t.Fatal("a peer signing with an unlisted key was accepted")
	}
}

// TestPartyCannotImpersonateAnother is the failure a shared token could not
// even express.
//
// With one secret held by everyone, any validator could submit a contribution
// as any other, and contributions decide who counts as a duplicate — so a
// validator could forge its peers' answers and register the same person twice.
// With per-party keys the forgery has to be signed by a key the forger does not
// have.
func TestPartyCannotImpersonateAnother(t *testing.T) {
	const n = 3
	privs, pubs := newPartyKeys(t, n)

	mail, err := NewMailbox(n, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519Authenticator(0, privs[0], pubs)
	if err != nil {
		t.Fatal(err)
	}
	h, err := mail.Handler(verifier)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(ExchangePath, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	body := EncodeElements([]Element{7, 8, 9})
	// Party 2 signs, but claims to be party 1.
	forged := ed25519.Sign(privs[2], RoundDigest("s", 0, 1, body))

	req, err := http.NewRequest(http.MethodPost, srv.URL+ExchangePath, bytesReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Mpc-Session", "s")
	req.Header.Set("X-Mpc-Round", "0")
	req.Header.Set("X-Mpc-Party", "1")
	req.Header.Set("X-Mpc-Signature", hex.EncodeToString(forged))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("party 2 spoke as party 1 and got %d, want 401 — one validator can forge "+
			"another's answer about whether someone is already registered", resp.StatusCode)
	}
}

// TestSignatureIsBoundToItsRound: a contribution captured once must not be
// replayable into a different round, session or party slot.
func TestSignatureIsBoundToItsRound(t *testing.T) {
	body := EncodeElements([]Element{1, 2, 3})
	base := RoundDigest("session-a", 3, 0, body)

	for _, tc := range []struct {
		name   string
		digest []byte
	}{
		{"different round", RoundDigest("session-a", 4, 0, body)},
		{"different session", RoundDigest("session-b", 3, 0, body)},
		{"different party", RoundDigest("session-a", 3, 1, body)},
		{"different payload", RoundDigest("session-a", 3, 0, EncodeElements([]Element{1, 2, 4}))},
	} {
		if string(tc.digest) == string(base) {
			t.Errorf("the digest is identical for %s — a signature could be replayed there", tc.name)
		}
	}

	// And the length prefixing must stop "a"+round 11 colliding with "a1"+round 1.
	if string(RoundDigest("a", 11, 0, nil)) == string(RoundDigest("a1", 1, 0, nil)) {
		t.Error("session and round are not unambiguously separated in the digest")
	}
}

func TestHandlerRefusesToServeWithoutAuth(t *testing.T) {
	mail, err := NewMailbox(2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mail.Handler(nil); err == nil {
		t.Error("a handler with no authenticator was served — every peer would be trusted")
	}
}

func TestAuthenticatorMustCoverEveryParty(t *testing.T) {
	privs, pubs := newPartyKeys(t, 2)
	twoParty, err := NewEd25519Authenticator(0, privs[0], pubs)
	if err != nil {
		t.Fatal(err)
	}
	mail, _ := NewMailbox(2, time.Minute)
	_, err = NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: []string{"https://a", "https://b", "https://c"}, Session: "s",
		Mailbox: mail, Auth: twoParty,
	})
	if err == nil {
		t.Error("three peers were accepted with keys for only two — the third would be unverifiable")
	}
}

func TestMismatchedPrivateKeyIsRefused(t *testing.T) {
	privs, pubs := newPartyKeys(t, 2)
	// Party 0 holding party 1's private key: every contribution it signs would
	// be rejected, and it should learn that at startup rather than mid-match.
	if _, err := NewEd25519Authenticator(0, privs[1], pubs); err == nil {
		t.Error("a private key that does not match the listed public key was accepted")
	}
}

// TestPlaintextPeerIsRefused: the wire carries no templates, but it does carry
// the values that decide whether someone counts as new.
func TestPlaintextPeerIsRefused(t *testing.T) {
	privs, pubs := newPartyKeys(t, 2)
	auth, err := NewEd25519Authenticator(0, privs[0], pubs)
	if err != nil {
		t.Fatal(err)
	}
	mail, _ := NewMailbox(2, time.Minute)
	_, err = NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: []string{"http://a", "http://b"}, Session: "s",
		Mailbox: mail, Auth: auth,
	})
	if err == nil {
		t.Error("a plaintext http:// peer was accepted without AllowInsecure")
	}
}

func TestSinglePartyConfigurationIsRefused(t *testing.T) {
	privs, pubs := newPartyKeys(t, 2)
	auth, err := NewEd25519Authenticator(0, privs[0], pubs)
	if err != nil {
		t.Fatal(err)
	}
	mail, _ := NewMailbox(2, time.Minute)
	_, err = NewHTTPTransport(HTTPConfig{
		Index: 0, Peers: []string{"https://only-me"}, Session: "s", Mailbox: mail, Auth: auth,
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

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// TestForgeryFailsWithoutTLS is the claim that decides whether the transport
// may run over plain HTTP between parties.
//
// The https requirement was written when a SHARED TOKEN was the only
// protection: anyone who could inject on the path and had the token could forge
// a peer's contribution, and contributions decide whether someone counts as
// already registered. Per-round signatures replaced that token, and the
// requirement was never revisited.
//
// This exercises what an attacker with full control of a plaintext path can
// actually do: inject a chosen contribution, replay a captured one into another
// round, and submit under another party's identity. Each must fail on the
// signature, not on the channel.
func TestForgeryFailsWithoutTLS(t *testing.T) {
	const n = 2
	privs, pubs := newPartyKeys(t, n)
	mail, err := NewMailbox(n, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewEd25519Authenticator(0, privs[0], pubs)
	if err != nil {
		t.Fatal(err)
	}
	h, err := mail.Handler(verifier)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(ExchangePath, h)
	srv := httptest.NewServer(mux) // plain http, exactly what an attacker would want
	defer srv.Close()

	post := func(session string, round, party int, body []byte, sig []byte) int {
		req, err := http.NewRequest(http.MethodPost, srv.URL+ExchangePath, bytesReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Mpc-Session", session)
		req.Header.Set("X-Mpc-Round", fmt.Sprint(round))
		req.Header.Set("X-Mpc-Party", fmt.Sprint(party))
		req.Header.Set("X-Mpc-Signature", hex.EncodeToString(sig))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	genuine := EncodeElements([]Element{1, 2, 3})
	genuineSig := ed25519.Sign(privs[1], RoundDigest("s", 0, 1, genuine))

	t.Run("chosen values with no valid signature", func(t *testing.T) {
		evil := EncodeElements([]Element{99, 99, 99})
		if code := post("s", 0, 1, evil, []byte("not a signature")); code != http.StatusUnauthorized {
			t.Errorf("injected values got %d, want 401 — the channel is plaintext, so only the "+
				"signature stands between an attacker and deciding who is a duplicate", code)
		}
	})

	t.Run("genuine signature over swapped values", func(t *testing.T) {
		evil := EncodeElements([]Element{99, 99, 99})
		if code := post("s", 0, 1, evil, genuineSig); code != http.StatusUnauthorized {
			t.Errorf("a real signature reused over different values got %d, want 401", code)
		}
	})

	t.Run("captured contribution replayed into another round", func(t *testing.T) {
		if code := post("s", 7, 1, genuine, genuineSig); code != http.StatusUnauthorized {
			t.Errorf("a round-0 contribution replayed as round 7 got %d, want 401 — the digest "+
				"binds the round precisely so a recorded exchange cannot be reused", code)
		}
	})

	t.Run("submitted under another party's identity", func(t *testing.T) {
		if code := post("s", 0, 0, genuine, genuineSig); code != http.StatusUnauthorized {
			t.Errorf("party 1's contribution submitted as party 0 got %d, want 401", code)
		}
	})

	t.Run("the genuine one still works", func(t *testing.T) {
		if code := post("s", 0, 1, genuine, genuineSig); code != http.StatusNoContent {
			t.Fatalf("the real contribution got %d, want 204 — a check that rejects everything "+
				"proves nothing", code)
		}
	})
}
