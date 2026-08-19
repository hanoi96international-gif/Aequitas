package mpc

import (
	"context"
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
func validators(t *testing.T, n int, token string) ([]Transport, *int64) {
	t.Helper()

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
		h, err := m.Handler(token)
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
			Index: i, Peers: peers, Session: "n-party-session", Token: token,
			Mailbox: mailboxes[i], AllowInsecure: true,
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
			transports, wire := validators(t, parties, "three-party-token")

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
	transports, _ := validators(t, parties, "verify-over-http-token")

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
