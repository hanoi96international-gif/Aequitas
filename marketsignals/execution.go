package marketsignals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// Execution.
//
// Turning a target position into orders is the easy part and not where bots
// lose money. Three other things are, and each has a specific defence here.
//
// RECONCILIATION. The bot's belief about its position and the exchange's
// record of it diverge constantly: an order is rejected for minimum notional,
// a fill is partial, a position is liquidated, somebody trades the account by
// hand. A bot that trusts its own memory then sizes the next order from a
// position it does not have. So the engine re-reads the account before every
// decision and HALTS on a mismatch rather than trading through it. A halted
// bot costs you an opportunity; a bot trading from a wrong position costs you
// the account.
//
// IDEMPOTENCY. A network timeout does not mean the order was not placed — it
// means you do not know. Retrying blindly is how a position ends up doubled.
// Every order carries a deterministic client ID derived from what it is for,
// so a retry is the same order and the exchange rejects the duplicate; and on
// any uncertain outcome the engine re-reads the account instead of resending.
//
// RAILS. Limits that hold regardless of what the signal says: position size,
// single-order size, daily loss, and a drawdown stop that flattens. They exist
// for the case where the signal logic is wrong, which is the case you cannot
// test for in advance.
//
// Nothing here holds a credential. A Broker is supplied by the caller, and the
// default one trades nothing at all.

// Side is an order direction.
type Side string

const (
	Buy  Side = "buy"
	Sell Side = "sell"
)

// Order is an instruction to change position.
type Order struct {
	// ClientID is the idempotency key. It is derived from what the order is
	// FOR — instrument, bar, target — so a retry after an ambiguous failure
	// produces a byte-identical order that the venue can reject as a
	// duplicate rather than filling twice.
	ClientID string
	Symbol   string
	Side     Side
	// Quantity is in base units (BTC, not USD).
	Quantity float64
	// ReduceOnly asks the venue to refuse anything that would increase the
	// position. Set when closing, so a race cannot flip the position through
	// zero into a new one on the other side.
	ReduceOnly bool
}

// Fill is what actually happened.
type Fill struct {
	ClientID string
	Quantity float64 // base units actually filled
	Price    float64
	Fee      float64
}

// Position is a held position, signed: negative is short.
type Position struct {
	Symbol   string
	Quantity float64
	Entry    float64
}

// Account is the venue's view of the account, which is the only view that
// counts.
type Account struct {
	EquityUSD float64
	Positions map[string]Position
}

// Position returns the held quantity for a symbol, or zero.
func (a Account) Position(symbol string) float64 {
	return a.Positions[symbol].Quantity
}

// Broker is a venue. Implementations hold whatever credentials they need; no
// other type in this package sees them.
type Broker interface {
	// Account reads the venue's current record. Called before every decision.
	Account(ctx context.Context) (Account, error)
	// Place submits an order. It must be idempotent on Order.ClientID:
	// submitting the same ClientID twice must not fill twice.
	Place(ctx context.Context, o Order) (Fill, error)
	// Rules reports the venue's constraints for a symbol.
	Rules(ctx context.Context, symbol string) (SymbolRules, error)
	// Name identifies the broker in logs, without revealing anything about
	// the account.
	Name() string
}

// SymbolRules are the venue's constraints on an order.
type SymbolRules struct {
	// LotStep is the quantity increment. An order not on the step is rejected
	// by most venues, and a bot that does not round is a bot whose orders
	// silently fail.
	LotStep float64
	// MinQuantity and MinNotionalUSD are the floors below which an order is
	// refused. Worth knowing in advance: an engine that keeps emitting orders
	// under the floor looks like it is trading and is not.
	MinQuantity    float64
	MinNotionalUSD float64
}

// Rails are the limits that hold whatever the signal says.
type Rails struct {
	// MaxPositionFraction caps |position| as a fraction of equity.
	MaxPositionFraction float64
	// MaxOrderFraction caps any single order, so a signal error or a bad
	// reconciliation cannot produce one enormous trade.
	MaxOrderFraction float64
	// MaxDailyLossFraction halts trading for the day once the account is down
	// this much from its start-of-day equity.
	MaxDailyLossFraction float64
	// MaxDrawdownFraction halts and flattens once the account is this far
	// below its observed peak.
	MaxDrawdownFraction float64
	// DryRun logs the orders it would place and places none. It is the
	// default, and it should be where any new configuration starts.
	DryRun bool
}

// DefaultRails are conservative and dry by default.
func DefaultRails() Rails {
	return Rails{
		MaxPositionFraction:  0.25,
		MaxOrderFraction:     0.10,
		MaxDailyLossFraction: 0.05,
		MaxDrawdownFraction:  0.15,
		DryRun:               true,
	}
}

// Action records what the engine did with a signal, for the log and for the
// operator.
type Action struct {
	Time    time.Time
	Symbol  string
	Target  float64
	Current float64
	Order   *Order
	Fill    *Fill
	Skipped string
	Halted  bool
	Reason  string
}

// ErrHalted is returned once the engine has stopped and will not trade again
// without an explicit Resume.
var ErrHalted = errors.New("execution halted")

// Engine turns signals into orders, under the rails.
type Engine struct {
	Broker Broker
	Rails  Rails

	// PositionTolerance is how far the venue's position may differ from the
	// engine's belief before it halts, expressed as a fraction of equity and
	// converted to base units at the current price. Small divergence from
	// fees and rounding is normal; a real mismatch is not.
	PositionTolerance float64

	Now func() time.Time

	mu sync.Mutex
	// believedQty is the last position the engine knows it established, in
	// BASE UNITS.
	//
	// Base units rather than a fraction of equity, and the distinction is not
	// pedantic — an earlier version stored the fraction and halted constantly
	// for no reason. A fraction of equity is not a stable identity for a
	// position: 0.02 BTC is 10% of a $10,000 account at $50,000 and 4% of it
	// after the price halves, with nothing having changed about the position.
	// Reconciliation has to compare the thing the engine actually controls.
	believedQty  float64
	lastEquity   float64
	peakEquity   float64
	dayStart     float64
	dayStartedAt time.Time
	halted       bool
	haltReason   string
}

// NewEngine returns an engine on conservative, dry-run rails.
func NewEngine(b Broker) *Engine {
	return &Engine{
		Broker:            b,
		Rails:             DefaultRails(),
		PositionTolerance: 0.02,
		Now:               time.Now,
	}
}

// Halted reports whether the engine has stopped, and why.
func (e *Engine) Halted() (bool, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.halted, e.haltReason
}

// Resume clears a halt. Deliberately manual: every halt means the engine found
// a state it could not explain, and clearing that automatically would defeat
// the point of stopping.
func (e *Engine) Resume() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.halted, e.haltReason = false, ""
}

// Drawdown reports the account's current decline from its observed peak.
//
// This is what wires the risk manager's kill switch to reality. In a backtest
// the switch reads a simulated equity curve; live, this is the curve, and
// without it the switch is inert.
func (e *Engine) Drawdown() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.peakEquity <= 0 {
		return 0
	}
	if e.lastEquity <= 0 {
		return 0
	}
	return math.Max(0, (e.peakEquity-e.lastEquity)/e.peakEquity)
}

// OnSignal reconciles, applies the rails, and places at most one order.
func (e *Engine) OnSignal(ctx context.Context, s LiveSignal) (Action, error) {
	act := Action{Time: e.Now(), Symbol: s.Symbol, Target: s.Target}

	if halted, reason := e.Halted(); halted {
		act.Halted, act.Reason = true, reason
		return act, ErrHalted
	}

	acct, err := e.Broker.Account(ctx)
	if err != nil {
		// A failure to read the account is not a reason to trade from memory.
		return act, fmt.Errorf("reading account: %w", err)
	}
	if acct.EquityUSD <= 0 {
		e.halt("account reports no equity")
		act.Halted, act.Reason = true, "account reports no equity"
		return act, ErrHalted
	}

	e.mu.Lock()
	e.lastEquity = acct.EquityUSD
	if acct.EquityUSD > e.peakEquity {
		e.peakEquity = acct.EquityUSD
	}
	day := e.Now().UTC().Truncate(24 * time.Hour)
	if !day.Equal(e.dayStartedAt) {
		e.dayStartedAt, e.dayStart = day, acct.EquityUSD
	}
	believedQty, peak, dayStart := e.believedQty, e.peakEquity, e.dayStart
	e.mu.Unlock()

	// ── Reconciliation ──
	held := acct.Position(s.Symbol)
	heldFraction := held * s.Close / acct.EquityUSD
	act.Current = heldFraction

	tolQty := e.PositionTolerance * acct.EquityUSD / s.Close
	if drift := math.Abs(held - believedQty); drift > tolQty {
		reason := fmt.Sprintf(
			"position mismatch: engine believed %.8f units, venue holds %.8f (drift %.8f, "+
				"tolerance %.8f). Sizing the next order from a position that is not there is "+
				"how an account is lost; resolve it and resume deliberately",
			believedQty, held, drift, tolQty)
		e.halt(reason)
		act.Halted, act.Reason = true, reason
		return act, ErrHalted
	}

	// ── Rails ──
	if dd := (peak - acct.EquityUSD) / peak; peak > 0 && dd >= e.Rails.MaxDrawdownFraction {
		reason := fmt.Sprintf("drawdown %s at or past the %s limit — flattening and halting",
			pct(dd), pct(e.Rails.MaxDrawdownFraction))
		act.Reason = reason
		if heldFraction != 0 {
			return e.flatten(ctx, act, acct, s, reason)
		}
		e.halt(reason)
		act.Halted = true
		return act, ErrHalted
	}
	if dayStart > 0 {
		if loss := (dayStart - acct.EquityUSD) / dayStart; loss >= e.Rails.MaxDailyLossFraction {
			reason := fmt.Sprintf("down %s today, past the %s daily limit",
				pct(loss), pct(e.Rails.MaxDailyLossFraction))
			act.Reason = reason
			if heldFraction != 0 {
				return e.flatten(ctx, act, acct, s, reason)
			}
			e.halt(reason)
			act.Halted = true
			return act, ErrHalted
		}
	}

	target := clamp(s.Target, -e.Rails.MaxPositionFraction, e.Rails.MaxPositionFraction)
	delta := target - heldFraction
	if math.Abs(delta) > e.Rails.MaxOrderFraction {
		// Approach the target in steps rather than refusing outright: a
		// capped order still moves in the right direction, and the cap is
		// there to bound a single mistake, not to freeze the position.
		delta = math.Copysign(e.Rails.MaxOrderFraction, delta)
		act.Reason = fmt.Sprintf("order capped at %s of equity", pct(e.Rails.MaxOrderFraction))
	}

	rules, err := e.Broker.Rules(ctx, s.Symbol)
	if err != nil {
		return act, fmt.Errorf("reading symbol rules: %w", err)
	}

	qty := delta * acct.EquityUSD / s.Close
	qty = roundToStep(qty, rules.LotStep)
	notional := math.Abs(qty) * s.Close

	if qty == 0 || math.Abs(qty) < rules.MinQuantity || notional < rules.MinNotionalUSD {
		act.Skipped = fmt.Sprintf("change of %s (%.8f units, $%.2f) is below the venue's minimum",
			pct(delta), qty, notional)
		return act, nil
	}

	order := Order{
		ClientID:   clientID(s.Symbol, s.BarTime, target),
		Symbol:     s.Symbol,
		Side:       sideFor(qty),
		Quantity:   math.Abs(qty),
		ReduceOnly: math.Abs(target) < math.Abs(heldFraction) && sameSign(target, heldFraction),
	}
	act.Order = &order

	if e.Rails.DryRun {
		act.Skipped = "dry run: no order was sent"
		return act, nil
	}

	fill, err := e.Broker.Place(ctx, order)
	if err != nil {
		// The order may or may not have been accepted. Do NOT retry — resend
		// is how a position doubles. Halt so the next cycle's reconciliation
		// happens under human eyes.
		reason := fmt.Sprintf("order outcome unknown (%v) — halting rather than retrying, "+
			"because a timeout does not mean the order was rejected", err)
		e.halt(reason)
		act.Halted, act.Reason = true, reason
		return act, ErrHalted
	}

	act.Fill = &fill
	e.mu.Lock()
	e.believedQty = held + signedQty(fill.Quantity, order.Side)
	e.mu.Unlock()
	return act, nil
}

func (e *Engine) flatten(ctx context.Context, act Action, acct Account, s LiveSignal, reason string) (Action, error) {
	held := acct.Position(s.Symbol)
	if e.Rails.DryRun {
		act.Skipped = "dry run: would have flattened"
		e.halt(reason)
		act.Halted = true
		return act, ErrHalted
	}
	rules, err := e.Broker.Rules(ctx, s.Symbol)
	if err != nil {
		return act, err
	}
	qty := roundToStep(-held, rules.LotStep)
	order := Order{
		ClientID: clientID(s.Symbol, s.BarTime, 0),
		Symbol:   s.Symbol,
		Side:     sideFor(qty),
		Quantity: math.Abs(qty),
		// Reduce-only on the way out: a race must not flip the position
		// through zero into a new one on the other side.
		ReduceOnly: true,
	}
	act.Order = &order

	fill, err := e.Broker.Place(ctx, order)
	if err != nil {
		e.halt(reason + " (and the flattening order's outcome is unknown)")
		act.Halted = true
		return act, ErrHalted
	}
	act.Fill = &fill
	e.mu.Lock()
	e.believedQty = 0
	e.mu.Unlock()
	e.halt(reason)
	act.Halted = true
	return act, ErrHalted
}

func (e *Engine) halt(reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.halted {
		e.halted, e.haltReason = true, reason
	}
}

// clientID derives a deterministic idempotency key. The same instrument, bar
// and target always produce the same ID, so a retry is recognisably the same
// order rather than a second one.
func clientID(symbol string, bar time.Time, target float64) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%.6f", symbol, bar.UTC().Unix(), target)))
	return "ms-" + hex.EncodeToString(h[:])[:24]
}

func sideFor(qty float64) Side {
	if qty < 0 {
		return Sell
	}
	return Buy
}

func signedQty(qty float64, side Side) float64 {
	if side == Sell {
		return -qty
	}
	return qty
}

func sameSign(a, b float64) bool { return a*b > 0 }

// roundToStep truncates toward zero onto the venue's lot increment. Toward
// zero rather than to nearest, so rounding can never make an order larger than
// intended.
func roundToStep(qty, step float64) float64 {
	if step <= 0 {
		return qty
	}
	return math.Trunc(qty/step) * step
}
