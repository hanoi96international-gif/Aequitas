package keeper

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Regression/safety-net test for the beta-launch audit (2026-07-05): this
// session already found and fixed several cases of a key existing in some
// locale dictionaries but not others (e.g. "~6 seconds" fixed in only 3 of
// 12 languages, node3/node3-desc missing from every locale) — setLang()'s
// generic i18n loop only overwrites innerHTML `if (t[key] !== undefined)`,
// so a missing key doesn't error or show a placeholder, it just silently
// leaves whatever text (usually stale English) was already there. That
// silence is exactly what let those gaps go unnoticed. This test makes any
// future partial-locale update loud instead of silent by verifying every
// locale in the T i18n dictionary (api_html.go) has the exact same key set
// as English, the maintained source of truth.
//
// Parses assets/explorer.js as text (not JS) — the dictionary is a plain
// object literal (`const T = { en:{...}, de:{...}, ... }`), so locale
// boundaries are found by searching for each known language code's opening
// marker rather than hardcoding line numbers, which would break the moment
// anyone edits unrelated content above these blocks.
var i18nLocales = []string{"en", "de", "es", "ru", "zh", "id", "it", "tr", "fr", "pt", "ar", "hi"}

var i18nKeyRe = regexp.MustCompile(`'([a-zA-Z0-9_-]+)':`)

func TestI18nLocaleKeysMatchEnglish(t *testing.T) {
	src, err := os.ReadFile("assets/explorer.js")
	if err != nil {
		t.Fatalf("could not read assets/explorer.js: %v", err)
	}
	content := string(src)

	tStart := strings.Index(content, "const T = {")
	if tStart < 0 {
		t.Fatal("could not find 'const T = {' in assets/explorer.js — has the i18n dictionary been renamed/restructured?")
	}

	type localeSpan struct {
		name  string
		start int
	}
	var spans []localeSpan
	for _, loc := range i18nLocales {
		marker := "\n" + loc + ":{"
		idx := strings.Index(content[tStart:], marker)
		if idx < 0 {
			t.Fatalf("could not find locale block %q (expected marker %q) — has a locale been renamed or removed?", loc, marker)
		}
		spans = append(spans, localeSpan{loc, tStart + idx})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	keysFor := func(i int) map[string]bool {
		end := len(content)
		if i+1 < len(spans) {
			end = spans[i+1].start
		}
		chunk := content[spans[i].start:end]
		keys := map[string]bool{}
		for _, m := range i18nKeyRe.FindAllStringSubmatch(chunk, -1) {
			keys[m[1]] = true
		}
		return keys
	}

	enIdx := -1
	for i, s := range spans {
		if s.name == "en" {
			enIdx = i
		}
	}
	if enIdx < 0 {
		t.Fatal("english locale block not found")
	}
	enKeys := keysFor(enIdx)
	// Sanity check: if the regex or markers are broken, this would silently
	// pass with near-empty key sets on both sides — guard against that.
	if len(enKeys) < 50 {
		t.Fatalf("suspiciously few English i18n keys extracted (%d) — the key-extraction regex or locale markers may be broken, not that the dictionary is actually this small", len(enKeys))
	}

	for i, s := range spans {
		if s.name == "en" {
			continue
		}
		locKeys := keysFor(i)
		var missing []string
		for k := range enKeys {
			if !locKeys[k] {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("locale %q is missing %d key(s) present in English (will silently show stale/English text for these): %v",
				s.name, len(missing), missing)
		}
	}
}
