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

// The bug this guards against: flex and grid items default to
// min-width:auto, meaning they refuse to shrink below their own content's
// unwrapped width. .header-right already worked around this on its own
// (min-width:0), which meant the fix was known and simply not applied
// everywhere — a translated value longer than its English source (German
// ran ~30% longer) then forced a grid track wider than the viewport, and
// that width propagated straight up through the whole page. Measured
// directly: 141px of real horizontal overflow on the Humans panel in German
// before this rule existed, zero after, across all twelve locales and every
// panel.
var universalResetRe = regexp.MustCompile(`\*\{[^}]*\}`)

func TestStylesheets_ResetMinWidthOnFlexGridItems(t *testing.T) {
	for _, tc := range []struct{ name, css string }{
		{"landing.go", landingHTML},
		{"explorer.css", explorerCSS},
	} {
		reset := universalResetRe.FindString(tc.css)
		if reset == "" {
			t.Fatalf("%s: no universal *{...} reset rule found — has it been renamed or restructured?", tc.name)
		}
		// Checked against the *{...} rule specifically, not the file as a whole:
		// min-width:0 is legitimately declared elsewhere too (.header-right, for
		// one), so a whole-file substring check would still pass even with the
		// universal rule reverted — exactly the case that must fail here.
		if !strings.Contains(reset, "min-width:0") {
			t.Errorf("%s: the universal reset (%s) no longer sets min-width:0 — flex/grid"+
				" items can blow out their container again the moment translated text"+
				" runs longer than English", tc.name, reset)
		}
	}
}

// KNIGHTDAG was set to display:none below 600px — hiding it on effectively
// every phone, the opposite of what PR #125 added the badge for. Guards
// against that specific rule coming back, not against .badge-dag being
// styled at all (it still legitimately needs display:none logic nowhere).
func TestExplorerCSS_KnightDAGBadgeIsNotHiddenOnPhones(t *testing.T) {
	if strings.Contains(explorerCSS, ".badge-dag{display:none}") {
		t.Error("explorer.css hides .badge-dag (the KNIGHTDAG badge) in a media query — " +
			"this was hiding it on every screen narrower than the breakpoint, i.e. most phones")
	}
}

// The Gini history chart plots pt.idx — the Aequitas Index, a 0-100 value —
// against a 0-100 axis. Its latest-value label printed that same number under
// the word "Gini", the 0-1 value, directly beside a target line reading
// "TARGET 0.30". On screen that read as Gini 9.581 against a target of 0.30:
// thirty-two times over target, when the chain was actually at Gini 0.096,
// comfortably under it. The chart asserted the exact opposite of the one
// number this whole project is built on.
//
// Guards the specific confusion, not the wording: an idx value must never be
// formatted under a bare "Gini" label.
func TestExplorerJS_GiniChartDoesNotLabelIndexAsGini(t *testing.T) {
	for _, bad := range []string{
		"'Gini: '+lpt.idx",
		"'Gini: ' + lpt.idx",
		"'Gini '+lpt.idx",
	} {
		if strings.Contains(explorerJS, bad) {
			t.Errorf("explorer.js formats %s — that prints the 0-100 Index under the"+
				" name of the 0-1 Gini, making the chain look far worse than it is", bad)
		}
	}
	// The fix keeps both scales on screen; if neither name survives, the label
	// has been rewritten into something this test can no longer vouch for.
	if !strings.Contains(explorerJS, "INDEX 30 = GINI 0.30") {
		t.Error("explorer.js no longer states the Index/Gini equivalence on the target line —" +
			" a bare number beside a 0-100 axis is what caused the original misreading")
	}
}

// min-width:0 lets a flex/grid BOX shrink. It cannot make the text inside it
// wrap: a 42-character wallet address, or a German compound noun, is a single
// token with no break opportunity, so it runs straight past its card border no
// matter how the box is sized. That is a different failure from the layout
// blowout, and it was reported separately — the connected-wallet address
// crossing its own box, and step descriptions crossing theirs.
func TestStylesheets_LongTokensCanWrap(t *testing.T) {
	for _, tc := range []struct{ name, css string }{
		{"landing.go", landingHTML},
		{"explorer.css", explorerCSS},
	} {
		if !strings.Contains(tc.css, "overflow-wrap:break-word") {
			t.Errorf("%s: body no longer sets overflow-wrap:break-word — an unbreakable"+
				" token (a wallet address, a long compound noun) will cross its container"+
				" border again instead of wrapping", tc.name)
		}
	}
	// The address is the worst case and gets the stronger value.
	if !strings.Contains(explorerCSS, "overflow-wrap:anywhere") {
		t.Error("explorer.css: .wadr no longer sets overflow-wrap:anywhere — the connected" +
			" wallet address is one 42-character token and will overflow its box")
	}
}
