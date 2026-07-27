package marketsignals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig_ReadsTheShippedExample(t *testing.T) {
	cfg, err := LoadConfig("config.example.json")
	if err != nil {
		t.Fatalf("the example config does not load: %v", err)
	}
	if len(cfg.Universe) < 8 {
		t.Fatalf("example universe has %d names; the cross-sectional allocator needs at "+
			"least 8 to rank anything", len(cfg.Universe))
	}
	if cfg.Interval.Duration() != time.Hour {
		t.Fatalf("interval %s, want 1h", cfg.Interval.Duration())
	}
	if cfg.Execution == nil || cfg.Execution.Broker != "paper" {
		t.Fatal("the shipped example must not name a broker that trades money")
	}
	// The example deliberately mixes winners and names that have collapsed, so
	// the universe is not purely survivors.
	sectors := map[CryptoSector]int{}
	for _, i := range cfg.Universe {
		sectors[i.Sector]++
	}
	if len(sectors) < 3 {
		t.Fatalf("example universe spans %d sectors; a single-sector book is one bet", len(sectors))
	}
}

func TestConfig_ValidateRejectsWhatWouldCostMoney(t *testing.T) {
	base := func() Config {
		c := DefaultConfig()
		c.Universe = []InstrumentConfig{{
			Symbol: "BTCUSDT", Class: ClassCrypto, Sector: SectorMajor,
			Venue: VenueCEX, ContinuousTrading: true,
		}}
		return c
	}

	cases := map[string]func(*Config){
		"empty universe":     func(c *Config) { c.Universe = nil },
		"duplicate symbol":   func(c *Config) { c.Universe = append(c.Universe, c.Universe[0]) },
		"zero target vol":    func(c *Config) { c.Risk.TargetVol = 0 },
		"drawdown past 100%": func(c *Config) { c.Risk.MaxDrawdown = 1.5 },
		"unknown broker": func(c *Config) {
			c.Execution = &ExecutionConfig{Broker: "binance-live", EquityUSD: 1000,
				MaxPositionFraction: 0.2, MaxOrderFraction: 0.1}
		},
		"order cap above position cap": func(c *Config) {
			c.Execution = &ExecutionConfig{Broker: "paper", EquityUSD: 1000,
				MaxPositionFraction: 0.1, MaxOrderFraction: 0.5}
		},
		"zero equity": func(c *Config) {
			c.Execution = &ExecutionConfig{Broker: "paper", EquityUSD: 0,
				MaxPositionFraction: 0.2, MaxOrderFraction: 0.1}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := base()
			mutate(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("expected the config to be rejected rather than defaulted")
			}
		})
	}

	if err := base().Validate(); err != nil {
		t.Fatalf("a sound config was rejected: %v", err)
	}
}

// TestConfig_HasNoPlaceToPutAToken. For every provider worth notifying
// through, the endpoint carries a bot token, and a token in a config file is a
// token in version control.
func TestConfig_HasNoPlaceToPutAToken(t *testing.T) {
	b, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	src := string(b)
	if strings.Contains(src, `json:"url"`) || strings.Contains(src, `json:"token"`) {
		t.Fatal("NotifyConfig has a field for a URL or token; those belong in the " +
			"environment, not in a file that gets committed")
	}
	if !strings.Contains(src, "NOTIFY_URL") {
		t.Fatal("the config should document where the endpoint actually comes from")
	}
}

func TestConfig_DurationReadsAsAString(t *testing.T) {
	var d Duration
	if err := d.UnmarshalJSON([]byte(`"15m"`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Duration() != 15*time.Minute {
		t.Fatalf("got %s, want 15m", d.Duration())
	}
	if err := d.UnmarshalJSON([]byte(`900000000000`)); err == nil {
		t.Fatal("a bare nanosecond count should be rejected; nobody can check it at a glance")
	}
	b, err := d.MarshalJSON()
	if err != nil || string(b) != `"15m0s"` {
		t.Fatalf("marshalled as %s (%v)", b, err)
	}
}

func TestConfig_UniverseFromReportsMissingBars(t *testing.T) {
	dir := t.TempDir()
	// Two configured instruments, one file on disk.
	s := trendSeries(300, 1, 0.004)
	var sb strings.Builder
	sb.WriteString("time,open,high,low,close,volume\n")
	for _, c := range s.Candles {
		sb.WriteString(strings.Join([]string{
			itoa(int(c.Time.Unix())), f2(c.Open), f2(c.High), f2(c.Low), f2(c.Close), "1000",
		}, ",") + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "AAA.csv"), []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := DefaultConfig()
	for _, sym := range []string{"AAA", "BBB"} {
		cfg.Universe = append(cfg.Universe, InstrumentConfig{
			Symbol: sym, Class: ClassCrypto, Sector: SectorMajor,
			Venue: VenueCEX, ContinuousTrading: true,
		})
	}

	u, err := cfg.UniverseFrom(dir)
	if err == nil {
		t.Fatal("a book quietly missing half its universe must not load silently")
	}
	if !strings.Contains(err.Error(), "BBB") {
		t.Fatalf("error %q should name the missing instrument", err)
	}
	if u == nil || len(u.Series) != 1 {
		t.Fatal("what did load should still be returned, so the caller can decide")
	}
}
