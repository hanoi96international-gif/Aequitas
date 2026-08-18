package keeper

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// Before these, the explorer could not be operated without a mouse at all.
// 28 of the elements carrying data-act were plain <div>s — every entry in the
// section bar, every sub-tab, the shortcut cards — and a div takes neither
// focus nor Enter on its own, while the only listener in the file was for
// 'click'. A keyboard visitor could reach the page and then go nowhere.
//
// None of this is visible in a screenshot, and nothing else fails when it
// regresses, which is why it is pinned here rather than left to review.

var dataActElementRe = regexp.MustCompile(`<(\w+)([^>]*\sdata-act="[^"]*"[^>]*)>`)

// Native controls are already focusable and already activate on Enter; only
// the improvised ones need the attributes.
func isNativelyInteractive(tag string) bool {
	switch strings.ToLower(tag) {
	case "button", "a", "input", "select", "textarea":
		return true
	}
	return false
}

func TestExplorerHTML_EveryClickableIsKeyboardReachable(t *testing.T) {
	matches := dataActElementRe.FindAllStringSubmatch(explorerHTML, -1)
	if len(matches) < 20 {
		t.Fatalf("only %d data-act elements found — has the attribute been renamed?", len(matches))
	}

	for _, m := range matches {
		tag, attrs := m[1], m[2]
		if isNativelyInteractive(tag) {
			continue
		}
		if !strings.Contains(attrs, "tabindex=") {
			t.Errorf("<%s data-act=…> has no tabindex — Tab skips it, so it cannot be reached by keyboard: %.120s", tag, m[0])
		}
		if !strings.Contains(attrs, "role=") {
			t.Errorf("<%s data-act=…> has no role — a screen reader announces it as plain text: %.120s", tag, m[0])
		}
	}
}

// A tabindex without a key handler is only half the fix: the element takes
// focus and then does nothing when activated.
func TestExplorerJS_ActivatesOnKeyboardNotOnlyClick(t *testing.T) {
	if !strings.Contains(explorerJS, "addEventListener('keydown'") {
		t.Error("explorer.js registers no keydown listener — improvised controls take focus but cannot be activated")
	}
	for _, key := range []string{"'Enter'", "' '"} {
		if !strings.Contains(explorerJS, key) {
			t.Errorf("explorer.js never tests for %s — that key cannot activate a control", key)
		}
	}
}

// Pinning the zoom fails WCAG 1.4.4, and the landing page never did it, so the
// two pages used to disagree about whether a reader may enlarge the text.
var viewportMetaRe = regexp.MustCompile(`<meta\s+name="viewport"\s+content="([^"]*)"`)

func TestPages_DoNotBlockPinchZoom(t *testing.T) {
	for _, tc := range []struct{ name, html string }{
		{"landing", landingHTML},
		{"explorer", explorerHTML},
	} {
		m := viewportMetaRe.FindStringSubmatch(tc.html)
		if m == nil {
			t.Errorf("%s page declares no viewport meta tag", tc.name)
			continue
		}
		for _, bad := range []string{"maximum-scale", "user-scalable=no", "user-scalable=0"} {
			if strings.Contains(m[1], bad) {
				t.Errorf("%s page viewport contains %q (%q) — a reader who needs to zoom cannot", tc.name, bad, m[1])
			}
		}
	}
}

// The explorer had no heading of any level in 184 KB of markup: everything
// was a styled <div>, so nothing navigating by document structure — screen
// readers, search engines — could see an outline at all.
func TestPages_HaveHeadingsAndAMainLandmark(t *testing.T) {
	headingRe := regexp.MustCompile(`<h[1-6][\s>]`)
	for _, tc := range []struct{ name, html string }{
		{"landing", landingHTML},
		{"explorer", explorerHTML},
	} {
		if n := len(headingRe.FindAllString(tc.html, -1)); n == 0 {
			t.Errorf("%s page contains no heading elements at all", tc.name)
		}
		if !strings.Contains(tc.html, "<main") {
			t.Errorf("%s page has no <main> landmark", tc.name)
		}
	}
}

// A focus style is what tells a keyboard user where they are. Only a handful
// of text inputs had one.
func TestStylesheets_DefineAVisibleFocusStyle(t *testing.T) {
	for _, tc := range []struct{ name, css string }{
		{"landing", landingHTML},
		{"explorer", explorerCSS},
	} {
		if !strings.Contains(tc.css, ":focus-visible") {
			t.Errorf("%s stylesheet defines no :focus-visible style — keyboard focus is invisible", tc.name)
		}
	}
}

// The explorer used to set ~260 of its font sizes between 0.6rem and 0.63rem
// — under 10px — with the smallest label at 0.48rem, or 7.7px. Contrast was
// never the problem on this site (every palette role clears WCAG AA); size
// was. The scale was lifted by a single monotonic curve rather than a floor,
// so the hierarchy the design builds with those steps survives.
//
// This pins the bottom of that scale. It is deliberately a floor and not an
// exact set: adding a new size is fine, adding an unreadable one is not.
var remFontSizeRe = regexp.MustCompile(`font-size:([0-9.]+)rem`)

func TestStylesheets_NoUnreadablyTinyText(t *testing.T) {
	const floorRem = 0.68 // 10.9px at a 16px root

	for _, tc := range []struct{ name, css string }{
		{"landing.go", landingHTML},
		{"explorer.css", explorerCSS},
		{"explorer.html", explorerHTML},
	} {
		seen := map[string]bool{}
		for _, m := range remFontSizeRe.FindAllStringSubmatch(tc.css, -1) {
			var v float64
			if _, err := fmt.Sscanf(m[1], "%g", &v); err != nil {
				continue
			}
			if v < floorRem && !seen[m[1]] {
				seen[m[1]] = true
				t.Errorf("%s sets font-size:%srem (%.1fpx) — below the %.2frem floor this scale was lifted to",
					tc.name, m[1], v*16, floorRem)
			}
		}
	}
}
