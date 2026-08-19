package keeper

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// /api/gini/history is chronological, and exactly one side may say so.
//
// THE INCIDENT THIS PINS (reported 2026-08-19: "the content of the chart is
// wrong").
//
// GetGiniHistory queries `ORDER BY captured_at DESC` — newest first, because
// that is how you take the newest N — and then reverses the rows itself, with
// the comment "Reverse to get chronological order (we queried DESC)". The
// endpoint therefore emits OLDEST FIRST.
//
// drawGiniHistoryChart reversed it a second time. The chart plots array index
// left to right, so time ran backwards across the canvas: a chain growing
// steadily less equal was drawn as one steadily improving. Rendered from a real
// server payload of 5 → 15 → 25 → 35 (rising inequality), the canvas showed a
// falling curve. And history[length-1], the point the latest-value label names,
// was the very FIRST snapshot ever taken — the label read "Index 5.0" for a
// chain sitting at 35.
//
// Nothing on the canvas dates the x axis, so neither the direction nor the
// mislabelled point was visible on its own. The only symptom was a trend
// pointing the wrong way, on the chart that publishes this project's headline
// number.
//
// Both halves are asserted because either one alone can restore the bug: a
// server that stops reversing, or a client that starts. They are one contract
// held in two files that never reference each other.
func TestGiniHistory_ChartPlotsTheServerOrderAsGiven(t *testing.T) {
	// Client half: the payload must reach the chart untouched.
	assign := regexp.MustCompile(`const history = \(d\.history \|\| \[\]\)([^;]*);`).
		FindStringSubmatch(explorerJS)
	if assign == nil {
		t.Fatal("explorer.js: could not find drawGiniHistoryChart's history assignment — " +
			"if it was rewritten, re-point this test at the new form rather than deleting it")
	}
	if strings.Contains(assign[1], "reverse") {
		t.Errorf("explorer.js reverses /api/gini/history (%q). The server already returns it "+
			"oldest-first, so this draws the chart backwards in time and makes the "+
			"latest-value label name the OLDEST snapshot.", "(d.history || [])"+assign[1])
	}

	// Server half: the DESC query must still be undone before it is returned.
	src, err := os.ReadFile("evm_storage.go")
	if err != nil {
		t.Fatalf("read evm_storage.go: %v", err)
	}
	fn := funcBody(string(src), "func (cs *ChainState) GetGiniHistory(")
	if fn == "" {
		t.Fatal("GetGiniHistory not found in evm_storage.go")
	}
	if !strings.Contains(fn, "ORDER BY captured_at DESC") {
		t.Fatal("GetGiniHistory no longer orders by captured_at DESC — this test's " +
			"reasoning about the reversal below no longer holds; re-derive it")
	}
	if !strings.Contains(fn, "result[i], result[j] = result[j], result[i]") {
		t.Error("GetGiniHistory selects newest-first but no longer reverses the rows. " +
			"The endpoint's consumers — drawGiniHistoryChart above — plot it as given, " +
			"so this hands them a chart that runs backwards in time.")
	}
}

// funcBody returns the source of the function whose declaration starts with
// decl, by brace matching from its opening brace.
func funcBody(src, decl string) string {
	i := strings.Index(src, decl)
	if i < 0 {
		return ""
	}
	// The body's opening brace is the last character on the declaration line;
	// the first "{" after decl belongs to a return type like
	// []map[string]interface{}, whose braces balance immediately.
	open := strings.Index(src[i:], "{\n")
	if open < 0 {
		return ""
	}
	depth := 0
	for k := i + open; k < len(src); k++ {
		switch src[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[i : k+1]
			}
		}
	}
	return ""
}
