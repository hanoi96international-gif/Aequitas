package marketsignals

import (
	"context"
	"fmt"
	"sync"
)

// PaperBroker is a venue that fills orders against a price you give it and
// settles nothing.
//
// It is the default, and not as a placeholder. Running the full execution path
// — reconciliation, idempotency, rails, the drawdown stop — against a paper
// account is the only way to find out whether the plumbing works before it can
// cost anything. Almost every bug in a trading bot is in that plumbing rather
// than in the signal, and almost all of them are visible on paper.
//
// It also closes a gap the live runner has to warn about otherwise: the risk
// manager's kill switch needs a real equity curve, and a paper account has
// one. Wire Engine.Drawdown into LiveRunner.Drawdown and the stop works from
// the first bar, on paper exactly as it would with money.
//
// The fills are optimistic in one specific way, and it is worth naming: they
// happen at the price passed in, in full, immediately. Real fills are partial,
// late, and worse. What the backtester's cost model says about that is the
// honest estimate; this broker is for testing the machinery, not for
// estimating returns.
type PaperBroker struct {
	mu        sync.Mutex
	equity    float64
	positions map[string]Position
	// filled records the client IDs already executed, so a resubmitted order
	// is rejected exactly as a real venue would reject it. Without this, the
	// idempotency guarantee could not be tested at all.
	filled map[string]Fill
	rules  SymbolRules
	marks  map[string]float64
	// FeeRate is charged on every fill.
	FeeRate float64
}

// NewPaperBroker returns a paper account with the given starting equity.
func NewPaperBroker(equityUSD float64) *PaperBroker {
	return &PaperBroker{
		equity:    equityUSD,
		positions: map[string]Position{},
		filled:    map[string]Fill{},
		marks:     map[string]float64{},
		rules:     SymbolRules{LotStep: 1e-6, MinQuantity: 1e-6, MinNotionalUSD: 5},
		FeeRate:   0.0005,
	}
}

// Name identifies the broker.
func (p *PaperBroker) Name() string { return "paper" }

// SetRules overrides the venue constraints, for matching a real venue's
// minimums while still trading nothing.
func (p *PaperBroker) SetRules(r SymbolRules) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rules = r
}

// Rules reports the venue constraints.
func (p *PaperBroker) Rules(context.Context, string) (SymbolRules, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rules, nil
}

// Account returns the paper account.
func (p *PaperBroker) Account(context.Context) (Account, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := Account{EquityUSD: p.equity, Positions: map[string]Position{}}
	for k, v := range p.positions {
		out.Positions[k] = v
	}
	return out, nil
}

// Place fills an order at the broker's last marked price.
//
// Resubmitting a client ID returns the ORIGINAL fill rather than filling
// again. That is what a venue with idempotency keys does, and reproducing it
// here is the only way the engine's retry behaviour can be exercised.
func (p *PaperBroker) Place(_ context.Context, o Order) (Fill, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if prior, ok := p.filled[o.ClientID]; ok {
		return prior, nil
	}
	price := p.marks[o.Symbol]
	if price <= 0 {
		return Fill{}, fmt.Errorf("paper broker has no price for %s; call Mark first", o.Symbol)
	}

	qty := o.Quantity
	if o.Side == Sell {
		qty = -qty
	}
	pos := p.positions[o.Symbol]

	if o.ReduceOnly {
		// Clamp so the fill cannot carry the position through zero — the same
		// guarantee a venue's reduce-only flag gives.
		if pos.Quantity > 0 && qty < -pos.Quantity {
			qty = -pos.Quantity
		}
		if pos.Quantity < 0 && qty > -pos.Quantity {
			qty = -pos.Quantity
		}
		if pos.Quantity == 0 {
			qty = 0
		}
	}

	fee := absf(qty) * price * p.FeeRate
	p.equity -= fee

	pos.Symbol = o.Symbol
	pos.Quantity += qty
	pos.Entry = price
	if pos.Quantity == 0 {
		delete(p.positions, o.Symbol)
	} else {
		p.positions[o.Symbol] = pos
	}

	fill := Fill{ClientID: o.ClientID, Quantity: absf(qty), Price: price, Fee: fee}
	p.filled[o.ClientID] = fill
	return fill, nil
}

// Mark sets the current price for a symbol and revalues the account.
//
// The live runner calls this with each closed bar, which is what gives the
// paper account a real equity curve rather than a static balance — and
// therefore what gives the drawdown stop something to read.
func (p *PaperBroker) Mark(symbol string, price float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.marks == nil {
		p.marks = map[string]float64{}
	}
	if pos, ok := p.positions[symbol]; ok && p.marks[symbol] > 0 {
		p.equity += pos.Quantity * (price - p.marks[symbol])
	}
	p.marks[symbol] = price
}

// Equity is the paper account's current value.
func (p *PaperBroker) Equity() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.equity
}
