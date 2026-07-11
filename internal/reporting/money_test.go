package reporting

import "testing"

func TestRoundMoney_TwoDecimals(t *testing.T) {
	if RoundMoney(1.005) != 1.01 {
		t.Fatalf("1.005 -> %v", RoundMoney(1.005))
	}
	if RoundMoney(2.004) != 2.00 {
		t.Fatalf("2.004 -> %v", RoundMoney(2.004))
	}
	if RoundMoney(-1.005) != -1.01 {
		t.Fatalf("-1.005 -> %v", RoundMoney(-1.005))
	}
}
