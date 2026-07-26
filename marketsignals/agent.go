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
