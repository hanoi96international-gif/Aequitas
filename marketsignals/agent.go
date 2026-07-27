package marketsignals

import "fmt"

// Direction is the side an agent wants to be on.
type Direction int

const (
	Short Direction = -1
	Flat  Direction = 0
	Long  Direction = 1
)

func (d Direction) String() string {
	switch d {
	case Long:
		return "long"
	case Short:
		return "short"
	default:
		return "flat"
	}
}

// Family is why an agent believes what it believes. The ensemble uses it to
// distinguish real corroboration from an echo: three trend agents agreeing is
// one opinion sampled three times, whereas a trend agent and a flow agent
// agreeing is two independent pieces of evidence. Weighting the first case
// like the second is the most common way an ensemble talks itself into a
// position.
type Family string

const (
	FamilyTrend       Family = "trend"
	FamilyReversion   Family = "reversion"
	FamilyFlow        Family = "flow"
	FamilyPositioning Family = "positioning"
	FamilyScreen      Family = "screen"

	// FamilyStructure covers everything read off the SHAPE of the price path:
	// Fibonacci retracements, chart patterns, trendlines, swing geometry.
	//
	// Fibonacci and chart patterns deliberately share this one family rather
	// than getting one each. They are computed from the same swing points by
	// the same logic and they agree with each other almost by construction —
	// a double bottom will usually sit on a retracement level because both
	// are describing the same low. Filing them separately would let the
	// ensemble count a single reading of the chart as two independent
	// confirmations, which is precisely the error the Family mechanism
	// exists to prevent.
	FamilyStructure Family = "structure"

	// FamilyMacro is the political and macroeconomic calendar — the only
	// family whose input is not derived from market data at all.
	FamilyMacro Family = "macro"
)

// Signal is an agent's opinion about the next bar. Strength is the agent's
// own conviction on [0,1] and is NOT a probability or an expected return —
// it is only comparable between evaluations of the same agent, which is why
// the ensemble normalises before combining.
type Signal struct {
	Agent    string
	Family   Family
	Dir      Direction
	Strength float64
	Note     string
}

// Flatf builds a no-position signal carrying the reason, so that a backtest
// log explains why an agent sat out instead of leaving a silent gap.
func Flatf(name string, fam Family, format string, args ...any) Signal {
	return Signal{Agent: name, Family: fam, Dir: Flat, Note: fmt.Sprintf(format, args...)}
}

// Agent turns a view of the past into an opinion about the next bar.
//
// Warmup is the number of bars an agent needs before Evaluate can mean
// anything; the backtester skips it rather than letting the agent emit
// signals from half-formed indicators, which otherwise front-loads a
// backtest with garbage trades that flatter or wreck the result at random.
type Agent interface {
	Name() string
	Family() Family
	Warmup() int
	Evaluate(v View) Signal
}

// SizedSignal is a Signal after the risk manager has turned conviction into
// an actual position, expressed as a fraction of equity. Negative is short.
type SizedSignal struct {
	Signal
	Target float64
	Reason string
}

// RiskAdjustment is an opinion about how much risk to carry, expressed
// without any opinion about direction.
//
// This separation is the point. Some information tells you which way to lean;
// other information tells you only that the next few hours are a bad time to
// have a view at all. A referendum result due in two hours does not make a
// long better or worse — it makes the SIZE of any position wrong, whichever
// way it points. Forcing that into a directional vote loses it entirely,
// which is why the macro agent modulates rather than votes.
type RiskAdjustment struct {
	// Scale multiplies the position the risk manager would otherwise take.
	// Constrained to [0,1]: a modulator may take risk off the table, never
	// add to it. Nothing on a calendar justifies betting bigger.
	Scale  float64
	Veto   bool
	Source string
	Reason string
}

// NoAdjustment leaves the position untouched.
func NoAdjustment(source, reason string) RiskAdjustment {
	return RiskAdjustment{Scale: 1, Source: source, Reason: reason}
}

// RiskModulator is implemented by anything that wants to shrink a position
// without expressing a direction. Strategies may implement it in addition to
// Strategy; the backtester applies it when present.
type RiskModulator interface {
	ModulateRisk(v View) RiskAdjustment
}
