package marketsignals

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Configuration.
//
// A dozen command-line flags is a demo, not a product. What you trade, what you
// assume it costs, and what you will not let it do are decisions worth writing
// down, reviewing, and putting in version control — not retyping into a shell
// each time and hoping the flags match yesterday's.
//
// Every field has a conservative default, and the ones that could quietly cost
// money have no default at all: Validate refuses a config rather than
// inventing a pool size, an account size, or a broker.

// Config is a complete description of one deployment.
type Config struct {
	// Universe is what to trade. At least one instrument; a cross-sectional
	// book needs at least MinNames of them.
	Universe []InstrumentConfig `json:"universe"`

	// Interval is the bar length, in Go duration notation ("1h", "15m", "4h").
	Interval Duration `json:"interval"`

	// DataDir is where bars CSVs live, named <SYMBOL>.csv.
	DataDir string `json:"data_dir"`

	// Risk applies to every instrument unless its profile overrides.
	Risk RiskConfig `json:"risk"`

	// Execution governs the trading engine. Absent means signal-only.
	Execution *ExecutionConfig `json:"execution,omitempty"`

	// Notify configures where signals are sent.
	Notify NotifyConfig `json:"notify"`

	// Hiring is the bar a strategy must clear before it is considered
	// deployable.
	Hiring HiringCriteria `json:"hiring"`
}

// InstrumentConfig describes one tradeable thing.
type InstrumentConfig struct {
	Symbol string       `json:"symbol"`
	Class  AssetClass   `json:"class"`
	Sector CryptoSector `json:"sector,omitempty"`
	Venue  VenueKind    `json:"venue"`

	Chain   string `json:"chain,omitempty"`
	Address string `json:"address,omitempty"`

	HasPerpetual      bool `json:"has_perpetual"`
	ContinuousTrading bool `json:"continuous_trading"`

	PoolLiquidityUSD float64 `json:"pool_liquidity_usd,omitempty"`
	AccountUSD       float64 `json:"account_usd,omitempty"`
}

// Instrument converts the config entry.
func (i InstrumentConfig) Instrument() Instrument {
	return Instrument{
		Symbol: i.Symbol, Class: i.Class, Sector: i.Sector, Venue: i.Venue,
		Chain: i.Chain, Address: i.Address,
		HasPerpetual: i.HasPerpetual, ContinuousTrading: i.ContinuousTrading,
		PoolLiquidityUSD: i.PoolLiquidityUSD, AccountUSD: i.AccountUSD,
	}
}

// RiskConfig is the account-level risk policy.
type RiskConfig struct {
	// TargetVol is the annualised volatility target for the book.
	TargetVol float64 `json:"target_vol"`
	// MaxGross and MaxNet cap total and directional exposure.
	MaxGross float64 `json:"max_gross"`
	MaxNet   float64 `json:"max_net"`
	// MaxDrawdown flattens and halts.
	MaxDrawdown float64 `json:"max_drawdown"`
}

// ExecutionConfig governs the trading engine.
type ExecutionConfig struct {
	// Broker names the venue. Only "paper" is implemented here; anything else
	// is rejected rather than silently ignored, because a config naming a
	// broker that does not exist should fail loudly rather than trade nothing
	// while appearing to trade.
	Broker string `json:"broker"`
	// EquityUSD is the account size.
	EquityUSD float64 `json:"equity_usd"`

	MaxPositionFraction  float64 `json:"max_position_fraction"`
	MaxOrderFraction     float64 `json:"max_order_fraction"`
	MaxDailyLossFraction float64 `json:"max_daily_loss_fraction"`

	// DryRun computes orders and sends none.
	DryRun bool `json:"dry_run"`
}

// Rails converts the config into engine limits.
func (e ExecutionConfig) Rails(maxDrawdown float64) Rails {
	return Rails{
		MaxPositionFraction:  e.MaxPositionFraction,
		MaxOrderFraction:     e.MaxOrderFraction,
		MaxDailyLossFraction: e.MaxDailyLossFraction,
		MaxDrawdownFraction:  maxDrawdown,
		DryRun:               e.DryRun,
	}
}

// NotifyConfig describes where signals go.
//
// The URL is deliberately NOT here. For every provider worth using it carries
// a bot token, and a token in a config file is a token in version control. It
// is read from the NOTIFY_URL environment variable instead, and this struct
// holds only the parts that are safe to commit.
type NotifyConfig struct {
	// Field is the JSON key the message goes into: "text" for Telegram and
	// Slack, "content" for Discord, empty for the raw signal.
	Field string `json:"field"`
	// ChatID is Telegram's chat identifier — not secret on its own.
	ChatID string `json:"chat_id,omitempty"`
	// MinChange is the position move worth a message, as a fraction of equity.
	MinChange float64 `json:"min_change"`
}

// Duration is a time.Duration that reads from JSON as "1h" rather than as a
// count of nanoseconds nobody can check at a glance.
type Duration time.Duration

// UnmarshalJSON parses a Go duration string.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("interval must be a string like \"1h\": %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// MarshalJSON writes the duration as a string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Duration returns the underlying value.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

// DefaultConfig is a conservative, signal-only starting point.
func DefaultConfig() Config {
	return Config{
		Interval: Duration(time.Hour),
		DataDir:  "data",
		Risk: RiskConfig{
			TargetVol: 0.20, MaxGross: 1.0, MaxNet: 0.5, MaxDrawdown: 0.15,
		},
		Notify: NotifyConfig{Field: "text", MinChange: 0.20},
		Hiring: DefaultHiringCriteria(),
	}
}

// LoadConfig reads and validates a config file.
func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Validate rejects a config rather than filling in a number that could cost
// money.
func (c Config) Validate() error {
	if len(c.Universe) == 0 {
		return fmt.Errorf("universe is empty; there is nothing to trade")
	}
	seen := map[string]bool{}
	for _, ic := range c.Universe {
		if seen[ic.Symbol] {
			return fmt.Errorf("symbol %q appears twice — a duplicate silently doubles its "+
				"weight in every book", ic.Symbol)
		}
		seen[ic.Symbol] = true
		if err := ic.Instrument().Validate(); err != nil {
			return err
		}
	}
	if c.Interval.Duration() <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	if c.Risk.TargetVol <= 0 {
		return fmt.Errorf("risk.target_vol must be positive; zero means never take a position")
	}
	if c.Risk.MaxGross <= 0 || c.Risk.MaxDrawdown <= 0 {
		return fmt.Errorf("risk.max_gross and risk.max_drawdown must be positive")
	}
	if c.Risk.MaxDrawdown >= 1 {
		return fmt.Errorf("risk.max_drawdown of %.2f means the stop fires only after the "+
			"account is gone", c.Risk.MaxDrawdown)
	}

	if e := c.Execution; e != nil {
		if e.Broker != "paper" {
			return fmt.Errorf("execution.broker %q is not implemented; only \"paper\" exists "+
				"here, and a broker that trades money is a deliberate code change by whoever "+
				"holds the credentials", e.Broker)
		}
		if e.EquityUSD <= 0 {
			return fmt.Errorf("execution.equity_usd must be positive")
		}
		if e.MaxPositionFraction <= 0 || e.MaxOrderFraction <= 0 {
			return fmt.Errorf("execution position and order caps must be positive; zero " +
				"disables trading in a way that looks like a running bot")
		}
		if e.MaxOrderFraction > e.MaxPositionFraction {
			return fmt.Errorf("execution.max_order_fraction (%.2f) exceeds "+
				"max_position_fraction (%.2f), so a single order could breach the position cap",
				e.MaxOrderFraction, e.MaxPositionFraction)
		}
	}
	return nil
}

// UniverseFrom loads every configured instrument's bars from DataDir.
func (c Config) UniverseFrom(dir string) (*Universe, error) {
	if dir == "" {
		dir = c.DataDir
	}
	u := &Universe{Series: map[string]*Series{}}
	var missing []string

	for _, ic := range c.Universe {
		path := fmt.Sprintf("%s/%s.csv", dir, ic.Symbol)
		s, err := LoadCSV(path, ic.Symbol, c.Interval.Duration())
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, ic.Symbol)
				continue
			}
			return nil, err
		}
		u.Series[ic.Symbol] = s
		u.Instruments = append(u.Instruments, ic.Instrument())
	}

	if len(u.Series) == 0 {
		return nil, fmt.Errorf("no bars found in %s for any configured instrument (missing: %v); "+
			"see %s/README.md", dir, missing, dir)
	}
	if len(missing) > 0 {
		// Not fatal, but never silent: a book quietly missing half its
		// universe produces a plausible result about a different strategy.
		return u, fmt.Errorf("loaded %d of %d instruments; no bars for %v",
			len(u.Series), len(c.Universe), missing)
	}
	return u, nil
}
