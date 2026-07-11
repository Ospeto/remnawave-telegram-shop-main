package reporting

import "math"

// RoundMoney rounds v to two decimal places using half-away-from-zero
// (commercial rounding): 1.005 → 1.01, -1.005 → -1.01, 2.004 → 2.00.
//
// Non-finite inputs (NaN, ±Inf) return 0 so downstream money fields never
// serialize as NaN/Inf in JSON/CSV. Callers that need to reject non-finite
// amounts should validate at the API/repository boundary before RoundMoney.
//
// A tiny epsilon counters binary float representation error on values such as
// 1.005 (where 1.005*100 is slightly under 100.5).
func RoundMoney(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	// Half-away-from-zero to 2 decimals. Tiny epsilon counters binary float
	// error on values like 1.005 (1.005*100 == 100.4999...).
	const eps = 1e-9
	scaled := v * 100
	if scaled >= 0 {
		scaled = math.Floor(scaled + 0.5 + eps)
	} else {
		scaled = math.Ceil(scaled - 0.5 - eps)
	}
	return scaled / 100
}
