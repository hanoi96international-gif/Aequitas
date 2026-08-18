package keeper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The link-preview card and the favicon are the two assets whose absence is
// invisible from inside the site: pages render perfectly without them, and the
// only symptom is that every link shared on X or Telegram — the two channels
// this site points people at — comes out as a bare URL, and every browser tab
// shows a blank page icon. Nothing else would fail if these routes broke, so
// they are pinned here.
func TestBuildMux_BrandAssetsAreServed(t *testing.T) {
	mux := newRoutingTestServer().buildMux()

	for _, tc := range []struct {
		path        string
		wantType    string
		wantMinSize int
	}{
		{"/favicon.svg", "image/svg+xml", 200},
		{"/favicon.ico", "image/svg+xml", 200},
		{"/apple-touch-icon.png", "image/png", 1000},
		{"/og-image.png", "image/png", 10000},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", tc.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.wantType) {
			t.Errorf("%s: Content-Type = %q, want %q", tc.path, ct, tc.wantType)
		}
		if n := rec.Body.Len(); n < tc.wantMinSize {
			t.Errorf("%s: body is %d bytes, want at least %d — is the embedded file empty or truncated?",
				tc.path, n, tc.wantMinSize)
		}
	}
}

// The PNG routes must actually carry PNG bytes. An embed that silently picked
// up the wrong file would still pass the size check above.
func TestBuildMux_ImageAssetsAreRealPNGs(t *testing.T) {
	mux := newRoutingTestServer().buildMux()
	const pngMagic = "\x89PNG\r\n\x1a\n"

	for _, path := range []string{"/apple-touch-icon.png", "/og-image.png"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if !strings.HasPrefix(rec.Body.String(), pngMagic) {
			t.Errorf("%s: body does not start with the PNG signature", path)
		}
	}
}

// Crawlers fetch the card cross-origin. Without this header some of them drop
// the image and fall back to a text-only preview.
func TestBuildMux_OGImageAllowsCrossOriginFetch(t *testing.T) {
	mux := newRoutingTestServer().buildMux()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/og-image.png", nil))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

// Both pages must advertise the card, and og:image must be absolute — crawlers
// do not resolve a relative path against the page they found it on.
func TestPages_DeclareLinkPreviewTags(t *testing.T) {
	for _, tc := range []struct{ name, html string }{
		{"landing", landingHTML},
		{"explorer", explorerHTML},
	} {
		for _, want := range []string{
			`property="og:title"`,
			`property="og:description"`,
			`property="og:image" content="https://aequitas.digital/og-image.png"`,
			`name="twitter:card" content="summary_large_image"`,
			`rel="icon" href="/favicon.svg"`,
			`rel="canonical"`,
		} {
			if !strings.Contains(tc.html, want) {
				t.Errorf("%s page is missing %s", tc.name, want)
			}
		}
	}
}
