package mpc

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// validators stands up n independent HTTP servers, one per party.
//
// The generalisation of twoValidators. Two parties is the minimum that hides
// anything and provides no margin: both shares together reconstruct every
// template, so with n=2 the operators can collude. A third independently
// operated party is the single largest available improvement, and this exists
// so that path is exercised rather than merely asserted to be possible.
// newPartyKeys makes one ed25519 keypair per party. Per-party keys, not a
// shared secret: that difference is what lets the validator set grow (auth.go).
func newPartyKeys(t *testing.T, n int) ([]ed25519.PrivateKey, []ed25519.PublicKey) {
	t.Helper()
	privs := make([]ed25519.PrivateKey, n)
	pubs := make([]ed25519.PublicKey, n)
	for i := 0; i < n; i++ {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			t.Fatal(err)
		}
		privs[i], pubs[i] = priv, pub
	}
	return privs, pubs
}

// validators stands up n independent HTTP servers, one per party.
//
// Two parties is the minimum that hides anything and provides no margin: both
// shares together reconstruct every template, so with n=2 the operators can
// collude. A third independently operated party is the largest available
// improvement, and this exists so that path is exercised rather than merely
// asserted to be possible.
//
// If auths is nil, fresh ed25519 keys are generated. Passing them lets a test
// hand one party the wrong key on purpose.
func validators(t *testing.T, n int, auths []Authenticator) ([]Transport, *int64) {
	t.Helper()

	if auths == nil {
		privs, pubs := newPartyKeys(t, n)
		auths = make([]Authenticator, n)
		for i := 0; i < n; i++ {
			a, err := NewEd25519Authenticator(i, privs[i], pubs)
			if err != nil {
				t.Fatal(err)
			}
			auths[i] = a
		}
	}

	mailboxes := make([]*Mailbox, n)
	var counted int64
	count := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&counted, r.ContentLength)
			next.ServeHTTP(w, r)
		})
	}

	peers := make([]string, n)
	for i := 0; i < n; i++ {
		m, err := NewMailbox(n, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		mailboxes[i] = m
		h, err := m.Handler(auths[i])
		if err != nil {
			t.Fatal(err)
		}
		mux := http.NewServeMux()
		mux.Handle(ExchangePath, count(h))
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		peers[i] = srv.URL
	}

	transports := make([]Transport, n)
	for i := 0; i < n; i++ {
		tr, err := NewHTTPTransport(HTTPConfig{
			Index: i, Peers: peers, Session: "n-party-session",
			Mailbox: mailboxes[i], Auth: auths[i], AllowInsecure: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		transports[i] = tr
	}
	return transports, &counted
}

// TestThirdPartyWorksToday is the answer to "what would adding a third
// operator take": nothing in the protocol, which this proves by running it.
//
// With three parties no two shares reconstruct anything, so the collusion that
// two parties cannot defend against requires all three. The remaining work is
// operational — finding an independent operator — not cryptographic.
func TestThirdPartyWorksToday(t *testing.T) {
	const length = 32
	for _, parties := range []int{2, 3, 4} {
		t.Run(fmt.Sprintf("%d-parties", parties), func(t *testing.T) {
			transports, wire := validators(t, parties, nil)

			cand := make([]uint8, length)
			for i := range cand {
				cand[i] = uint8(i % 3 % 2)
			}
			same := make([]uint8, length)
			copy(same, cand)
			same[7] ^= 1
			stranger := make([]uint8, length)
			for i := range stranger {
				stranger[i] = uint8((i * 5) % 2)
			}
			enrolled := [][]uint8{same, stranger}

			candRows, err := SplitTemplateForParties(cand, parties)
			if err != nil {
				t.Fatal(err)
			}
			rows := make([][]PartyTemplate, len(enrolled))
			for i, e := range enrolled {
				if rows[i], err = SplitTemplateForParties(e, parties); err != nil {
					t.Fatal(err)
				}
			}
			triples, err := GenerateTriples(TriplesForManyComparison(length, len(enrolled))+4096, parties)
			if err != nil {
				t.Fatal(err)
			}

			results := make([][]MatchResult, parties)
			errs := make([]error, parties)
			var wg sync.WaitGroup
			for p := 0; p < parties; p++ {
				wg.Add(1)
				go func(p int) {
					defer wg.Done()
					mine := make([]PartyTemplate, len(rows))
					for i := range rows {
						mine[i] = rows[i][p]
					}
					m := &DistributedMatcher{
						Session:   NewSession(transports[p], NewTripleStore(triples[p])),
						Threshold: 4,
					}
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					results[p], errs[p] = m.CompareMany(ctx, candRows[p], mine)
				}(p)
			}
			wg.Wait()

			for p, err := range errs {
				if err != nil {
					t.Fatalf("party %d: %v", p, err)
				}
			}
			for p := 1; p < parties; p++ {
				for k := range results[0] {
					if results[p][k].Similar != results[0][k].Similar {
						t.Fatalf("party %d disagrees with party 0 on candidate %d", p, k)
					}
				}
			}
			if !results[0][0].Similar {
				t.Error("the returning person was not recognised")
			}
			if results[0][1].Similar {
				t.Error("a stranger was flagged as already registered")
			}
			t.Logf("%d parties: correct decisions, %d bytes on the wire",
				parties, atomic.LoadInt64(wire))
		})
	}
}

// TestVerificationWorksAcrossTheNetwork: triple verification has to hold over
// the real transport too, not only in the in-memory harness.
func TestVerificationWorksAcrossTheNetwork(t *testing.T) {
	const parties, count = 3, 32
	transports, _ := validators(t, parties, nil)

	triples, err := GenerateTriples(2*count, parties)
	if err != nil {
		t.Fatal(err)
	}
	triples[2][9].C = Add(triples[2][9].C, 3) // one forged triple, on one party

	errs := make([]error, parties)
	var wg sync.WaitGroup
	for p := 0; p < parties; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			s := NewSession(transports[p], NewTripleStore(nil))
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			errs[p] = s.SacrificeVerify(ctx, triples[p][:count], triples[p][count:2*count])
		}(p)
	}
	wg.Wait()

	for p, err := range errs {
		if err == nil {
			t.Errorf("party %d accepted a forged triple over the network", p)
		}
	}
}

// TestDealerDistributedTriplesMakeAComparisonWork is the end-to-end proof that
// was missing while the node generated its own triples.
//
// Triples take the whole path a deployment puts them through: generated once,
// encoded to the distribution format, written out per party, read back
// independently, and used by parties that reach each other only over HTTP. If
// the correlation survives all of that, the comparison returns the answer the
// plaintext distance implies. If it does not, CompareMany refuses — which is
// exactly what the broken local-generation path did.
func TestDealerDistributedTriplesMakeAComparisonWork(t *testing.T) {
	const length, parties, threshold = 32, 2, 4

	// The dealer, once.
	dealt, err := GenerateTriples(TriplesForManyComparison(length, 2)+4096, parties)
	if err != nil {
		t.Fatal(err)
	}
	// Through the file format, one blob per party.
	blobs := make([][]byte, parties)
	for i := range dealt {
		blobs[i] = EncodeTriples(dealt[i])
	}

	transports, _ := validators(t, parties, nil)

	cand := make([]uint8, length)
	for i := range cand {
		cand[i] = uint8(i % 2)
	}
	same := make([]uint8, length)
	copy(same, cand)
	same[5] ^= 1 // a returning person
	// A genuinely different person. Note (i*7)%2 == i%2 — an earlier version of
	// this line produced a "stranger" byte-identical to the candidate, and the
	// test duly reported a false accusation that was actually a correct match.
	stranger := make([]uint8, length)
	copy(stranger, cand)
	for i := 0; i < length/2; i++ {
		stranger[i] ^= 1 // half the bits flipped: far past any sane threshold
	}
	enrolled := [][]uint8{same, stranger}

	candRows, err := SplitTemplateForParties(cand, parties)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([][]PartyTemplate, len(enrolled))
	for i, e := range enrolled {
		if rows[i], err = SplitTemplateForParties(e, parties); err != nil {
			t.Fatal(err)
		}
	}

	results := make([][]MatchResult, parties)
	errs := make([]error, parties)
	var wg sync.WaitGroup
	for p := 0; p < parties; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			// Each party reads ONLY its own blob, the way it reads its own file.
			mine, err := DecodeTriples(blobs[p])
			if err != nil {
				errs[p] = err
				return
			}
			enrol := make([]PartyTemplate, len(rows))
			for i := range rows {
				enrol[i] = rows[i][p]
			}
			m := &DistributedMatcher{
				Session:   NewSession(transports[p], NewTripleStore(mine)),
				Threshold: threshold,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			results[p], errs[p] = m.CompareMany(ctx, candRows[p], enrol)
		}(p)
	}
	wg.Wait()

	for p, err := range errs {
		if err != nil {
			t.Fatalf("party %d: %v — dealer-distributed triples did not survive the round trip", p, err)
		}
	}
	if results[0][0].Similar != results[1][0].Similar {
		t.Fatal("the parties disagreed")
	}
	if !results[0][0].Similar {
		t.Error("the returning person was not recognised")
	}
	if results[0][1].Similar {
		t.Error("a stranger was flagged as already registered")
	}
}
