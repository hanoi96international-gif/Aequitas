package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The one property that makes this endpoint worth having is that it answers
// while the state lock is held — an operator needs it precisely when
// something else is wedged. A dump endpoint that itself blocks on cs.mu
// would be useless in the only situation it exists for, so that is what
// these tests pin, along with the auth gate and the grouping that turns a
// raw dump into a diagnosis.

func newGoroutineDumpServer(t *testing.T) *APIServer {
	t.Helper()
	return &APIServer{state: newTestState()}
}

func TestGoroutineDump_RequiresToken(t *testing.T) {
	t.Setenv("SNAPSHOT_TOKEN", "correct-horse")
	a := newGoroutineDumpServer(t)

	for _, tc := range []struct {
		name, header string
		wantCode     int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"right token", "Bearer correct-horse", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			goroutineDumpLastAt = time.Time{} // defeat the rate limiter between subtests
			req := httptest.NewRequest("GET", "/api/debug/goroutines?summary", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			a.handleGoroutineDump(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status %d, want %d (body: %s)", rec.Code, tc.wantCode, rec.Body.String())
			}
		})
	}
}

func TestGoroutineDump_RefusesWhenTokenUnset(t *testing.T) {
	t.Setenv("SNAPSHOT_TOKEN", "")
	goroutineDumpLastAt = time.Time{}
	a := newGoroutineDumpServer(t)
	req := httptest.NewRequest("GET", "/api/debug/goroutines", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rec := httptest.NewRecorder()
	a.handleGoroutineDump(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403 — an unconfigured node must not serve stacks to anyone", rec.Code)
	}
}

// TestGoroutineDump_AnswersWhileStateLockIsHeld is the point of the whole
// file. cs.mu is taken in WRITE mode and held for the entire request; the
// dump must still complete. If this ever starts hanging, the endpoint has
// acquired an application lock somewhere and is no longer usable for the
// job it exists to do.
func TestGoroutineDump_AnswersWhileStateLockIsHeld(t *testing.T) {
	t.Setenv("SNAPSHOT_TOKEN", "tok")
	goroutineDumpLastAt = time.Time{}
	a := newGoroutineDumpServer(t)

	a.state.mu.Lock()

	// A goroutine parked on the very lock being held, so the summary has
	// real contention to report rather than an idle process.
	var wg sync.WaitGroup
	release := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-release
		a.state.mu.RLock()
		a.state.mu.RUnlock()
	}()

	done := make(chan string, 1)
	go func() {
		req := httptest.NewRequest("GET", "/api/debug/goroutines?summary", nil)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		a.handleGoroutineDump(rec, req)
		done <- rec.Body.String()
	}()

	var body string
	select {
	case body = <-done:
	case <-time.After(15 * time.Second):
		a.state.mu.Unlock()
		close(release)
		t.Fatal("the goroutine dump did not return while cs.mu was held — it is taking an " +
			"application lock somewhere, which makes it useless in exactly the situation it exists for")
	}

	a.state.mu.Unlock()
	close(release)
	wg.Wait()

	if !strings.Contains(body, "goroutines:") {
		t.Errorf("summary missing its header line; got:\n%s", body)
	}
	if !strings.Contains(body, "blocked in:") {
		t.Errorf("summary missing the grouped output; got:\n%s", body)
	}
}

func TestGoroutineDump_FullDumpContainsStacks(t *testing.T) {
	t.Setenv("SNAPSHOT_TOKEN", "tok")
	goroutineDumpLastAt = time.Time{}
	a := newGoroutineDumpServer(t)
	req := httptest.NewRequest("GET", "/api/debug/goroutines", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	a.handleGoroutineDump(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "goroutine ") {
		t.Errorf("full dump contains no goroutine headers")
	}
	if rec.Header().Get("X-Goroutine-Count") == "" {
		t.Error("X-Goroutine-Count header missing — it is what a monitoring check would trend")
	}
}

func TestGoroutineDump_RateLimited(t *testing.T) {
	t.Setenv("SNAPSHOT_TOKEN", "tok")
	goroutineDumpLastAt = time.Time{}
	a := newGoroutineDumpServer(t)

	do := func() int {
		req := httptest.NewRequest("GET", "/api/debug/goroutines?summary", nil)
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		a.handleGoroutineDump(rec, req)
		return rec.Code
	}
	if code := do(); code != http.StatusOK {
		t.Fatalf("first request: %d", code)
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Errorf("second immediate request: %d, want 429 — collecting a dump stops the world, so "+
			"a monitor pointed here by mistake must not be able to cause the outage", code)
	}
}

// TestSummarizeGoroutineDump_GroupsByBlockingFrame checks the grouping logic
// against a hand-written dump in Go's exact format, so the parser is pinned
// independently of whatever the test process itself happens to be doing.
func TestSummarizeGoroutineDump_GroupsByBlockingFrame(t *testing.T) {
	const dump = `goroutine 1 [semacquire, 13 minutes]:
sync.runtime_SemacquireRWMutexR(0xc0000b4008?, 0x0?, 0x0?)
	/usr/local/go/src/runtime/sema.go:82 +0x25
sync.(*RWMutex).RLock(...)
	/usr/local/go/src/sync/rwmutex.go:71
github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper.(*ChainState).TransferAtomic(0xc000180000)
	/app/x/humanity/keeper/state.go:100 +0x45

goroutine 2 [semacquire, 2 minutes]:
sync.runtime_SemacquireRWMutexR(0xc0000b4008?, 0x0?, 0x0?)
	/usr/local/go/src/runtime/sema.go:82 +0x25
sync.(*RWMutex).RLock(...)
	/usr/local/go/src/sync/rwmutex.go:71
github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper.(*ChainState).TransferAtomic(0xc000180000)
	/app/x/humanity/keeper/state.go:100 +0x45

goroutine 3 [IO wait]:
internal/poll.runtime_pollWait(0x7f0, 0x72)
	/usr/local/go/src/runtime/netpoll.go:345 +0x85
github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper.(*APIServer).Start(0xc000180000)
	/app/x/humanity/keeper/api.go:700 +0x45
`
	got := summarizeGoroutineDump([]byte(dump), false)

	if !strings.Contains(got, "TransferAtomic") {
		t.Errorf("summary does not name the contended subsystem:\n%s", got)
	}
	if !strings.Contains(got, "sync.(*RWMutex).RLock") {
		t.Errorf("summary does not name the blocking frame — that is the whole diagnosis:\n%s", got)
	}
	if !strings.Contains(got, "waited >= 13min") {
		t.Errorf("summary lost the longest reported wait:\n%s", got)
	}
	// The two identically-blocked goroutines must collapse into one counted
	// group, and the longest-waiting group must sort first.
	idxTransfer := strings.Index(got, "TransferAtomic")
	idxIOWait := strings.Index(got, "IO wait")
	if idxIOWait >= 0 && idxTransfer > idxIOWait {
		t.Errorf("the 13-minute lock wait sorted below an idle IO wait:\n%s", got)
	}
	if !strings.Contains(got, "     2  semacquire") {
		t.Errorf("the two identically-blocked goroutines did not collapse into one group of 2:\n%s", got)
	}
}
