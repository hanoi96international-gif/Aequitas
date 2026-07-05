package keeper

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGzipMiddleware_CompressesWhenAccepted is the regression guard for the
// 2026-07-05 website audit finding: the explorer/landing HTML response
// (~800KB) was served with no Content-Encoding at all despite every real
// browser advertising gzip support — pure wasted bandwidth on every single
// page load. Verifies a client that sends Accept-Encoding: gzip gets back
// a gzip-compressed, correctly-decodable body with the right header.
func TestGzipMiddleware_CompressesWhenAccepted(t *testing.T) {
	body := strings.Repeat("hello world, this is compressible text. ", 200)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})

	req := httptest.NewRequest("GET", "/explorer", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	rec := httptest.NewRecorder()
	gzipMiddleware(inner).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
	}
	gzr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("response body is not valid gzip: %v", err)
	}
	decoded, err := io.ReadAll(gzr)
	if err != nil {
		t.Fatalf("could not decode gzip body: %v", err)
	}
	if string(decoded) != body {
		t.Fatal("decoded gzip body does not match the original response")
	}
	if rec.Body.Len() >= len(body) {
		t.Fatalf("compressed body (%d bytes) is not smaller than the original (%d bytes) for highly repetitive text", rec.Body.Len(), len(body))
	}
}

// TestGzipMiddleware_SkipsWithoutAcceptEncoding verifies a client that
// doesn't advertise gzip support gets the plain, uncompressed response —
// this middleware must never break a client that can't decompress.
func TestGzipMiddleware_SkipsWithoutAcceptEncoding(t *testing.T) {
	body := "plain response"
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})

	req := httptest.NewRequest("GET", "/explorer", nil)
	rec := httptest.NewRecorder()
	gzipMiddleware(inner).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty (client did not advertise gzip support)", got)
	}
	if rec.Body.String() != body {
		t.Fatalf("body = %q, want unmodified %q", rec.Body.String(), body)
	}
}

// TestGzipMiddleware_SkipsDownloadPaths verifies /download/ paths (PDF/APK,
// already-compressed formats) are never double-gzipped even when the
// client advertises support — see gzipMiddleware's own comment.
func TestGzipMiddleware_SkipsDownloadPaths(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pdf-bytes-here"))
	})

	req := httptest.NewRequest("GET", "/download/node-guide-en.pdf", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	gzipMiddleware(inner).ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty for a /download/ path", got)
	}
	if rec.Body.String() != "pdf-bytes-here" {
		t.Fatal("a /download/ path's body must pass through unmodified")
	}
}
