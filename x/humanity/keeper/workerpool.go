package keeper

// workerPool is a small, fixed-size pool of persistent goroutines that
// execute submitted closures, reused across calls instead of spawning a
// fresh goroutine per call. Submit blocks until an idle worker picks up the
// job (unbuffered channel) — callers that need to wait for completion do so
// via their own sync.WaitGroup/channel inside the submitted closure, same
// as they would with a raw `go func(){...}()`.
type workerPool struct {
	jobs chan func()
}

// newWorkerPool starts n persistent worker goroutines, each looping forever
// on jobs until the process exits (there is no Stop — every user of this
// pool in this codebase is a package-level pool sized for a fixed, always-on
// piece of per-block work, not a scoped/short-lived pool).
func newWorkerPool(n int) *workerPool {
	p := &workerPool{jobs: make(chan func())}
	for i := 0; i < n; i++ {
		go func() {
			for job := range p.jobs {
				job()
			}
		}()
	}
	return p
}

// submit hands job to whichever worker is next idle, blocking until one
// picks it up. The job itself is responsible for its own panic recovery and
// for signaling completion (WaitGroup.Done, a result channel, etc.) — this
// pool only replaces the goroutine spinup, not the caller's synchronization.
func (p *workerPool) submit(job func()) {
	p.jobs <- job
}

// produceBlockPool backs ProduceBlock's concurrent LoadPendingTxs/StateRoot
// pair (block.go) — see that call site's own comment. Sized at 2 because
// ProduceBlock always submits exactly 2 jobs per tick and always awaits both
// before returning, so a 3rd worker would never have anything to do and a
// single worker would serialize what's meant to run concurrently.
var produceBlockPool = newWorkerPool(2)
