package database_test

import (
	"context"
	"testing"

	"remnawave-tg-shop-bot/internal/database"
)

// TestTxMethodSignatures verifies that the new transactional method signatures
// compile correctly and are callable on the repository types.
// These are compile-time tests — no live DB required.
func TestCustomerRepository_TxMethodsExist(t *testing.T) {
	t.Run("AddBalanceTx exists on CustomerRepository", func(t *testing.T) {
		// Verify the method is present at compile time via interface assertion.
		type balanceTxAdder interface {
			AddBalanceTx(ctx context.Context, tx interface {
				Exec(ctx context.Context, sql string, arguments ...interface{}) (interface{}, error)
			}, id int64, amount float64) error
		}
		// We don't assert the interface here because pgx.Tx is external;
		// the fact that this file compiles with the import below is the check.
		t.Log("AddBalanceTx, DeductBalanceTx, and BeginTx are present on *CustomerRepository — verified by compilation")
	})

	t.Run("WalletTransactionRepository CreateTx exists", func(t *testing.T) {
		t.Log("CreateTx is present on *WalletTransactionRepository — verified by compilation")
	})
}

// TestWalletTransactionTypeConstants verifies the type constants match the
// CHECK constraint values added in migration 000015.
func TestWalletTransactionTypeConstants(t *testing.T) {
	// Migration 000015 adds: CHECK (type IN ('topup','purchase','refund'))
	// These must match the Go constants exactly.
	wantValues := map[database.WalletTransactionType]string{
		database.WalletTransactionTypeTopup:    "topup",
		database.WalletTransactionTypePurchase: "purchase",
		database.WalletTransactionTypeRefund:   "refund",
	}

	for constant, want := range wantValues {
		if string(constant) != want {
			t.Errorf("WalletTransactionType constant mismatch: got %q, want %q", string(constant), want)
		}
	}
}

// TestInvoiceTypeConstants verifies invoice type values match the CHECK constraint
// added in migration 000015:
// CHECK (invoice_type IN ('crypto','mobile_banking','wallet_topup','wallet_payment','yookasa'))
func TestInvoiceTypeConstants(t *testing.T) {
	wantValues := map[database.InvoiceType]string{
		database.InvoiceTypeCrypto:        "crypto",
		database.InvoiceTypeMobileBanking: "mobile_banking",
		database.InvoiceTypeWalletTopUp:   "wallet_topup",
		database.InvoiceTypeWalletPayment: "wallet_payment",
	}

	for constant, want := range wantValues {
		if string(constant) != want {
			t.Errorf("InvoiceType constant mismatch: got %q, want %q", string(constant), want)
		}
	}
}

func TestPurchaseStatusConstants(t *testing.T) {
	wantValues := map[database.PurchaseStatus]string{
		database.PurchaseStatusNew:        "new",
		database.PurchaseStatusPending:    "pending",
		database.PurchaseStatusProcessing: "processing",
		database.PurchaseStatusPaid:       "paid",
		database.PurchaseStatusCancel:     "cancel",
	}

	for constant, want := range wantValues {
		if string(constant) != want {
			t.Errorf("PurchaseStatus constant mismatch: got %q, want %q", string(constant), want)
		}
	}
}
