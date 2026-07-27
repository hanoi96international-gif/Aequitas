package marketsignals

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// SocialSnapshot is one bar's worth of social activity for an instrument —
// posts on X or an equivalent public feed, aggregated over exactly the window
// [Candle.Time, Candle.Time+Interval).
//
// The quality fields exist because raw mention counts are worthless. Crypto
// social metrics are the most trivially purchasable numbers in this entire
// package: engagement farms sell mentions by the thousand, and a token can
// look like it is trending for a few hundred dollars. Counting posts measures
// somebody's marketing budget. What is harder to fake — and therefore what
// this type is built around — is a large number of DISTINCT, established
// accounts saying DIFFERENT things.
type SocialSnapshot struct {
	Time          time.Time `json:"time"`
	Posts         int       `json:"posts"`
	UniqueAuthors int       `json:"unique_authors"`

	// DuplicateTextRatio is the share of posts that are near-copies of
	// another post in the same window — the signature of a copy-paste
	// campaign.
	DuplicateTextRatio float64 `json:"duplicate_text_ratio"`
	// MedianAuthorAgeDays is the median age of posting accounts. A swarm of
	// accounts created last week is a swarm of one person.
	MedianAuthorAgeDays float64 `json:"median_author_age_days"`
	// NewAccountRatio is the share of authors younger than a month.
	NewAccountRatio float64 `json:"new_account_ratio"`

	Likes   int `json:"likes"`
	Reposts int `json:"reposts"`
}

// Credibility scores how much of this window's activity looks like genuine
// independent interest, on [0,1]. It multiplies rather than averages its
// components on purpose: these are all ways of faking attention, and a
// campaign that fails any one of them should not be rescued by scoring well
// on the others.
func (s SocialSnapshot) Credibility() float64 {
	if s.Posts <= 0 {
		return 0
	}
	// Distinct voices. Twenty accounts posting five times each is not a
	// hundred people talking.
	diversity := clamp(float64(s.UniqueAuthors)/float64(s.Posts), 0, 1)
	// Original wording.
	originality := clamp(1-s.DuplicateTextRatio, 0, 1)
	// Established accounts. Saturates at 180 days: older than that carries no
	// extra information about whether the account is real.
	maturity := clamp(s.MedianAuthorAgeDays/180, 0, 1)
	// Not a freshly minted swarm.
	organic := clamp(1-s.NewAccountRatio, 0, 1)

	return diversity * originality * maturity * organic
}

// CredibleAttention is the window's post count discounted by its credibility
// — the only attention figure anything downstream should use.
func (s SocialSnapshot) CredibleAttention() float64 {
	return float64(s.Posts) * s.Credibility()
}

// SocialAgent reads attention, and it reads it mostly as a warning.
//
// The intuition that hype precedes price is backwards for anything with a
// public feed. By the time an asset is trending, the move that created the
// attention has already happened — the attention IS the move, observed after
// the fact — and the people posting are frequently the people who need
// somebody to sell to. Buying peak attention is buying from the person who
// manufactured it.
//
// So the agent has two modes, and the contrarian one is the load-bearing half:
//
//   - EXTREME credible attention (top percentile of its own history) is read
//     as distribution and leans short, or at minimum takes risk off.
//   - Sharp ACCELERATION that has not yet reached that extreme, with high
//     credibility, leans long — but only mildly. This is the half that is
//     genuinely hard, and the half most such systems get wrong by mistaking a
//     completed move for a starting one.
//
// If credibility is low, the agent stands down entirely rather than picking a
// side. Manufactured attention says nothing about direction; it says only
// that somebody paid for it.
type SocialAgent struct {
	Lookback int // window defining what is extreme FOR THIS ASSET
	// MinCredibility is the floor below which the feed is treated as bought
	// and the agent refuses to act on it at all.
	MinCredibility float64
	// ExtremePct is the percentile of credible attention above which the
	// reading flips contrarian.
	ExtremePct float64
	// MaxEntryPct is the ceiling for the constructive reading: attention may
	// be accelerating hard, but if it has already reached this percentile the
	// move is not early any more and the contrarian logic owns the range.
	//
	// This is a ceiling on the RANK while the trigger is on the ACCELERATION,
	// and the two must not be set to contradict each other. An earlier
	// version gated the constructive branch on the rank being LOW — below the
	// 60th percentile — which is unsatisfiable in combination with a 3x
	// acceleration trigger: if 40% of the window exceeded a value three times
	// the window's own mean, that mean would have to exceed itself. The
	// branch was unreachable, which in a signal agent is worse than a wrong
	// branch, because nothing ever fails and the dead code reads as working.
	MaxEntryPct float64
	// MinAcceleration is the multiple of the trailing mean that counts as a
	// genuine surge.
	MinAcceleration float64
}

// NewSocialAgent returns the agent with conservative settings.
func NewSocialAgent() *SocialAgent {
	return &SocialAgent{
		Lookback:        168, // a week of hourly bars
		MinCredibility:  0.15,
		ExtremePct:      0.97,
		MaxEntryPct:     0.90,
		MinAcceleration: 3.0,
	}
}

func (a *SocialAgent) Name() string   { return "social" }
func (a *SocialAgent) Family() Family { return FamilySocial }

func (a *SocialAgent) Warmup() int { return a.Lookback + 1 }

func (a *SocialAgent) Evaluate(v View) Signal {
	if v.Len() < a.Warmup() {
		return Flatf(a.Name(), a.Family(), "warming up (%d/%d bars)", v.Len(), a.Warmup())
	}
	snaps := v.Social()
	if snaps == nil {
		return Flatf(a.Name(), a.Family(), "series carries no social data")
	}

	last := snaps[len(snaps)-1]
	cred := last.Credibility()
	if cred < a.MinCredibility {
		return Flatf(a.Name(), a.Family(),
			"attention looks manufactured (credibility %s: %d posts from %d authors, %s duplicates, %s new accounts) — "+
				"that says who paid, not which way to lean",
			f2(cred), last.Posts, last.UniqueAuthors, pct(last.DuplicateTextRatio), pct(last.NewAccountRatio))
	}

	series := make([]float64, len(snaps))
	for i, s := range snaps {
		series[i] = s.CredibleAttention()
	}
	rank := PercentileRank(series, a.Lookback)
	if math.IsNaN(rank) {
		return Flatf(a.Name(), a.Family(), "attention distribution undefined")
	}

	baseline := SMA(series[:len(series)-1], a.Lookback-1)
	accel := 0.0
	if baseline > 0 {
		accel = series[len(series)-1] / baseline
	}

	switch {
	case rank >= a.ExtremePct:
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Short,
			Strength: clamp((rank-a.ExtremePct)/(1-a.ExtremePct), 0, 1) * cred,
			Note: fmt.Sprintf("credible attention at the %s percentile of its own history — "+
				"at peak attention the crowd is usually the exit liquidity, not the buyer", pct(rank)),
		}
	case rank <= a.MaxEntryPct && accel >= a.MinAcceleration:
		return Signal{
			Agent: a.Name(), Family: a.Family(), Dir: Long,
			// Deliberately capped low. Early attention is the weakest claim
			// this package makes and it should never carry a position on its
			// own.
			Strength: clamp((accel/a.MinAcceleration-1)*0.5, 0, 0.5) * cred,
			Note: fmt.Sprintf("credible attention %sx its weekly baseline and not yet extreme "+
				"(%s percentile), credibility %s", f2(accel), pct(rank), f2(cred)),
		}
	default:
		return Flatf(a.Name(), a.Family(), "attention unremarkable (%s percentile, %sx baseline)",
			pct(rank), f2(accel))
	}
}

// ModulateRisk cuts size when attention is extreme regardless of which way the
// price agents are leaning. A market that everyone is watching is a market
// where the next headline moves it further than your stop, and that is a
// reason to be smaller whichever direction you favour.
func (a *SocialAgent) ModulateRisk(v View) RiskAdjustment {
	if v.Len() < a.Warmup() {
		return NoAdjustment(a.Name(), "insufficient history")
	}
	snaps := v.Social()
	if snaps == nil {
		return NoAdjustment(a.Name(), "no social data")
	}
	series := make([]float64, len(snaps))
	for i, s := range snaps {
		series[i] = s.CredibleAttention()
	}
	rank := PercentileRank(series, a.Lookback)
	if math.IsNaN(rank) || rank < a.ExtremePct {
		return NoAdjustment(a.Name(), "attention within its normal range")
	}
	return RiskAdjustment{
		Scale:  0.5,
		Source: a.Name(),
		Reason: fmt.Sprintf("attention at the %s percentile — headline risk is elevated in "+
			"both directions", pct(rank)),
	}
}

// ── Discovery ────────────────────────────────────────────────────────────

// ProjectAttention is one candidate project's social standing, used to find
// what is being talked about before deciding whether any of it is worth
// screening.
type ProjectAttention struct {
	Instrument Instrument     `json:"instrument"`
	Current    SocialSnapshot `json:"current"`
	// BaselineAttention is the project's own trailing average credible
	// attention, so that a surge is measured against its own history rather
	// than against a large-cap's.
	BaselineAttention float64 `json:"baseline_attention"`
}

// HypeCandidate is a ranked discovery result.
type HypeCandidate struct {
	ProjectAttention
	CredibleAttention float64
	Credibility       float64
	Acceleration      float64
	// Note states what the ranking does and does not mean.
	Note string
}

// RankByCredibleAttention orders candidates by how much genuine, accelerating
// interest they are attracting.
//
// What this produces is a WATCHLIST, and the distinction matters more than the
// ranking does. A high position here means an asset is worth putting through
// the launch screener and the expert profile for its sector — it is not a
// reason to buy anything. Ranking by attention and then buying the top of the
// list is a mechanical way of always arriving last, which is precisely how
// social-driven trading loses money.
func RankByCredibleAttention(candidates []ProjectAttention) []HypeCandidate {
	out := make([]HypeCandidate, 0, len(candidates))
	for _, c := range candidates {
		cred := c.Current.Credibility()
		attn := c.Current.CredibleAttention()
		accel := 0.0
		if c.BaselineAttention > 0 {
			accel = attn / c.BaselineAttention
		}
		hc := HypeCandidate{
			ProjectAttention:  c,
			CredibleAttention: attn,
			Credibility:       cred,
			Acceleration:      accel,
		}
		switch {
		case cred < 0.15:
			hc.Note = "attention appears bought — worth ignoring rather than investigating"
		case accel >= 5:
			hc.Note = "sharp rise in credible attention — screen it; do not buy the ranking"
		case accel >= 2:
			hc.Note = "building credible attention"
		default:
			hc.Note = "steady"
		}
		out = append(out, hc)
	}
	// Rank by credible attention weighted by how fast it is growing, so an
	// established name with flat interest does not permanently occupy the top.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CredibleAttention*math.Max(out[i].Acceleration, 1) >
			out[j].CredibleAttention*math.Max(out[j].Acceleration, 1)
	})
	return out
}

// Report renders a discovery ranking.
func RankReport(cands []HypeCandidate) string {
	out := "Discovery watchlist — ranked by credible, accelerating attention\n\n"
	for i, c := range cands {
		out += fmt.Sprintf("%2d. %-28s attention %8.0f  credibility %.2f  %.1fx baseline\n",
			i+1, c.Instrument.String(), c.CredibleAttention, c.Credibility, c.Acceleration)
		out += "    " + c.Note + "\n"
	}
	out += "\nThis is a list of things to INVESTIGATE. Attention is an effect of a move\n" +
		"far more often than a cause of one, so buying the top of this ranking is a\n" +
		"reliable way to arrive after the people who created it.\n"
	return out
}
