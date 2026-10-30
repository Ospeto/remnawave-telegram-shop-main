package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@127.0.0.1:54332/wavy_test?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping integration test: cannot connect to test database at %s: %v", dbURL, err)
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping integration test: ping failed for test database at %s: %v", dbURL, err)
		return nil
	}
	return pool
}

// TestIntegration_SumSettlementsByPeriod_ExcludesWalletSettlements tests the real PostgreSQL
// database persistence and reporting query.
func TestIntegration_SumSettlementsByPeriod_ExcludesWalletSettlements(t *testing.T) {
	pool := getTestPool(t)
	if pool == nil {
		return
	}
	defer pool.Close()
	ctx := context.Background()

	const testTelegramID int64 = 12345678
	cleanup := func() {
		var custID int64
		if err := pool.QueryRow(ctx, `SELECT id FROM customer WHERE telegram_id = $1`, testTelegramID).Scan(&custID); err == nil {
			_, _ = pool.Exec(ctx, `DELETE FROM reseller_ledger_entry WHERE customer_id = $1`, custID)
			_, _ = pool.Exec(ctx, `DELETE FROM reseller_credit_account WHERE customer_id = $1`, custID)
			_, _ = pool.Exec(ctx, `DELETE FROM purchase WHERE customer_id = $1`, custID)
			_, _ = pool.Exec(ctx, `DELETE FROM customer WHERE id = $1`, custID)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	loc := revenueSummaryLocation() // Yangon
	start := time.Date(2029, 10, 1, 0, 0, 0, 0, loc).UTC()
	end := time.Date(2029, 11, 1, 0, 0, 0, 0, loc).UTC()

	// 1. Create a customer
	var customerID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO customer (telegram_id, language, balance, is_reseller)
		VALUES ($1, 'en', 100000, true)
		RETURNING id`, testTelegramID).Scan(&customerID)
	if err != nil {
		t.Fatalf("failed to insert customer: %v", err)
	}

	// 2. Ensure reseller credit account with 100,000 balance owed
	resellerRepo := NewResellerCreditRepository(pool)
	_, err = resellerRepo.EnsureAccount(ctx, customerID, 200000)
	if err != nil {
		t.Fatalf("failed to ensure reseller account: %v", err)
	}
	_, err = pool.Exec(ctx, `UPDATE reseller_credit_account SET balance_owed = 100000 WHERE customer_id = $1`, customerID)
	if err != nil {
		t.Fatalf("failed to set balance_owed: %v", err)
	}

	// 3. Insert wallet top-up purchase (100,000 MMK cash collected)
	var topupPurchaseID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO purchase (customer_id, amount, month, currency, status, invoice_type, payment_method, paid_at)
		VALUES ($1, 100000, 1, 'MMK', 'paid', 'wallet_topup', 'mobile_banking', $2)
		RETURNING id`, customerID, time.Date(2029, 10, 1, 10, 0, 0, 0, loc).UTC()).Scan(&topupPurchaseID)
	if err != nil {
		t.Fatalf("failed to insert wallet topup: %v", err)
	}

	// 4. Insert postpaid service purchase (100,000 MMK service delivered, 0 cash)
	var postpaidPurchaseID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO purchase (customer_id, amount, month, currency, status, invoice_type, payment_method, paid_at)
		VALUES ($1, 100000, 1, 'MMK', 'paid', 'postpaid', 'postpaid', $2)
		RETURNING id`, customerID, time.Date(2029, 10, 2, 10, 0, 0, 0, loc).UTC()).Scan(&postpaidPurchaseID)
	if err != nil {
		t.Fatalf("failed to insert postpaid purchase: %v", err)
	}

	// 5. Persist wallet-funded settlement via RecordSettlementTx
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx1.Rollback(ctx) }()

	walletSettleEntry, acctAfterWallet, created, err := resellerRepo.RecordSettlementTx(ctx, tx1, CreateLedgerEntryInput{
		CustomerID:     customerID,
		EntryType:      ResellerLedgerEntryTypeSettlement,
		Direction:      ResellerLedgerDirectionDecrease,
		Amount:         60000, // 60,000 settled from wallet
		EffectiveAt:    time.Date(2029, 10, 3, 10, 0, 0, 0, loc).UTC(),
		CreatedBy:      fmt.Sprintf("customer:%d", testTelegramID),
		IdempotencyKey: "settle-wallet-1",
	})
	if err != nil || !created {
		t.Fatalf("failed to record wallet settlement: created=%v err=%v", created, err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if acctAfterWallet.BalanceOwed != 40000 {
		t.Fatalf("balance_owed after wallet settle = %v want 40000", acctAfterWallet.BalanceOwed)
	}

	// 6. Persist admin external-cash settlement via RecordSettlementTx
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx2.Rollback(ctx) }()

	adminSettleEntry, acctAfterAdmin, created, err := resellerRepo.RecordSettlementTx(ctx, tx2, CreateLedgerEntryInput{
		CustomerID:     customerID,
		EntryType:      ResellerLedgerEntryTypeSettlement,
		Direction:      ResellerLedgerDirectionDecrease,
		Amount:         40000, // 40,000 paid external cash to admin
		EffectiveAt:    time.Date(2029, 10, 4, 10, 0, 0, 0, loc).UTC(),
		CreatedBy:      fmt.Sprintf("admin:%d", 98765432),
		IdempotencyKey: "settle-admin-1",
	})
	if err != nil || !created {
		t.Fatalf("failed to record admin settlement: created=%v err=%v", created, err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if acctAfterAdmin.BalanceOwed != 0 {
		t.Fatalf("balance_owed after admin settle = %v want 0", acctAfterAdmin.BalanceOwed)
	}

	// 7. Execute production SumSettlementsByPeriod query against real PostgreSQL
	settlementRows, err := resellerRepo.SumSettlementsByPeriod(ctx, start, end, RevenuePeriodMonth)
	if err != nil {
		t.Fatalf("SumSettlementsByPeriod failed: %v", err)
	}

	// Assert: Only admin external cash (40,000 MMK) is included.
	// The 60,000 MMK wallet settlement MUST be excluded.
	if len(settlementRows) != 1 {
		t.Fatalf("got %d settlement rows, want 1", len(settlementRows))
	}
	if settlementRows[0].SettlementTotal != 40000 {
		t.Fatalf("GREEN CHECK FAILED: SettlementTotal = %v, want 40000 (admin cash only)", settlementRows[0].SettlementTotal)
	}
	if settlementRows[0].SettlementCount != 1 {
		t.Fatalf("SettlementCount = %v, want 1", settlementRows[0].SettlementCount)
	}

	// 8. RED Regression Check: Execute unpatched SQL query (without created_by LIKE 'admin:%')
	unpatchedSQL := `
		SELECT
			DATE_TRUNC('month', rle.effective_at AT TIME ZONE 'Asia/Yangon')::date AS period_start,
			'MMK' AS currency,
			COALESCE(SUM(rle.amount), 0) AS settlement_total,
			COUNT(*) AS settlement_count
		FROM reseller_ledger_entry rle
		WHERE rle.entry_type = 'settlement'
		  AND rle.effective_at >= $1
		  AND rle.effective_at < $2
		GROUP BY 1, 2`
	var unpatchedTotal float64
	var unpatchedCount int
	var periodStart time.Time
	var currency string
	err = pool.QueryRow(ctx, unpatchedSQL, start, end).Scan(&periodStart, &currency, &unpatchedTotal, &unpatchedCount)
	if err != nil {
		t.Fatalf("unpatched query failed: %v", err)
	}

	// In the unpatched query, both wallet (60,000) and admin (40,000) settlements were summed to 100,000 MMK
	if unpatchedTotal != 100000 {
		t.Fatalf("RED CHECK: unpatched total = %v, expected 100000 (double-counting wallet settlement)", unpatchedTotal)
	}

	// 9. Execute purchase revenue summary query against real PostgreSQL
	purchaseRepo := NewPurchaseRepository(pool)
	purchaseRows, err := purchaseRepo.GetRevenueSummaryRange(ctx, start, end, RevenuePeriodMonth)
	if err != nil {
		t.Fatalf("GetRevenueSummaryRange failed: %v", err)
	}

	if len(purchaseRows) == 0 {
		t.Fatalf("expected purchase rows")
	}

	// Window aggregates: cash collected from purchases is 100,000 (top-up). Service revenue is 100,000 (postpaid).
	if purchaseRows[0].PeriodCashCollected != 100000 {
		t.Fatalf("PeriodCashCollected = %v, want 100000", purchaseRows[0].PeriodCashCollected)
	}
	if purchaseRows[0].PeriodServiceRevenue != 100000 {
		t.Fatalf("PeriodServiceRevenue = %v, want 100000", purchaseRows[0].PeriodServiceRevenue)
	}

	// 10. Combine purchase cash (100,000) + settlement cash (40,000) = 140,000 MMK total cash collected.
	totalCashCollected := purchaseRows[0].PeriodCashCollected + settlementRows[0].SettlementTotal
	if totalCashCollected != 140000 {
		t.Fatalf("total cash collected = %v, want 140000 (100k top-up + 40k admin cash)", totalCashCollected)
	}

	// Verify unpatched total cash collected would be 200,000 (double-counting the 60,000 wallet settlement)
	unpatchedCashCollected := purchaseRows[0].PeriodCashCollected + unpatchedTotal
	if unpatchedCashCollected != 200000 {
		t.Fatalf("unpatched cash collected = %v, want 200000", unpatchedCashCollected)
	}

	// 11. Assert that reporting queries DID NOT alter balances or debt
	acctCheck, err := resellerRepo.GetAccount(ctx, customerID)
	if err != nil {
		t.Fatal(err)
	}
	if acctCheck.BalanceOwed != 0 {
		t.Fatalf("balance_owed mutated by reporting query: %v", acctCheck.BalanceOwed)
	}

	var balanceCheck float64
	err = pool.QueryRow(ctx, `SELECT balance FROM customer WHERE id = $1`, customerID).Scan(&balanceCheck)
	if err != nil {
		t.Fatal(err)
	}
	if balanceCheck != 100000 {
		t.Fatalf("customer balance mutated by reporting query: %v", balanceCheck)
	}

	_ = walletSettleEntry
	_ = adminSettleEntry
}
