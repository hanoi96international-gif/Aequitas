package marketsignals

import "strconv"

// Small formatting helpers, used to keep signal notes readable without
// dragging fmt.Sprintf through every branch of every agent.

func f2(x float64) string { return strconv.FormatFloat(x, 'f', 2, 64) }

func pct(frac float64) string { return strconv.FormatFloat(frac*100, 'f', 0, 64) + "%" }

// bps renders a funding rate as basis points, the unit funding is actually
// quoted in.
func bps(rate float64) string { return strconv.FormatFloat(rate*10_000, 'f', 2, 64) + "bp" }

func itoa(i int) string { return strconv.Itoa(i) }
