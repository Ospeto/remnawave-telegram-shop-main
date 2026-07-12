package payment

import (
	"context"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/database"
)

func TestCreatePurchaseWithOptionalExtendRejectsPostpaidWithoutRepo(t *testing.T) {
	svc := &PaymentService{
		// resellerCreditRepo intentionally nil
	}
	customer := &database.Customer{ID: 1, IsReseller: true}

	_, _, err := svc.createPurchaseWithOptionalExtend(
		context.Background(),
		1000,
		30,
		100,
		customer,
		database.InvoiceTypePostpaid,
		"",
		nil,
	)
	if err == nil {
		t.Fatal("expected error when reseller credit repository is not configured")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %q, want message containing %q", err.Error(), "not configured")
	}
}

func TestCreatePostpaidPurchaseRejectsNonReseller(t *testing.T) {
	svc := &PaymentService{
		resellerCreditRepo: database.NewResellerCreditRepository(nil),
	}
	customer := &database.Customer{ID: 1, IsReseller: false}

	_, _, err := svc.createPostpaidPurchase(context.Background(), 1000, 30, 100, customer, "", nil, nil)
	if err == nil {
		t.Fatal("expected error for non-reseller customer")
	}
	if !strings.Contains(err.Error(), "only available for resellers") {
		t.Fatalf("error = %q, want reseller-only message", err.Error())
	}
}

func TestCreatePostpaidPurchaseRejectsNonPositiveAmount(t *testing.T) {
	svc := &PaymentService{
		resellerCreditRepo: database.NewResellerCreditRepository(nil),
	}
	customer := &database.Customer{ID: 1, IsReseller: true}

	for _, amount := range []float64{0, -1, -0.01} {
		_, _, err := svc.createPostpaidPurchase(context.Background(), amount, 30, 100, customer, "", nil, nil)
		if err == nil {
			t.Fatalf("amount=%v: expected positive-amount error", amount)
		}
		if !strings.Contains(err.Error(), "must be positive") {
			t.Fatalf("amount=%v: error = %q, want positive-amount message", amount, err.Error())
		}
	}
}

func TestCreatePostpaidPurchaseRejectsNilCustomer(t *testing.T) {
	svc := &PaymentService{
		resellerCreditRepo: database.NewResellerCreditRepository(nil),
	}
	_, _, err := svc.createPostpaidPurchase(context.Background(), 1000, 30, 100, nil, "", nil, nil)
	if err == nil {
		t.Fatal("expected error for nil customer")
	}
	if !strings.Contains(err.Error(), "only available for resellers") {
		t.Fatalf("error = %q, want reseller-only message", err.Error())
	}
}

func TestPostpaidSaleIdempotencyKeyHelpers(t *testing.T) {
	if got := postpaidSaleIdempotencyKey(context.Background(), 42); got != "postpaid-sale:42" {
		t.Fatalf("postpaidSaleIdempotencyKey without ctx key = %q, want postpaid-sale:42", got)
	}
	if got := postpaidSaleReverseIdempotencyKey(99); got != "postpaid-sale-reverse:99" {
		t.Fatalf("postpaidSaleReverseIdempotencyKey = %q, want postpaid-sale-reverse:99", got)
	}
}

func TestTriggersReferralConversionExcludesPostpaid(t *testing.T) {
	if triggersReferralConversion(database.InvoiceTypePostpaid, 1000) {
		t.Fatal("postpaid must not trigger referral conversion")
	}
	if !triggersReferralConversion(database.InvoiceTypeWalletPayment, 1000) {
		t.Fatal("wallet_payment should still trigger referral conversion")
	}
	if !triggersReferralConversion(database.InvoiceTypeMobileBanking, 1000) {
		t.Fatal("mobile_banking should still trigger referral conversion")
	}
}

func TestResumeExistingPurchaseHandlesPostpaidPaidAndCancel(t *testing.T) {
	svc := &PaymentService{}

	paid := &database.Purchase{
		ID:          10,
		InvoiceType: database.InvoiceTypePostpaid,
		Status:      database.PurchaseStatusPaid,
	}
	if _, id, err := svc.resumeExistingPurchase(context.Background(), paid); err != nil || id != 10 {
		t.Fatalf("paid postpaid resume: id=%d err=%v", id, err)
	}

	cancelled := &database.Purchase{
		ID:          11,
		InvoiceType: database.InvoiceTypePostpaid,
		Status:      database.PurchaseStatusCancel,
	}
	_, id, err := svc.resumeExistingPurchase(context.Background(), cancelled)
	if err == nil || id != 11 {
		t.Fatalf("cancelled postpaid resume: id=%d err=%v, want cancelled error", id, err)
	}
	if !strings.Contains(err.Error(), "already cancelled") {
		t.Fatalf("cancelled postpaid resume error = %v, want already cancelled", err)
	}
}
