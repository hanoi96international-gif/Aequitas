package marketsignals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

func TestLoadCSV_HeaderAndTimestampFormats(t *testing.T) {
	cases := map[string]string{
		"rfc3339": `time,open,high,low,close,volume
2024-01-01T00:00:00Z,100,101,99,100.5,10
2024-01-01T01:00:00Z,100.5,102,100,101,12
2024-01-01T02:00:00Z,101,103,100.5,102,11
`,
		"epoch seconds": `1704067200,100,101,99,100.5,10
1704070800,100.5,102,100,101,12
1704074400,101,103,100.5,102,11
`,
		"epoch millis": `1704067200000,100,101,99,100.5,10
1704070800000,100.5,102,100,101,12
1704074400000,101,103,100.5,102,11
`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			s, err := LoadCSV(writeTemp(t, "bars.csv", content), "TEST", time.Hour)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(s.Candles) != 3 {
				t.Fatalf("got %d candles, want 3", len(s.Candles))
			}
			want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			if !s.Candles[0].Time.Equal(want) {
				t.Fatalf("first bar at %s, want %s", s.Candles[0].Time, want)
			}
			if s.Candles[2].Close != 102 {
				t.Fatalf("last close %v, want 102", s.Candles[2].Close)
			}
		})
	}
}

func TestLoadCSV_OptionalFlowAndFunding(t *testing.T) {
	withCols := `time,open,high,low,close,volume,buy_volume,sell_volume,funding
2024-01-01T00:00:00Z,100,101,99,100.5,10,6,4,0.0001
2024-01-01T01:00:00Z,100.5,102,100,101,12,7,5,0.0002
`
	s, err := LoadCSV(writeTemp(t, "flow.csv", withCols), "TEST", time.Hour)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !s.Candles[0].HasFlow() {
		t.Fatal("taker split column was not read")
	}
	if len(s.Funding) != len(s.Candles) {
		t.Fatalf("funding covers %d of %d bars", len(s.Funding), len(s.Candles))
	}

	minimal := `time,open,high,low,close,volume
2024-01-01T00:00:00Z,100,101,99,100.5,10
2024-01-01T01:00:00Z,100.5,102,100,101,12
`
	s2, err := LoadCSV(writeTemp(t, "min.csv", minimal), "TEST", time.Hour)
	if err != nil {
		t.Fatalf("load minimal: %v", err)
	}
	if s2.Candles[0].HasFlow() {
		t.Fatal("a file with no taker columns must not report flow")
	}
	if s2.Funding != nil {
		t.Fatal("a file with no funding column must leave Funding nil, not zero-filled — " +
			"zeros read as 'funding is neutral' when the truth is that it is unknown")
	}
}

func TestLoadCSV_RejectsCorruptInput(t *testing.T) {
	cases := map[string]string{
		"impossible OHLC": `time,open,high,low,close,volume
2024-01-01T00:00:00Z,100,99,101,100,10
2024-01-01T01:00:00Z,100,101,99,100,10
2024-01-01T02:00:00Z,100,101,99,100,10
`,
		"out of order": `time,open,high,low,close,volume
2024-01-01T02:00:00Z,100,101,99,100,10
2024-01-01T00:00:00Z,100,101,99,100,10
`,
		"too few columns": `time,open,high,low
2024-01-01T00:00:00Z,100,101,99
`,
		"non-numeric price": `time,open,high,low,close,volume
2024-01-01T00:00:00Z,abc,101,99,100,10
`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadCSV(writeTemp(t, "bad.csv", content), "TEST", time.Hour); err == nil {
				t.Fatal("expected an error rather than a silently corrupted series")
			}
		})
	}
}

// TestLoadCSV_RejectsPartialFunding: a half-filled funding column is worse
// than an absent one, because the missing rows arrive as zeros and read as a
// neutral funding rate rather than as no data.
func TestLoadCSV_RejectsPartialFunding(t *testing.T) {
	content := `time,open,high,low,close,volume,buy_volume,sell_volume,funding
2024-01-01T00:00:00Z,100,101,99,100.5,10,6,4,0.0001
2024-01-01T01:00:00Z,100.5,102,100,101,12,7,5
2024-01-01T02:00:00Z,101,103,100,102,12,7,5,0.0002
`
	_, err := LoadCSV(writeTemp(t, "partial.csv", content), "TEST", time.Hour)
	if err == nil {
		t.Fatal("expected a partially populated funding column to be rejected")
	}
	if !strings.Contains(err.Error(), "funding") {
		t.Fatalf("error %q should name the funding column", err)
	}
}

func TestLoadLaunches_ReadsTheExampleFile(t *testing.T) {
	launches, err := LoadLaunches("examples/launches.json")
	if err != nil {
		t.Fatalf("load examples: %v", err)
	}
	if len(launches) < 4 {
		t.Fatalf("got %d launches, want the full example set", len(launches))
	}

	a := NewLaunchAgent()
	byName := map[string]LaunchScreen{}
	for _, l := range launches {
		byName[l.Symbol] = a.Screen(l)
	}

	if got := byName["SAFEMOON2"].Verdict; got != Reject {
		t.Fatalf("the textbook rug screened as %s", got)
	}
	// The example that exists to make the point: flawless liquidity, holders
	// and volume, disqualified by one upgradeable proxy.
	if got := byName["LOOKSGOOD"].Verdict; got != Reject {
		t.Fatalf("a launch with excellent metrics and an upgradeable proxy screened as %s — "+
			"good numbers must not outvote a veto", got)
	}
	if got := byName["STEADY"].Verdict; got != Accept {
		t.Fatalf("the clean example screened as %s", got)
	}
}
