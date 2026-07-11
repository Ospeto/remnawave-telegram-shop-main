package reporting

import "math"

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
