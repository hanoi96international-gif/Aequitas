package keeper

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/mpc"
)

// The endpoints that connect a capture to the duplicate check, and the gate
// that makes the answer count.
//
// # THE DIRECTION OF FAILURE
//
// Two errors are possible and they are not symmetric.
//
// Approving someone who is already registered mints a second account for one
// person. It dilutes everyone's share, and by the time it is noticed the funds
// have moved. It is effectively irreversible.
//
// Refusing this attempt costs the person a retry. It is annoying and it is
// completely reversible.
//
// So when the check cannot be performed — a peer is down, triples are
// exhausted, the committee cannot be formed — this refuses the attempt and says
// to try again. It never approves by default, and it never records a permanent
// rejection either: nobody gets locked out because a validator was rebooting.
//
// # WHY THE VERDICT IS NOT SOMETHING THE CLIENT TELLS US
//
// The comparison happens before registration, so registration has to know how
// it went. Taking the client's word for it would make the whole subsystem
// decorative — anyone could post "not a duplicate". The verdict is recorded
// here, keyed by a session the client cannot choose the outcome of, consumed
// once, and expires quickly.

// mpcVerdict is one completed check, waiting to be spent by a registration.
type mpcVerdict struct {
	duplicate bool
	compared  int
	committee string
	at        time.Time
}

var (
	mpcVerdictMu sync.Mutex
	mpcVerdicts  = map[string]mpcVerdict{}
)

// mpcVerdictTTL bounds how long a check stays spendable.
//
// Short, because a stale pass is a pass against an older enrolment set: someone
// who registered in between would not have been compared. Long enough that a
// person finishing a normal registration flow does not have to redo it.
const mpcVerdictTTL = 10 * time.Minute

func recordMPCVerdict(session string, v mpcVerdict) {
	mpcVerdictMu.Lock()
	defer mpcVerdictMu.Unlock()
	cutoff := time.Now().Add(-mpcVerdictTTL)
	for k, old := range mpcVerdicts {
		if old.at.Before(cutoff) {
			delete(mpcVerdicts, k)
		}
	}
	mpcVerdicts[session] = v
}

// consumeMPCVerdict takes a verdict, once.
//
// Single use: a "not a duplicate" answer is about one capture at one moment,
// and letting it authorise several registrations would turn one honest check
// into a supply of accounts.
func consumeMPCVerdict(session string) (mpcVerdict, bool) {
	mpcVerdictMu.Lock()
	defer mpcVerdictMu.Unlock()
	v, ok := mpcVerdicts[session]
	if !ok {
		return mpcVerdict{}, false
	}
	delete(mpcVerdicts, session)
	if time.Since(v.at) > mpcVerdictTTL {
		return mpcVerdict{}, false
	}
	return v, true
}

// mpcClientAuthorized checks the service credential of the capture pipeline.
//
// A shared token is right HERE and was wrong for peers, and the difference is
// worth stating because the obvious "make it consistent" change would undo it.
// Between validators a shared secret lets any party impersonate any other, and
// those parties vote on who is a duplicate. This is a service credential for
// one caller, which impersonates nobody: the worst it grants is the ability to
// submit captures, which is what the caller is for.
func mpcClientAuthorized(r *http.Request) bool {
	want := os.Getenv("MPC_CLIENT_TOKEN")
	if want == "" {
		return false
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" {
		got = r.Header.Get("X-Mpc-Client-Token")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

type mpcSubmission struct {
	Session      string   `json:"session"`
	EnrollmentID string   `json:"enrollment_id"`
	Row          string   `json:"row"`  // hex, this party's row only
	Keys         []uint32 `json:"keys"` // one bucket key per LSH table
	Threshold    int      `json:"threshold"`
}

func (s *mpcSubmission) decodeRow() (mpc.PartyTemplate, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(s.Row, "0x"))
	if err != nil {
		return nil, fmt.Errorf("row is not hex: %w", err)
	}
	return decodeRow(raw)
}

func mpcJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": msg})
}

func readMPCSubmission(w http.ResponseWriter, r *http.Request) (*mpcSubmission, bool) {
	if r.Method != http.MethodPost {
		mpcJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return nil, false
	}
	if !mpcClientAuthorized(r) {
		mpcJSONError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		mpcJSONError(w, http.StatusRequestEntityTooLarge, "body too large")
		return nil, false
	}
	var sub mpcSubmission
	if err := json.Unmarshal(body, &sub); err != nil {
		mpcJSONError(w, http.StatusBadRequest, "invalid JSON")
		return nil, false
	}
	if len(sub.Keys) == 0 {
		mpcJSONError(w, http.StatusBadRequest,
			"no bucket keys: the capture would be stored and then never compared against anyone")
		return nil, false
	}
	return &sub, true
}

// handleMPCEnroll stores this party's row for a newly registered person.
func (a *APIServer) handleMPCEnroll(w http.ResponseWriter, r *http.Request) {
	sub, ok := readMPCSubmission(w, r)
	if !ok {
		return
	}
	if a.mpc == nil {
		mpcJSONError(w, http.StatusServiceUnavailable, "this node is not an MPC party")
		return
	}
	if sub.EnrollmentID == "" {
		mpcJSONError(w, http.StatusBadRequest, "enrollment_id is required")
		return
	}
	row, err := sub.decodeRow()
	if err != nil {
		mpcJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	committee, index, err := a.mpcCommittee()
	if err != nil {
		mpcJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := a.blockchain.state.SaveMPCShare(
		sub.EnrollmentID, committee.ID, index, row); err != nil {
		mpcJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stored":       true,
		"committee_id": committee.ID,
		"party_index":  index,
	})
}

// handleMPCCheck runs the duplicate comparison and records the verdict.
func (a *APIServer) handleMPCCheck(w http.ResponseWriter, r *http.Request) {
	sub, ok := readMPCSubmission(w, r)
	if !ok {
		return
	}
	if a.mpc == nil {
		mpcJSONError(w, http.StatusServiceUnavailable, "this node is not an MPC party")
		return
	}
	if sub.Session == "" {
		mpcJSONError(w, http.StatusBadRequest, "session is required")
		return
	}
	row, err := sub.decodeRow()
	if err != nil {
		mpcJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	committee, _, err := a.mpcCommittee()
	if err != nil {
		mpcJSONError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Gegen ALLE vergleichen, nicht gegen die Bucket-Vorauswahl: die filtert
	// bei dieser Schwelle den echten Treffer mit weg (0,05 % Trefferquote bei
	// d=165, gemessen am 24.08.2026). Begruendung und Zahlen in
	// MPCAllShares' eigenem Kommentar.
	ids, rows, err := a.blockchain.state.MPCAllShares(committee.ID, 0)
	if err != nil {
		mpcJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	duplicate := false
	if len(rows) > 0 {
		transport, err := a.mpc.TransportFor(sub.Session)
		if err != nil {
			mpcJSONError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		triples, err := a.mpcTriples(sub.Session, mpc.TriplesForManyComparison(len(row), len(rows)))
		if err != nil {
			mpcJSONError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		matcher := &mpc.DistributedMatcher{
			Session:   mpc.NewSession(transport, triples),
			Threshold: sub.Threshold,
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()

		results, err := matcher.CompareMany(ctx, row, rows)
		if err != nil {
			// Refuse the attempt, do not approve it. A comparison that could
			// not run says nothing about whether this person is already
			// registered, and guessing "no" is the irreversible mistake.
			mpcJSONError(w, http.StatusServiceUnavailable,
				"the duplicate check could not complete, please try again: "+err.Error())
			return
		}
		for i, res := range results {
			if res.Similar {
				duplicate = true
				fmt.Printf("[MPC] session %s matches enrolment %s\n", sub.Session, ids[i])
				break
			}
		}
	}

	recordMPCVerdict(sub.Session, mpcVerdict{
		duplicate: duplicate,
		compared:  len(rows),
		committee: committee.ID,
		at:        time.Now(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"duplicate":    duplicate,
		"compared":     len(rows),
		"committee_id": committee.ID,
	})
}

// mpcCommittee resolves the committee this node belongs to right now.
func (a *APIServer) mpcCommittee() (*mpc.Committee, int, error) {
	if a.mpc == nil {
		return nil, 0, fmt.Errorf("this node is not an MPC party")
	}
	if a.mpc.discover == nil {
		// Static configuration: the peer list IS the committee.
		parties := make([]mpc.Party, len(a.mpc.peers))
		for i, p := range a.mpc.peers {
			parties[i] = mpc.Party{URL: p, Address: strings.ToLower(a.mpc.auth.addrs[i].Hex())}
		}
		return &mpc.Committee{ID: "static", Parties: parties}, a.mpc.index, nil
	}
	c, err := a.mpc.committeeNow()
	if err != nil {
		return nil, 0, err
	}
	idx := c.IndexOf(a.mpc.selfAddr)
	if idx < 0 {
		return nil, 0, fmt.Errorf("this node is not in committee %s", c.ID)
	}
	return c, idx, nil
}

// mpcTriples hands out this party's share of pre-distributed triples.
//
// # WHY THIS DOES NOT GENERATE THEM
//
// A Beaver triple is a CORRELATED secret: party 0 holds a_0, party 1 holds a_1,
// and a_0+a_1 = a with c = a*b. They have to be produced once and their rows
// handed out. The first version of this function called GenerateTriples locally
// on every party, which gives each party a row from a different draw. Nothing
// objects — the shapes match — and the arithmetic silently stops meaning
// anything. TestIndependentlyGeneratedTriplesCannotWork pins it.
//
// It failed closed rather than lying (the opened verdict is then a random field
// element, and CompareMany refuses anything that is not 0 or 1), but it could
// never have worked. So triples are LOADED here and never made.
//
// # WHY THE OFFSET IS PERSISTED
//
// A triple may be used at most once. Reusing one stops the blinding and leaks
// the difference of the two secrets it was used on. Reloading the file from the
// start after a restart would do exactly that, silently, so the consumed offset
// is written to the config table before the triples are handed out — losing
// unused triples on a crash is the acceptable direction, reusing them is not.
func (a *APIServer) mpcTriples(session string, need int) (*mpc.TripleStore, error) {
	if need <= 0 {
		return mpc.NewTripleStore(nil), nil
	}
	path := strings.TrimSpace(os.Getenv("MPC_TRIPLE_FILE"))
	if path == "" {
		return nil, fmt.Errorf("MPC_TRIPLE_FILE is not set: this party has no multiplication " +
			"triples. They must be generated ONCE by a dealer that is not a computing party, " +
			"and each party's row delivered to it — a party that makes its own gets an " +
			"uncorrelated set and every comparison becomes noise")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mpc: reading %s: %w", path, err)
	}
	all, err := mpc.DecodeTriples(raw)
	if err != nil {
		return nil, err
	}

	// Verification doubles the requirement: half the supply is spent proving
	// the other half was not forged (mpc/sacrifice.go).
	want := mpc.TriplesForVerifiedWork(need)

	// One allocator decides the range, and both parties use it -- see
	// mpc_triple_sync.go. Per-party counters cannot stay equal (measured
	// 2026-08-24: 10240 against 4096) and a max-of-the-peers read is a race,
	// not a fix (measured: a 2048 gap survived it intact).
	offset, err := a.tripleRangeFor(session, want)
	if err != nil {
		return nil, err
	}
	if offset+want > len(all) {
		return nil, fmt.Errorf("mpc: %d triples left in %s, this comparison needs %d — refusing "+
			"to reuse, because a reused triple stops blinding and leaks the difference of the "+
			"secrets it was used on; have the dealer deliver more",
			len(all)-offset, path, want)
	}

	// Advance BEFORE handing them out, so a crash mid-comparison loses triples
	// rather than replaying them.
	// The counter was already advanced by the allocator, inside
	// allocateTripleRange, under its lock. Advancing again here would double
	// count and burn through a finite supply twice as fast.
	return mpc.NewTripleStore(all[offset : offset+want]), nil
}

// mpcGateAllows decides whether a registration may proceed.
//
// Returns a message to show the person when it may not. The message says
// "try again" for every operational failure, because a person refused by a
// rebooting validator must not be told they are already registered.
func mpcGateAllows(session string) (bool, string) {
	if strings.ToLower(os.Getenv("MPC_REQUIRED")) != "true" {
		return true, ""
	}
	if session == "" {
		return false, "this registration carries no duplicate-check session; please redo the " +
			"biometric step"
	}
	v, ok := consumeMPCVerdict(session)
	if !ok {
		return false, "the duplicate check has expired or was not completed; please redo the " +
			"biometric step"
	}
	if v.duplicate {
		return false, "this biometric is already registered"
	}
	return true, ""
}
