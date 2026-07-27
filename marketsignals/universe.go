package marketsignals

import "fmt"

// AssetClass is the coarsest split, and it is a real one rather than a
// taxonomy for its own sake: the classes differ in when they trade, what a
// round trip costs, and which agents are even applicable.
//
// The most consequential difference is trading hours. An equity or ETF market
// is closed most of the time, so a large share of its total price movement
// happens in the overnight gap — where no stop protects you and no signal can
// be acted on. A crypto perpetual never closes and has no gap, but pays
// funding and can lose most of its liquidity in an hour. Treating these with
// one set of assumptions gets one of them badly wrong.
type AssetClass string

const (
	ClassEquity ClassOf = "equity"
	ClassETF    ClassOf = "etf"
	ClassCrypto ClassOf = "crypto"
)

// ClassOf aliases AssetClass so the constants above read naturally.
type ClassOf = AssetClass

// CryptoSector subdivides crypto, because "crypto" is not one market. Bitcoin
// and a two-day-old memecoin share a settlement layer and nothing else: not
// their volatility, not their liquidity, not their cost of trading, not the
// question you should be asking about them.
//
// The sector is what selects an expert profile below, and the profiles differ
// far more than newcomers expect — a strategy calibrated on BTC will size a
// memecoin position roughly ten times too large.
type CryptoSector string

const (
	SectorNone       CryptoSector = ""
	SectorMajor      CryptoSector = "major"     // BTC, ETH
	SectorLargeAlt   CryptoSector = "large_alt" // liquid top-50 names
	SectorDeFi       CryptoSector = "defi"
	SectorInfra      CryptoSector = "infra"     // L1s, L2s, DePIN, oracles
	SectorNarrative  CryptoSector = "narrative" // AI, RWA, gaming — theme-driven
	SectorMeme       CryptoSector = "meme"
	SectorNewLaunch  CryptoSector = "new_launch" // days old, no usable history
	SectorStablecoin CryptoSector = "stablecoin"
)

// VenueKind is where the instrument actually trades, which decides what the
// data can tell you. A centralised order book reports a taker split; an AMM
// pool reports swaps against a curve, where "volume" includes the arbitrage
// that exists only to correct the pool and tells you nothing about demand.
type VenueKind string

const (
	VenueCEX      VenueKind = "cex"
	VenueDEX      VenueKind = "dex"
	VenueExchange VenueKind = "exchange" // regulated equity/ETF venue
)

// Instrument identifies one tradeable thing precisely enough to choose how to
// treat it.
type Instrument struct {
	Symbol string       `json:"symbol"`
	Name   string       `json:"name"`
	Class  AssetClass   `json:"class"`
	Sector CryptoSector `json:"sector,omitempty"`
	Venue  VenueKind    `json:"venue"`

	// Chain and Address identify an on-chain asset. Empty for CEX and
	// exchange-listed instruments.
	Chain   string `json:"chain,omitempty"`
	Address string `json:"address,omitempty"`

	// HasPerpetual marks an instrument with a funding-bearing perpetual, the
	// precondition for the positioning agent to have any input at all.
	HasPerpetual bool `json:"has_perpetual"`

	// ContinuousTrading is true for markets with no closing bell. Its absence
	// is what makes overnight gap risk real, and it changes how a stop should
	// be thought about far more than most parameters do.
	ContinuousTrading bool `json:"continuous_trading"`

	// MedianDailyVolumeUSD drives the cost model. A signal that is excellent
	// on a $2bn/day name can be untradeable on a $200k/day one, and the
	// difference is not visible anywhere in the price series.
	MedianDailyVolumeUSD float64 `json:"median_daily_volume_usd,omitempty"`
}

// Validate catches instrument descriptions that would silently select the
// wrong expert profile.
func (i Instrument) Validate() error {
	switch i.Class {
	case ClassEquity, ClassETF:
		if i.Sector != SectorNone {
			return fmt.Errorf("%s: crypto sector %q set on a %s", i.Symbol, i.Sector, i.Class)
		}
		if i.ContinuousTrading {
			return fmt.Errorf("%s: %s markets close; ContinuousTrading must be false", i.Symbol, i.Class)
		}
	case ClassCrypto:
		if i.Sector == SectorNone {
			return fmt.Errorf("%s: crypto instrument needs a sector — 'crypto' is not one market",
				i.Symbol)
		}
		if i.Venue == VenueExchange {
			return fmt.Errorf("%s: crypto does not trade on a regulated equity venue", i.Symbol)
		}
	default:
		return fmt.Errorf("%s: unknown asset class %q", i.Symbol, i.Class)
	}
	if i.Venue == VenueDEX && (i.Chain == "" || i.Address == "") {
		return fmt.Errorf("%s: a DEX instrument needs a chain and a contract address", i.Symbol)
	}
	return nil
}

// String renders the instrument for reports.
func (i Instrument) String() string {
	if i.Sector != SectorNone {
		return fmt.Sprintf("%s (%s/%s on %s)", i.Symbol, i.Class, i.Sector, i.Venue)
	}
	return fmt.Sprintf("%s (%s on %s)", i.Symbol, i.Class, i.Venue)
}
