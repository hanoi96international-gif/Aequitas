package keeper

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
)

// Decimal stores exact monetary amounts as int64 micro-units.
// 1 AEQ = 1_000_000 micro-AEQ. This eliminates float64 rounding errors
// that accumulate across many ledger operations.
//
// JSON serializes as a decimal number (e.g. 1000.000000) for API
// backward-compatibility — existing clients see no change.
type Decimal int64

const DecimalPrecision = int64(1_000_000)

// NewDecimal converts a float64 AEQ amount to Decimal, rounding to 6dp.
func NewDecimal(aeq float64) Decimal {
	if math.IsNaN(aeq) || math.IsInf(aeq, 0) {
		return 0
	}
	// P2-FIX: guard against int64 overflow for large finite floats.
	// math.Round on a value > ~9.2e12 AEQ (9.2e18 micro-units) would
	// overflow int64, producing a large negative or garbage value.
	result := math.Round(aeq * float64(DecimalPrecision))
	// FIX (audit 2026-06-29): float64(math.MaxInt64) itself rounds UP to
	// 2^63 (9223372036854775808.0) when converted to float64, since
	// math.MaxInt64 (2^63-1) isn't exactly representable in float64. A
	// `result` of exactly 2^63 therefore failed this ">" check (2^63 is
	// not > 2^63) and fell through to Decimal(result) — converting an
	// out-of-int64-range float64 to int64 in Go is implementation-defined
	// for that exact boundary and reliably yields math.MinInt64 on amd64
	// (the CVTTSD2SI "invalid" sentinel), silently turning a huge positive
	// input into a huge negative balance. Reachable from attacker-supplied
	// JSON via UnmarshalJSON below, not just internal callers. ">=" closes
	// the exact boundary value the float64 rounding created; the lower
	// bound has no equivalent gap since math.MinInt64 (-2^63) IS exactly
	// representable in float64.
	if result >= float64(math.MaxInt64) || result < float64(math.MinInt64) {
		return 0
	}
	return Decimal(result)
}

// NewDecimalFromMicro creates a Decimal directly from micro-units.
func NewDecimalFromMicro(micro int64) Decimal { return Decimal(micro) }

// Float returns the AEQ value as float64 (for display/legacy code).
func (d Decimal) Float() float64 { return float64(d) / float64(DecimalPrecision) }

// Micro returns the underlying int64 micro-unit value.
func (d Decimal) Micro() int64 { return int64(d) }

// Add returns d + other. Exact.
func (d Decimal) Add(other Decimal) Decimal { return d + other }

// Sub returns d - other. Exact.
func (d Decimal) Sub(other Decimal) Decimal { return d - other }

// MulFloat multiplies by a float64 (e.g. for rate/fee calculations), rounding.
func (d Decimal) MulFloat(f float64) Decimal {
	// P3-10: guard against int64 overflow when f > 1 and d is large
	result := math.Round(float64(d) * f)
	// FIX (audit 2026-06-29): same off-by-one as NewDecimal's overflow
	// guard — float64(math.MaxInt64) rounds up to exactly 2^63, so a
	// `result` of precisely 2^63 passed this "> maxD" check uncaught and
	// fell through to Decimal(result), an out-of-range float64->int64
	// conversion that reliably yields math.MinInt64 on amd64 instead of
	// the clamped math.MaxInt64 this guard exists to produce.
	const maxD = float64(math.MaxInt64)
	if result >= maxD {
		return Decimal(math.MaxInt64)
	}
	if result < -maxD {
		return Decimal(math.MinInt64)
	}
	return Decimal(result)
}

// FIX (P3-3, beta-launch audit 2026-06-27): DivDecimal used to live here —
// removed rather than kept around fixed-but-unused. It had zero callers
// anywhere in this codebase and zero test coverage; a correct-looking but
// never-exercised method is exactly the kind of thing a future caller
// reaches for without anyone having actually verified it against this
// type's real usage patterns. If AEQ division is ever genuinely needed,
// write it fresh against the actual call site's requirements (and add a
// test) rather than resurrecting this.

// IsZero returns true when d == 0.
func (d Decimal) IsZero() bool { return d == 0 }

// IsPositive returns true when d > 0.
func (d Decimal) IsPositive() bool { return d > 0 }

// IsNegative returns true when d < 0.
func (d Decimal) IsNegative() bool { return d < 0 }

// AtLeastZero floors d at 0 — the exact "max(0.0, ...)" clamp several
// reserve/share-burning call sites need (performance audit 2026-07-06:
// consolidates the repeated float64 max(0.0, round6(...)) pattern into one
// exact int64 comparison, no rounding involved since Decimal is already
// exact).
func (d Decimal) AtLeastZero() Decimal {
	if d < 0 {
		return 0
	}
	return d
}

// Neg returns -d.
func (d Decimal) Neg() Decimal { return -d }

// String formats as "X.YYYYYY AEQ" for debugging.
func (d Decimal) String() string { return fmt.Sprintf("%.6f", d.Float()) }

// MarshalJSON serializes as a plain float number for API compatibility.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Float())
}

// UnmarshalJSON accepts a float number and converts to micro-units.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return fmt.Errorf("invalid decimal value: overflow or NaN")
	}
	*d = NewDecimal(f)
	return nil
}

// AMMSwapOut computes the constant-product AMM output amount:
// amountOut = (reserveOut * amountInAfterFee) / (reserveIn + amountInAfterFee)
// Uses big.Int arithmetic to avoid int64 overflow for large reserves.
func AMMSwapOut(reserveIn, reserveOut, amountInAfterFee Decimal) Decimal {
	ri := new(big.Int).SetInt64(int64(reserveIn))
	ro := new(big.Int).SetInt64(int64(reserveOut))
	ai := new(big.Int).SetInt64(int64(amountInAfterFee))
	// numerator = reserveOut * amountIn
	num := new(big.Int).Mul(ro, ai)
	// denominator = reserveIn + amountIn
	den := new(big.Int).Add(ri, ai)
	if den.Sign() == 0 {
		return 0
	}
	result := new(big.Int).Div(num, den)
	// P0-4: guard against overflow — result must fit in int64.
	// If it doesn't, the swap is too large for the current pool size.
	if result.BitLen() > 63 || result.Sign() < 0 {
		return 0 // caller will detect zero output and reject the swap
	}
	return Decimal(result.Int64())
}
