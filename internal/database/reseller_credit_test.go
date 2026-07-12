package database

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeResellerAmountRejectsNonPositive(t *testing.T) {
	if _, err := normalizeResellerAmount(0); err == nil {
		t.Fatal("expected error")
	}
	if _, err := normalizeResellerAmount(-1); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeResellerAmount_TwoDecimals(t *testing.T) {
	got, err := normalizeResellerAmount(10.005)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 10.01 {
		t.Fatalf("got %v want 10.01", got)
	}
}

func TestRemainingCredit(t *testing.T) {
	a := ResellerCreditAccount{CreditLimit: 10000, BalanceOwed: 2500}
	if got := a.RemainingCredit(); got != 7500 {
		t.Fatalf("RemainingCredit = %v, want 7500", got)
	}
}

func TestRemainingCredit_ZeroWhenOverLimit(t *testing.T) {
	// Defensive: if owed somehow exceeds limit, remaining should not go negative.
	a := ResellerCreditAccount{CreditLimit: 100, BalanceOwed: 150}
	if got := a.RemainingCredit(); got != 0 {
		t.Fatalf("RemainingCredit = %v, want 0", got)
	}
}

func TestValidateCreateLedgerEntryInput_Sale(t *testing.T) {
	pid := int64(42)
	amount, err := validateCreateLedgerEntryInput(CreateLedgerEntryInput{
		CustomerID:     1,
		EntryType:      ResellerLedgerEntryTypeSale,
		Direction:      ResellerLedgerDirectionIncrease,
		Amount:         100.005,
		PurchaseID:     &pid,
		EffectiveAt:    time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC),
		CreatedBy:      "system",
		IdempotencyKey: "sale-1",
	}, ResellerLedgerEntryTypeSale, ResellerLedgerDirectionIncrease, true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if amount != 100.01 {
		t.Fatalf("amount = %v want 100.01", amount)
	}
}

func TestValidateCreateLedgerEntryInput_SaleRequiresPurchaseID(t *testing.T) {
	_, err := validateCreateLedgerEntryInput(CreateLedgerEntryInput{
		CustomerID:     1,
		EntryType:      ResellerLedgerEntryTypeSale,
		Direction:      ResellerLedgerDirectionIncrease,
		Amount:         100,
		PurchaseID:     nil,
		CreatedBy:      "system",
		IdempotencyKey: "sale-1",
	}, ResellerLedgerEntryTypeSale, ResellerLedgerDirectionIncrease, true)
	if err == nil {
		t.Fatal("expected purchase_id required error")
	}
}

func TestValidateCreateLedgerEntryInput_SettlementDirection(t *testing.T) {
	_, err := validateCreateLedgerEntryInput(CreateLedgerEntryInput{
		CustomerID:     1,
		EntryType:      ResellerLedgerEntryTypeSettlement,
		Direction:      ResellerLedgerDirectionIncrease, // wrong
		Amount:         50,
		CreatedBy:      "admin:1",
		IdempotencyKey: "settle-1",
	}, ResellerLedgerEntryTypeSettlement, ResellerLedgerDirectionDecrease, false)
	if err == nil {
		t.Fatal("expected direction mismatch error")
	}
}

func TestValidateCreateLedgerEntryInput_RequiresIdempotencyAndCreatedBy(t *testing.T) {
	pid := int64(1)
	base := CreateLedgerEntryInput{
		CustomerID: 1,
		EntryType:  ResellerLedgerEntryTypeSale,
		Direction:  ResellerLedgerDirectionIncrease,
		Amount:     10,
		PurchaseID: &pid,
	}
	if _, err := validateCreateLedgerEntryInput(base, ResellerLedgerEntryTypeSale, ResellerLedgerDirectionIncrease, true); err == nil {
		t.Fatal("expected error for missing created_by/idempotency")
	}
	base.CreatedBy = "system"
	if _, err := validateCreateLedgerEntryInput(base, ResellerLedgerEntryTypeSale, ResellerLedgerDirectionIncrease, true); err == nil {
		t.Fatal("expected error for missing idempotency_key")
	}
}

func TestValidateCreditLimit(t *testing.T) {
	if err := validateCreditLimit(-1, 0); err == nil {
		t.Fatal("expected reject negative limit")
	}
	if err := validateCreditLimit(100, 150); !errors.Is(err, ErrResellerCreditLimitBelowOwed) {
		t.Fatalf("err=%v want ErrResellerCreditLimitBelowOwed", err)
	}
	if err := validateCreditLimit(200, 150); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := validateCreditLimit(150, 150); err != nil {
		t.Fatalf("equal limit and owed should be ok: %v", err)
	}
}

func TestCanIncreaseOwed(t *testing.T) {
	if err := canIncreaseOwed(9000, 1000, 10000); err != nil {
		t.Fatalf("exact limit should pass: %v", err)
	}
	if err := canIncreaseOwed(9000, 1000.01, 10000); !errors.Is(err, ErrResellerInsufficientCredit) {
		t.Fatalf("err=%v want ErrResellerInsufficientCredit", err)
	}
}

func TestCanDecreaseOwed(t *testing.T) {
	if err := canDecreaseOwed(100, 100); err != nil {
		t.Fatalf("full settlement should pass: %v", err)
	}
	if err := canDecreaseOwed(100, 100.01); !errors.Is(err, ErrResellerOverSettlement) {
		t.Fatalf("err=%v want ErrResellerOverSettlement", err)
	}
}

func TestResellerLedgerPayloadMatches(t *testing.T) {
	pid := int64(9)
	eff := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	existing := &ResellerLedgerEntry{
		CustomerID:  1,
		EntryType:   ResellerLedgerEntryTypeSale,
		Direction:   ResellerLedgerDirectionIncrease,
		Amount:      100.01,
		PurchaseID:  &pid,
		EffectiveAt: eff,
	}
	in := CreateLedgerEntryInput{
		CustomerID:  1,
		EntryType:   ResellerLedgerEntryTypeSale,
		Direction:   ResellerLedgerDirectionIncrease,
		Amount:      100.01,
		PurchaseID:  &pid,
		EffectiveAt: eff,
	}
	if !resellerLedgerPayloadMatches(existing, in, 100.01) {
		t.Fatal("expected match")
	}
	// Different amount → mismatch
	if resellerLedgerPayloadMatches(existing, in, 50) {
		t.Fatal("expected amount mismatch")
	}
	// Different purchase_id → mismatch
	other := int64(10)
	in2 := in
	in2.PurchaseID = &other
	if resellerLedgerPayloadMatches(existing, in2, 100.01) {
		t.Fatal("expected purchase_id mismatch")
	}
	// Different effective_at → mismatch
	in3 := in
	in3.EffectiveAt = eff.Add(time.Hour)
	if resellerLedgerPayloadMatches(existing, in3, 100.01) {
		t.Fatal("expected effective_at mismatch")
	}
}

func TestBuildResellerLedgerInsertSQL_Idempotency(t *testing.T) {
	sql := buildInsertResellerLedgerEntrySQL()
	if !strings.Contains(sql, "INSERT INTO reseller_ledger_entry") {
		t.Fatalf("missing insert: %s", sql)
	}
	if !strings.Contains(sql, "ON CONFLICT (idempotency_key) DO NOTHING") {
		t.Fatalf("missing idempotency conflict clause: %s", sql)
	}
	if !strings.Contains(sql, "RETURNING") {
		t.Fatalf("missing RETURNING: %s", sql)
	}
}

func TestBuildLockResellerAccountSQL_ForUpdate(t *testing.T) {
	sql := buildLockResellerAccountSQL()
	if !strings.Contains(sql, "FROM reseller_credit_account") {
		t.Fatalf("missing table: %s", sql)
	}
	if !strings.Contains(sql, "FOR UPDATE") {
		t.Fatalf("missing FOR UPDATE: %s", sql)
	}
}

func TestBuildUpdateBalanceOwedSQL(t *testing.T) {
	inc := buildUpdateBalanceOwedSQL(true)
	if !strings.Contains(inc, "balance_owed = balance_owed + $2") {
		t.Fatalf("increase SQL: %s", inc)
	}
	dec := buildUpdateBalanceOwedSQL(false)
	if !strings.Contains(dec, "balance_owed = balance_owed - $2") {
		t.Fatalf("decrease SQL: %s", dec)
	}
}

func TestBuildEnsureAccountSQL(t *testing.T) {
	sql := buildEnsureAccountSQL()
	if !strings.Contains(sql, "INSERT INTO reseller_credit_account") {
		t.Fatalf("missing insert: %s", sql)
	}
	if !strings.Contains(sql, "ON CONFLICT (customer_id) DO NOTHING") && !strings.Contains(sql, "ON CONFLICT DO NOTHING") {
		// Accept either form; we use (customer_id) for clarity.
		if !strings.Contains(sql, "ON CONFLICT") {
			t.Fatalf("missing ON CONFLICT: %s", sql)
		}
	}
}

func TestBuildListLedgerSQL(t *testing.T) {
	sql := buildListLedgerSQL()
	if !strings.Contains(sql, "FROM reseller_ledger_entry") {
		t.Fatalf("missing table: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY") {
		t.Fatalf("missing order: %s", sql)
	}
	if !strings.Contains(sql, "LIMIT") || !strings.Contains(sql, "OFFSET") {
		t.Fatalf("missing pagination: %s", sql)
	}
}

func TestValidateCreateLedgerEntryInput_AdjustmentEitherDirection(t *testing.T) {
	// Adjustment allows either direction; requiredType is adjustment, requiredDir empty means any.
	eff := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	amount, err := validateCreateLedgerEntryInput(CreateLedgerEntryInput{
		CustomerID:     1,
		EntryType:      ResellerLedgerEntryTypeAdjustment,
		Direction:      ResellerLedgerDirectionIncrease,
		Amount:         25,
		EffectiveAt:    eff,
		CreatedBy:      "admin:1",
		IdempotencyKey: "adj-1",
	}, ResellerLedgerEntryTypeAdjustment, "", false)
	if err != nil {
		t.Fatalf("increase adj: %v", err)
	}
	if amount != 25 {
		t.Fatalf("amount=%v", amount)
	}
	_, err = validateCreateLedgerEntryInput(CreateLedgerEntryInput{
		CustomerID:     1,
		EntryType:      ResellerLedgerEntryTypeAdjustment,
		Direction:      ResellerLedgerDirectionDecrease,
		Amount:         25,
		EffectiveAt:    eff,
		CreatedBy:      "admin:1",
		IdempotencyKey: "adj-2",
	}, ResellerLedgerEntryTypeAdjustment, "", false)
	if err != nil {
		t.Fatalf("decrease adj: %v", err)
	}
	// Wrong entry type
	_, err = validateCreateLedgerEntryInput(CreateLedgerEntryInput{
		CustomerID:     1,
		EntryType:      ResellerLedgerEntryTypeSale,
		Direction:      ResellerLedgerDirectionIncrease,
		Amount:         25,
		EffectiveAt:    eff,
		CreatedBy:      "admin:1",
		IdempotencyKey: "adj-3",
	}, ResellerLedgerEntryTypeAdjustment, "", false)
	if err == nil {
		t.Fatal("expected entry type mismatch")
	}
}

func TestBuildSumSettlementsByPeriodSQL_YangonAndSettlementOnly(t *testing.T) {
	sql, err := buildSumSettlementsByPeriodSQL(RevenuePeriodDay)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Asia/Yangon",
		"entry_type = 'settlement'",
		"effective_at >= $1",
		"effective_at < $2",
		"'MMK'",
		"reseller_ledger_entry",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in %s", want, sql)
		}
	}
	// Must not count sales or adjustments as cash.
	if strings.Contains(sql, "entry_type = 'sale'") || strings.Contains(sql, "adjustment") {
		t.Fatalf("settlement SQL must not include sale/adjustment filters: %s", sql)
	}
}

func TestBuildSumSettlementsByPeriodSQL_WeekBucket(t *testing.T) {
	sql, err := buildSumSettlementsByPeriodSQL(RevenuePeriodWeek)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "DATE_TRUNC('week'") {
		t.Fatalf("week bucket missing: %s", sql)
	}
}

func TestBuildSumSettlementsByPeriodSQL_UnsupportedPeriod(t *testing.T) {
	_, err := buildSumSettlementsByPeriodSQL(RevenueSummaryPeriod("quarter"))
	if err == nil {
		t.Fatal("expected unsupported period error")
	}
}
