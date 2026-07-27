package marketsignals

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func liveSig(bar time.Time, target, close float64) LiveSignal {
	return LiveSignal{
		Time: bar, BarTime: bar, Symbol: "BTCUSDT",
		Target: target, Direction: sideOf(target), Close: close,
	}
}

func armedEngine(equity float64) (*Engine, *PaperBroker) {
	b := NewPaperBroker(equity)
	b.Mark("BTCUSDT", 50_000)
	e := NewEngine(b)
	e.Rails.DryRun = false
	e.Now = func() time.Time { return time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC) }
	return e, b
}

func TestEngine_DefaultsToDryRun(t *testing.T) {
	b := NewPaperBroker(10_000)
	b.Mark("BTCUSDT", 50_000)
	e := NewEngine(b)

	act, err := e.OnSignal(context.Background(), liveSig(time.Now(), 0.2, 50_000))
	if err != nil {
		t.Fatalf("OnSignal: %v", err)
	}
	if act.Order == nil {
		t.Fatal("no order was computed at all")
	}
	if !strings.Contains(act.Skipped, "dry run") {
		t.Fatalf("skipped %q; a fresh engine must not trade", act.Skipped)
	}
	if b.Equity() != 10_000 {
		t.Fatalf("equity moved to %v in dry run", b.Equity())
	}
}

func TestEngine_PlacesAnOrderTowardTheTarget(t *testing.T) {
	e, _ := armedEngine(10_000)

	act, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.10, 50_000))
	if err != nil {
		t.Fatalf("OnSignal: %v", err)
	}
	if act.Fill == nil {
		t.Fatalf("no fill; skipped=%q reason=%q", act.Skipped, act.Reason)
	}
	// 10% of $10,000 is $1,000, which at $50,000 is 0.02 BTC.
	if math.Abs(act.Fill.Quantity-0.02) > 1e-9 {
		t.Fatalf("filled %v, want 0.02 BTC", act.Fill.Quantity)
	}
	if act.Order.Side != Buy {
		t.Fatalf("side %q for a long target", act.Order.Side)
	}
}

// TestEngine_HaltsOnAPositionMismatch is the defence that matters most. A bot
// sizing its next order from a position it does not have is how an account is
// lost, and the mismatch has many ordinary causes: a rejected order, a partial
// fill, a liquidation, somebody trading the account by hand.
func TestEngine_HaltsOnAPositionMismatch(t *testing.T) {
	e, b := armedEngine(10_000)

	if _, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.10, 50_000)); err != nil {
		t.Fatalf("first signal: %v", err)
	}

	// Something moved the position behind the engine's back.
	b.mu.Lock()
	b.positions["BTCUSDT"] = Position{Symbol: "BTCUSDT", Quantity: 0.10, Entry: 50_000}
	b.mu.Unlock()

	act, err := e.OnSignal(context.Background(), liveSig(e.Now().Add(time.Hour), 0.10, 50_000))
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("err = %v, want ErrHalted", err)
	}
	if !act.Halted || !strings.Contains(act.Reason, "position mismatch") {
		t.Fatalf("reason %q should name the mismatch", act.Reason)
	}

	// And it stays halted rather than quietly resuming next bar.
	if _, err := e.OnSignal(context.Background(), liveSig(e.Now().Add(2*time.Hour), 0, 50_000)); !errors.Is(err, ErrHalted) {
		t.Fatalf("engine resumed on its own: %v", err)
	}
	e.Resume()
	if halted, _ := e.Halted(); halted {
		t.Fatal("Resume did not clear the halt")
	}
}

// TestEngine_DoesNotResendAfterAnAmbiguousFailure. A timeout does not mean the
// order was rejected — it means the outcome is unknown, and resending is how a
// position doubles.
func TestEngine_DoesNotResendAfterAnAmbiguousFailure(t *testing.T) {
	b := &flakyBroker{PaperBroker: NewPaperBroker(10_000), failNext: true}
	b.Mark("BTCUSDT", 50_000)
	e := NewEngine(b)
	e.Rails.DryRun = false
	e.Now = func() time.Time { return time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC) }

	act, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.10, 50_000))
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("err = %v, want ErrHalted after an unknown outcome", err)
	}
	if !strings.Contains(act.Reason, "does not mean the order was rejected") {
		t.Fatalf("reason %q should explain why it did not retry", act.Reason)
	}
	if b.places != 1 {
		t.Fatalf("the engine sent %d orders after one ambiguous failure; want 1", b.places)
	}
}

// TestEngine_IdempotentClientIDs: the same instrument, bar and target must
// produce the same key, so a venue can recognise a resend as a duplicate.
func TestEngine_IdempotentClientIDs(t *testing.T) {
	bar := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	a := clientID("BTCUSDT", bar, 0.25)
	if b := clientID("BTCUSDT", bar, 0.25); a != b {
		t.Fatalf("the same order produced two IDs: %q and %q", a, b)
	}
	for _, other := range []string{
		clientID("ETHUSDT", bar, 0.25),
		clientID("BTCUSDT", bar.Add(time.Hour), 0.25),
		clientID("BTCUSDT", bar, 0.26),
	} {
		if other == a {
			t.Fatalf("a different order reused the ID %q", a)
		}
	}

	// And the paper broker honours it, as a venue with idempotency keys does.
	b := NewPaperBroker(10_000)
	b.Mark("BTCUSDT", 50_000)
	o := Order{ClientID: a, Symbol: "BTCUSDT", Side: Buy, Quantity: 0.1}
	f1, _ := b.Place(context.Background(), o)
	f2, _ := b.Place(context.Background(), o)
	if f1 != f2 {
		t.Fatalf("resubmitting one client ID produced two different fills")
	}
	acct, _ := b.Account(context.Background())
	if math.Abs(acct.Position("BTCUSDT")-0.1) > 1e-12 {
		t.Fatalf("position %v after a duplicate submission; want 0.1", acct.Position("BTCUSDT"))
	}
}

func TestEngine_RailsCapPositionAndOrderSize(t *testing.T) {
	e, _ := armedEngine(10_000)
	e.Rails.MaxPositionFraction = 0.20
	e.Rails.MaxOrderFraction = 0.05

	// Ask for far more than either rail allows.
	act, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.90, 50_000))
	if err != nil {
		t.Fatalf("OnSignal: %v", err)
	}
	if act.Fill == nil {
		t.Fatalf("no fill; skipped=%q", act.Skipped)
	}
	// One order may move at most 5% of $10,000 = $500 = 0.01 BTC.
	if math.Abs(act.Fill.Quantity-0.01) > 1e-9 {
		t.Fatalf("filled %v, want the 0.01 BTC the order cap allows", act.Fill.Quantity)
	}
	if !strings.Contains(act.Reason, "capped") {
		t.Fatalf("reason %q should record the cap", act.Reason)
	}
}

func TestEngine_SkipsOrdersBelowTheVenueMinimum(t *testing.T) {
	e, b := armedEngine(10_000)
	b.SetRules(SymbolRules{LotStep: 0.001, MinQuantity: 0.001, MinNotionalUSD: 100})

	// 0.1% of $10,000 is $10 — under the $100 minimum notional.
	act, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.001, 50_000))
	if err != nil {
		t.Fatalf("OnSignal: %v", err)
	}
	if act.Fill != nil {
		t.Fatal("an order under the venue minimum was sent")
	}
	if !strings.Contains(act.Skipped, "below the venue's minimum") {
		t.Fatalf("skipped %q", act.Skipped)
	}
}

func TestEngine_RoundsTowardZeroOntoTheLotStep(t *testing.T) {
	// Rounding to nearest could make an order LARGER than intended, which is
	// the wrong direction for a rounding error to go.
	if got := roundToStep(0.0199, 0.01); math.Abs(got-0.01) > 1e-12 {
		t.Fatalf("roundToStep(0.0199, 0.01) = %v, want 0.01", got)
	}
	if got := roundToStep(-0.0199, 0.01); math.Abs(got-(-0.01)) > 1e-12 {
		t.Fatalf("roundToStep(-0.0199, 0.01) = %v, want -0.01", got)
	}
}

// TestEngine_FlattensAndHaltsOnDrawdown. The stop has to actually close the
// position, not merely stop opening new ones.
func TestEngine_FlattensAndHaltsOnDrawdown(t *testing.T) {
	e, b := armedEngine(10_000)
	e.Rails.MaxDrawdownFraction = 0.10
	e.Rails.MaxDailyLossFraction = 0.99 // isolate the drawdown rail
	e.Rails.MaxOrderFraction = 0.25     // let the whole position go on at once

	// A 20% position is needed to breach a 10% account drawdown at all: a 10%
	// position losing everything is only a 10% account loss, so the earlier
	// version of this test could never have fired the rail it was testing.
	if _, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.20, 50_000)); err != nil {
		t.Fatalf("opening: %v", err)
	}
	acct, _ := b.Account(context.Background())
	if acct.Position("BTCUSDT") == 0 {
		t.Fatal("no position was opened")
	}

	// Price falls 60%: a 20% position loses 12% of the account.
	b.Mark("BTCUSDT", 20_000)

	act, err := e.OnSignal(context.Background(), liveSig(e.Now().Add(time.Hour), 0.20, 20_000))
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("err = %v, want ErrHalted", err)
	}
	if !strings.Contains(act.Reason, "drawdown") {
		t.Fatalf("reason %q should name the drawdown", act.Reason)
	}
	acct, _ = b.Account(context.Background())
	if acct.Position("BTCUSDT") != 0 {
		t.Fatalf("still holding %v after the drawdown stop; the stop must flatten, not just "+
			"stop opening", acct.Position("BTCUSDT"))
	}
}

func TestEngine_HaltsOnDailyLoss(t *testing.T) {
	e, b := armedEngine(10_000)
	e.Rails.MaxDailyLossFraction = 0.03
	e.Rails.MaxDrawdownFraction = 0.99 // isolate the daily rail

	if _, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.10, 50_000)); err != nil {
		t.Fatalf("opening: %v", err)
	}
	b.Mark("BTCUSDT", 30_000) // a loss well past 3% of the account

	_, err := e.OnSignal(context.Background(), liveSig(e.Now().Add(time.Hour), 0.10, 30_000))
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("err = %v, want ErrHalted on the daily loss limit", err)
	}
	if _, reason := e.Halted(); !strings.Contains(reason, "today") {
		t.Fatalf("halt reason %q should name the daily limit", reason)
	}
}

// TestEngine_DrawdownFeedsTheKillSwitch closes the loop the live runner has to
// warn about otherwise: with a broker attached, the risk manager's stop reads a
// real equity curve.
func TestEngine_DrawdownFeedsTheKillSwitch(t *testing.T) {
	e, b := armedEngine(10_000)

	if _, err := e.OnSignal(context.Background(), liveSig(e.Now(), 0.20, 50_000)); err != nil {
		t.Fatalf("opening: %v", err)
	}
	if dd := e.Drawdown(); dd > 0.01 {
		t.Fatalf("drawdown %v on a fresh account", dd)
	}

	b.Mark("BTCUSDT", 45_000)
	_, _ = e.OnSignal(context.Background(), liveSig(e.Now().Add(time.Hour), 0.20, 45_000))

	if dd := e.Drawdown(); dd <= 0 {
		t.Fatalf("drawdown %v after a 10%% price fall on a 20%% position; the kill switch "+
			"would still be reading zero", dd)
	}
}

func TestEngine_RefusesToTradeWithoutEquity(t *testing.T) {
	b := NewPaperBroker(0)
	b.Mark("BTCUSDT", 50_000)
	e := NewEngine(b)
	e.Rails.DryRun = false

	_, err := e.OnSignal(context.Background(), liveSig(time.Now(), 0.1, 50_000))
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("err = %v, want ErrHalted on a zero-equity account", err)
	}
}

func TestPaperBroker_ReduceOnlyCannotFlipThePosition(t *testing.T) {
	b := NewPaperBroker(10_000)
	b.Mark("BTCUSDT", 50_000)

	if _, err := b.Place(context.Background(), Order{
		ClientID: "open", Symbol: "BTCUSDT", Side: Buy, Quantity: 0.05,
	}); err != nil {
		t.Fatalf("open: %v", err)
	}
	// Try to sell far more than is held, reduce-only.
	if _, err := b.Place(context.Background(), Order{
		ClientID: "close", Symbol: "BTCUSDT", Side: Sell, Quantity: 5, ReduceOnly: true,
	}); err != nil {
		t.Fatalf("close: %v", err)
	}
	acct, _ := b.Account(context.Background())
	if got := acct.Position("BTCUSDT"); got != 0 {
		t.Fatalf("position %v; a reduce-only order carried it through zero into a short", got)
	}
}

func TestPaperBroker_MarkRevaluesTheAccount(t *testing.T) {
	b := NewPaperBroker(10_000)
	b.Mark("BTCUSDT", 50_000)
	if _, err := b.Place(context.Background(), Order{
		ClientID: "x", Symbol: "BTCUSDT", Side: Buy, Quantity: 0.1,
	}); err != nil {
		t.Fatalf("place: %v", err)
	}
	before := b.Equity()

	b.Mark("BTCUSDT", 55_000) // 0.1 BTC × $5,000
	if got := b.Equity() - before; math.Abs(got-500) > 1e-6 {
		t.Fatalf("equity moved %v on a $5,000 rise in a 0.1 BTC position, want 500", got)
	}
}

// flakyBroker fails the first Place, to exercise the ambiguous-outcome path.
type flakyBroker struct {
	*PaperBroker
	failNext bool
	places   int
}

func (f *flakyBroker) Place(ctx context.Context, o Order) (Fill, error) {
	f.places++
	if f.failNext {
		f.failNext = false
		return Fill{}, errors.New("timeout waiting for the venue")
	}
	return f.PaperBroker.Place(ctx, o)
}
