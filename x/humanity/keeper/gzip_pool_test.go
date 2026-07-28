package keeper

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// Reusing a gzip.Writer is only safe if every response still carries gzip's
// trailer and decodes to exactly what was written. Getting that wrong would not
// look like a compression bug: /api/blocks is the peer sync path, so a
// truncated body reaches every catching-up node as an undecodable response and
// stalls it -- a failure mode this project has already lived through once, when
// a partially-written /api/blocks body left both secondaries retrying the same
// min_height forever.
func TestPooledGzipWriter_RoundTripsAcrossReuse(t *testing.T) {
	// Deliberately reuses the same writer several times in a row, since a
	// missing Reset or a missing Close only shows up on the SECOND use.
	for i := 0; i < 5; i++ {
		want := strings.Repeat(fmt.Sprintf("block-%d-payload;", i), 500)

		var buf bytes.Buffer
		gz := acquireGzipWriter(&buf)
		if _, err := io.WriteString(gz, want); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		releaseGzipWriter(gz)

		zr, err := gzip.NewReader(&buf)
		if err != nil {
			t.Fatalf("round %d: response is not readable gzip: %v", i, err)
		}
		got, err := io.ReadAll(zr)
		if err != nil {
			t.Fatalf("round %d: body did not decode (a missing trailer looks exactly like this): %v", i, err)
		}
		if err := zr.Close(); err != nil {
			t.Fatalf("round %d: gzip stream did not end cleanly: %v", i, err)
		}
		if string(got) != want {
			t.Fatalf("round %d: decoded %d bytes, want %d", i, len(got), len(want))
		}
	}
}

// The middleware runs one writer per in-flight request, so the pool is used
// concurrently. A writer handed to two requests at once would interleave two
// bodies into one stream.
func TestPooledGzipWriter_ConcurrentRequestsStayIntact(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			want := strings.Repeat(fmt.Sprintf("payload-%02d;", n), 400)
			var buf bytes.Buffer
			gz := acquireGzipWriter(&buf)
			io.WriteString(gz, want)
			releaseGzipWriter(gz)

			zr, err := gzip.NewReader(&buf)
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: not gzip: %w", n, err)
				return
			}
			got, err := io.ReadAll(zr)
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: %w", n, err)
				return
			}
			if string(got) != want {
				errs <- fmt.Errorf("goroutine %d: body was not its own -- pooled writer shared between requests", n)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

// BestSpeed is a deliberate trade, not an accident, so it is worth pinning that
// it still compresses meaningfully -- a level that stopped compressing would
// silently multiply what every syncing peer has to download.
func TestPooledGzipWriter_StillCompresses(t *testing.T) {
	// Repetitive like real block JSON, which is why 505KB of it measured 88KB.
	want := strings.Repeat(`{"hash":"0xabc","height":1234,"txs":[]},`, 2000)
	var buf bytes.Buffer
	gz := acquireGzipWriter(&buf)
	io.WriteString(gz, want)
	releaseGzipWriter(gz)

	ratio := float64(len(want)) / float64(buf.Len())
	if ratio < 3 {
		t.Fatalf("compressed %d bytes to %d (%.1fx); block JSON this repetitive should compress far better -- is the level still valid?",
			len(want), buf.Len(), ratio)
	}
}
