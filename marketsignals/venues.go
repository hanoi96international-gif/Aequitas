package marketsignals

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Data sources.
//
// This file defines what the framework needs from the outside world and
// ships one implementation: files on disk. There is deliberately no live
// adapter for X, for a centralised exchange, or for a DEX aggregator, and
// the omission is not an oversight to be filled in later by whoever reads
// this — it is where the honest boundary of this package falls.
//
// Every such adapter needs credentials, and a credential is a decision about
// what may act on someone's behalf. A library that quietly grows the ability
// to authenticate, poll a venue and read an account has changed what it is.
// Writing one is a small job for whoever holds the keys; guessing at it here
// would produce code that cannot be tested, cannot be trusted, and whose
// failure mode is a silent stream of wrong prices.
//
// What the interfaces below DO give you is the shape such an adapter must
// satisfy, including the normalisation problems that make venue data
// non-interchangeable — which is the part that is genuinely easy to get
// wrong and expensive to discover later.

// BarSource supplies price history for an instrument.
//
// The contract that matters: bars must be CLOSED. A partially formed current
// bar looks exactly like a closed one to every agent here, and feeding it in
// means an agent decides on a close that has not happened, then gets filled
// at a price derived from it. That is lookahead entering through the data
// layer, where none of this package's structural defences can see it.
type BarSource interface {
	// Bars returns closed bars for the instrument over [from, to), oldest
	// first, at the requested interval.
	Bars(i Instrument, interval time.Duration, from, to time.Time) (*Series, error)
}

// LaunchSource supplies the on-chain facts the launch screener needs.
//
// Every field in Launch must come from an authority that cannot be edited by
// the project: an RPC node, a block explorer's verified-source flag, a
// simulated sell against the live pool. A source that relays a project's own
// self-description is not a LaunchSource, it is a press release with a JSON
// content type.
type LaunchSource interface {
	Launches(chain string, since time.Time) ([]Launch, error)
	Inspect(chain, address string) (Launch, error)
}

// SocialSource supplies attention data.
//
// The quality fields on SocialSnapshot are not optional extras. An
// implementation that fills Posts and leaves DuplicateTextRatio,
// MedianAuthorAgeDays and NewAccountRatio at zero produces snapshots whose
// Credibility() is zero, and the social agent will correctly refuse to act on
// them. That is the intended failure: an adapter that cannot assess whether
// attention is genuine should not be able to pass off raw counts as if it
// could.
type SocialSource interface {
	// Attention returns per-bar snapshots aligned to the same bar boundaries
	// as the price series they will accompany.
	Attention(i Instrument, interval time.Duration, from, to time.Time) ([]SocialSnapshot, error)
	// Trending returns candidate projects currently attracting attention,
	// for discovery rather than for trading.
	Trending(chain string, limit int) ([]ProjectAttention, error)
}

// ── Venue normalisation ──────────────────────────────────────────────────

// NormalizeDEXVolume corrects an AMM pool's reported volume for the share of
// it that is arbitrage.
//
// A constant-product pool does not have buyers and sellers in the sense a
// central limit order book does. A large part of its volume is bots pushing
// the pool back to the price it already has elsewhere, and that flow carries
// no information about demand whatsoever — it is a mechanical consequence of
// the pool having drifted. Feeding raw pool volume to the order-flow agent
// produces a confident reading of a number that is mostly robots correcting
// each other.
//
// arbShare is the estimated fraction of volume that is arbitrage. It is a
// judgement the caller must make from their own indexer; there is no way to
// derive it from OHLCV, and defaulting it to zero would silently assert that
// none of the volume is arbitrage, which is never true.
func NormalizeDEXVolume(volume, arbShare float64) float64 {
	return volume * clamp(1-arbShare, 0, 1)
}

// CrossVenuePrice reports whether an instrument's price differs materially
// between two venues, and by how much.
//
// A persistent gap between a CEX quote and a DEX pool is not free money and
// should not be read as one: it is usually the cost of bridging, the
// withdrawal queue, or a pool too thin for the size that would close it. What
// it IS useful for is data hygiene — a large divergence means at least one of
// the two feeds is not describing the market you think you are trading, and
// any signal computed from it is suspect.
func CrossVenuePrice(cexPrice, dexPrice float64) (spreadFraction float64, material bool) {
	if cexPrice <= 0 || dexPrice <= 0 {
		return 0, false
	}
	spread := (dexPrice - cexPrice) / cexPrice
	return spread, absf(spread) > 0.01
}

// ── File-backed source ───────────────────────────────────────────────────

// FileSource reads everything from a directory on disk, which is what makes
// the whole framework runnable and testable without a single credential.
//
// Layout:
//
//	<dir>/bars/<SYMBOL>.csv        time,open,high,low,close,volume[,buy,sell][,funding]
//	<dir>/social/<SYMBOL>.json     [] of SocialSnapshot
//	<dir>/events/<SYMBOL>.json     [] of Event
//	<dir>/launches/<chain>.json    [] of Launch
//	<dir>/trending/<chain>.json    [] of ProjectAttention
type FileSource struct {
	Dir string
}

// Bars loads a series and attaches its social and event files when present.
// The from/to window filters the loaded bars; a file need not match it.
func (f FileSource) Bars(i Instrument, interval time.Duration, from, to time.Time) (*Series, error) {
	s, err := LoadCSV(fmt.Sprintf("%s/bars/%s.csv", f.Dir, i.Symbol), i.Symbol, interval)
	if err != nil {
		return nil, err
	}

	if events, err := loadJSON[Event](fmt.Sprintf("%s/events/%s.json", f.Dir, i.Symbol)); err == nil {
		s.Events = events
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if social, err := loadJSON[SocialSnapshot](fmt.Sprintf("%s/social/%s.json", f.Dir, i.Symbol)); err == nil {
		if len(social) != len(s.Candles) {
			return nil, fmt.Errorf("%s: %d social snapshots for %d bars — they must align "+
				"index for index or an agent reads one bar's attention against another's price",
				i.Symbol, len(social), len(s.Candles))
		}
		s.Social = social
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if !from.IsZero() || !to.IsZero() {
		s = windowSeries(s, from, to)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	for _, e := range s.Events {
		if err := e.Validate(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Launches reads the chain's launch file and filters by first-seen age.
func (f FileSource) Launches(chain string, since time.Time) ([]Launch, error) {
	all, err := loadJSON[Launch](fmt.Sprintf("%s/launches/%s.json", f.Dir, chain))
	if err != nil {
		return nil, err
	}
	if since.IsZero() {
		return all, nil
	}
	maxAge := time.Since(since).Minutes()
	var out []Launch
	for _, l := range all {
		if l.AgeMinutes <= maxAge {
			out = append(out, l)
		}
	}
	return out, nil
}

// Inspect finds one token in the chain's launch file.
func (f FileSource) Inspect(chain, address string) (Launch, error) {
	all, err := f.Launches(chain, time.Time{})
	if err != nil {
		return Launch{}, err
	}
	for _, l := range all {
		if l.TokenAddress == address {
			return l, nil
		}
	}
	return Launch{}, fmt.Errorf("%s: no record of %s", chain, address)
}

// Attention loads per-bar social snapshots.
func (f FileSource) Attention(i Instrument, _ time.Duration, from, to time.Time) ([]SocialSnapshot, error) {
	all, err := loadJSON[SocialSnapshot](fmt.Sprintf("%s/social/%s.json", f.Dir, i.Symbol))
	if err != nil {
		return nil, err
	}
	if from.IsZero() && to.IsZero() {
		return all, nil
	}
	var out []SocialSnapshot
	for _, s := range all {
		if (!from.IsZero() && s.Time.Before(from)) || (!to.IsZero() && !s.Time.Before(to)) {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// Trending loads the discovery candidates for a chain.
func (f FileSource) Trending(chain string, limit int) ([]ProjectAttention, error) {
	all, err := loadJSON[ProjectAttention](fmt.Sprintf("%s/trending/%s.json", f.Dir, chain))
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func loadJSON[T any](path string) ([]T, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []T
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

// windowSeries trims a series to [from, to), keeping every aligned side
// channel in step. Trimming the bars without trimming Social and Funding
// alongside them would leave an agent reading one bar's attention against
// another's price — a silent misalignment that produces plausible nonsense.
func windowSeries(s *Series, from, to time.Time) *Series {
	out := &Series{Symbol: s.Symbol, Interval: s.Interval, Events: s.Events}
	for i, c := range s.Candles {
		if !from.IsZero() && c.Time.Before(from) {
			continue
		}
		if !to.IsZero() && !c.Time.Before(to) {
			continue
		}
		out.Candles = append(out.Candles, c)
		if i < len(s.Funding) {
			out.Funding = append(out.Funding, s.Funding[i])
		}
		if i < len(s.Social) {
			out.Social = append(out.Social, s.Social[i])
		}
	}
	return out
}
