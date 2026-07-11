package database

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeAdjustmentAmount_TwoDecimals(t *testing.T) {
	got, err := normalizeAdjustmentAmount(10.005)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 10.01 {
		t.Fatalf("got %v want 10.01", got)
	}
}

func TestNormalizeAdjustmentAmount_RejectsNonPositive(t *testing.T) {
	if _, err := normalizeAdjustmentAmount(0); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := normalizeAdjustmentAmount(-1); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestBuildCreateFinancialAdjustmentSQL_UsesIdempotencyConflict(t *testing.T) {
	sql := buildCreateFinancialAdjustmentSQL()
	if !strings.Contains(sql, "INSERT INTO financial_adjustment") {
		t.Fatalf("missing insert: %s", sql)
	}
	if !strings.Contains(sql, "ON CONFLICT (idempotency_key) DO NOTHING") {
		t.Fatalf("missing idempotency conflict clause: %s", sql)
	}
}

func TestBuildSumRefundsByPeriodSQL_YangonAndAdminExclusion(t *testing.T) {
	sql, err := buildSumRefundsByPeriodSQL(RevenuePeriodDay)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Asia/Yangon",
		"adjustment_type = 'refund'",
		"telegram_id",
		"effective_at >= $1",
		"effective_at < $2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in %s", want, sql)
		}
	}
}

func TestCreateFinancialAdjustmentInput_RequiresIdempotencyKey(t *testing.T) {
	err := validateCreateFinancialAdjustmentInput(CreateFinancialAdjustmentInput{
		AdjustmentType: FinancialAdjustmentTypeRefund,
		Amount:         100,
		Currency:       "MMK",
		EffectiveAt:    time.Now(),
		CreatedBy:      "admin:1",
		IdempotencyKey: "",
	})
	if err == nil {
		t.Fatal("expected missing idempotency key error")
	}
}
