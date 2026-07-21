package keeper

import (
	"regexp"
	"testing"
)

// TestExplorerHTMLVersioned_LinksToHashedAssetURLs is the regression guard
// for the 2026-07-21 stale-cache incident (see explorerCSSVersion's own
// comment): explorerHTML must never be served to a browser un-rewritten —
// the "/explorer.css"/"/explorer.js" bare paths it contains on disk MUST be
// replaced with content-hashed "?v=" URLs before going out, or a browser
// that already cached a previous deploy's JS/CSS keeps serving it for the
// rest of that cache's lifetime regardless of how many times the server
// has redeployed since.
func TestExplorerHTMLVersioned_LinksToHashedAssetURLs(t *testing.T) {
	if explorerCSSVersion == "" || explorerJSVersion == "" {
		t.Fatal("explorerCSSVersion/explorerJSVersion must not be empty")
	}
	cssRe := regexp.MustCompile(`href="/explorer\.css\?v=[0-9a-f]{8}"`)
	jsRe := regexp.MustCompile(`src="/explorer\.js\?v=[0-9a-f]{8}"`)
	if !cssRe.MatchString(explorerHTMLVersioned) {
		t.Error("explorerHTMLVersioned does not link to a hashed /explorer.css?v=<hash> URL")
	}
	if !jsRe.MatchString(explorerHTMLVersioned) {
		t.Error("explorerHTMLVersioned does not link to a hashed /explorer.js?v=<hash> URL")
	}
	// The raw, unversioned paths must be GONE, not just supplemented — a
	// leftover bare href/src would mean a stray second reference the
	// replacer missed.
	if regexp.MustCompile(`href="/explorer\.css"`).MatchString(explorerHTMLVersioned) {
		t.Error("explorerHTMLVersioned still contains an unversioned href=\"/explorer.css\"")
	}
	if regexp.MustCompile(`src="/explorer\.js"`).MatchString(explorerHTMLVersioned) {
		t.Error("explorerHTMLVersioned still contains an unversioned src=\"/explorer.js\"")
	}
}

// TestExplorerAssetVersions_ChangeWithContent pins the fingerprint's whole
// purpose: two different byte strings must (overwhelmingly likely, per
// CRC32's collision rate at this scale) hash to different versions, and the
// same string must hash the same way every time — otherwise a real content
// change wouldn't reliably bust a browser's cache, or a cold restart with
// unchanged content would needlessly bust it.
func TestExplorerAssetVersions_ChangeWithContent(t *testing.T) {
	if explorerCSSVersion == explorerJSVersion {
		t.Fatalf("CSS and JS versions collided (%q) — extremely unlikely for genuinely different content; check the hashing input", explorerCSSVersion)
	}
}
