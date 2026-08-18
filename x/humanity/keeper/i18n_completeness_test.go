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

// The test above only ever compares the locales against each other, so it is
// blind to the two ways the dictionary and the markup drift apart on their
// own — and both had happened by the time anyone looked:
//
//   - 31 keys sat in all twelve locales that no element referenced any more.
//     Translators kept carrying them forward, and several were stale copies
//     of navigation labels the site had since renamed.
//   - 3 keys (expl-consensus, expl-consensus-d, s-validators) were spelled
//     into data-i18n attributes but existed in no locale at all. setLang()
//     only assigns `if (t[key] !== undefined)`, so those three elements
//     silently kept their English source text in all twelve languages —
//     exactly the silence this file was written to end.
//
// Neither shows up as a failure anywhere else: the first is invisible, and
// the second looks like working English until you switch language.

// i18nDynamicKeys are read straight out of T by JavaScript rather than through
// a data-i18n attribute, so the markup scan below cannot see them. setLang()
// is the only other reader and it goes through data-i18n, so this list stays
// short — add to it if you introduce another T[curLang]['...'] lookup.
var i18nDynamicKeys = map[string]bool{
	"guard-none": true, // guardian address panel, see explorer.js
}

// englishI18nKeys returns the key set of the English block, which the test
// above already establishes as the maintained source of truth.
func englishI18nKeys(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("assets/explorer.js")
	if err != nil {
		t.Fatalf("could not read assets/explorer.js: %v", err)
	}
	content := string(src)

	tStart := strings.Index(content, "const T = {")
	if tStart < 0 {
		t.Fatal("could not find 'const T = {' in assets/explorer.js — has the i18n dictionary been renamed/restructured?")
	}
	enStart := strings.Index(content[tStart:], "\nen:{")
	if enStart < 0 {
		t.Fatal("could not find the english locale block")
	}
	// The English block runs until the next locale opens; de follows it.
	deStart := strings.Index(content[tStart:], "\nde:{")
	if deStart < 0 || deStart <= enStart {
		t.Fatal("could not find the german locale block after the english one")
	}
	chunk := content[tStart+enStart : tStart+deStart]

	keys := map[string]bool{}
	for _, m := range i18nKeyRe.FindAllStringSubmatch(chunk, -1) {
		keys[m[1]] = true
	}
	if len(keys) < 50 {
		t.Fatalf("suspiciously few English i18n keys extracted (%d) — the key-extraction regex or locale markers may be broken", len(keys))
	}
	return keys
}

var i18nMarkupKeyRe = regexp.MustCompile(`data-i18n="([^"]+)"`)

// markupI18nKeys returns every key the explorer markup asks to be translated.
func markupI18nKeys(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("assets/explorer.html")
	if err != nil {
		t.Fatalf("could not read assets/explorer.html: %v", err)
	}
	keys := map[string]bool{}
	for _, m := range i18nMarkupKeyRe.FindAllStringSubmatch(string(src), -1) {
		keys[m[1]] = true
	}
	if len(keys) < 50 {
		t.Fatalf("suspiciously few data-i18n attributes found (%d) — has the markup or the attribute name changed?", len(keys))
	}
	return keys
}

// A data-i18n attribute naming a key no locale defines is the worst of the
// two failures: the element renders its English source text in every
// language, and nothing anywhere reports a problem.
func TestI18nMarkupKeysExistInDictionary(t *testing.T) {
	enKeys := englishI18nKeys(t)

	var missing []string
	for k := range markupI18nKeys(t) {
		if !enKeys[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d data-i18n key(s) in assets/explorer.html are defined in no locale (these elements stay English in all %d languages): %v",
			len(missing), len(i18nLocales), missing)
	}
}

// The other direction: a key every translator maintains that no element will
// ever display. Harmless to render, but it is work asked of twelve people for
// nothing, and it hides which strings are actually live.
func TestI18nDictionaryHasNoOrphanedKeys(t *testing.T) {
	markupKeys := markupI18nKeys(t)

	var orphans []string
	for k := range englishI18nKeys(t) {
		if !markupKeys[k] && !i18nDynamicKeys[k] {
			orphans = append(orphans, k)
		}
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		t.Errorf("%d i18n key(s) are defined in every locale but referenced by no data-i18n attribute — delete them, or add them to i18nDynamicKeys if JavaScript reads them out of T directly: %v",
			len(orphans), orphans)
	}
}
