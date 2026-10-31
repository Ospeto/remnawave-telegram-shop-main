package payment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/translation"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
)

// Run against a disposable local PostgreSQL database with WAVY_TEST_DATABASE_URL.
// Each invocation creates its own schema and applies the real application migrations.
func trialTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("WAVY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WAVY_TEST_DATABASE_URL to run PostgreSQL trial renewal contracts")
	}
	ctx := context.Background()
	admin, err := pgxpool.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "trial_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrations, err := filepath.Glob("../../db/migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("migrations: %v", err)
	}
	for _, path := range migrations {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	return pool
}

func TestTrialPaidRenewalContract(t *testing.T) {
	pool := trialTestPool(t)
	ctx := context.Background()
	paidSquad, trialSquad := uuid.NewString(), uuid.NewString()
	paidExternal, trialExternal := uuid.NewString(), uuid.NewString()
	for k, v := range map[string]string{
		"DISABLE_ENV_FILE": "true", "ADMIN_TELEGRAM_ID": "1", "TELEGRAM_TOKEN": "test", "TRIAL_TRAFFIC_LIMIT": "1", "TRIAL_DAYS": "1",
		"REMNAWAVE_URL": "http://localhost", "REMNAWAVE_TOKEN": "test", "DATABASE_URL": "postgres://unused", "REFERRAL_DAYS": "0",
		"SQUAD_UUIDS": paidSquad, "TRIAL_INTERNAL_SQUADS": trialSquad, "EXTERNAL_SQUAD_UUID": paidExternal, "TRIAL_EXTERNAL_SQUAD_UUID": trialExternal,
		"REMNAWAVE_TAG": "PAID", "TRIAL_REMNAWAVE_TAG": "TRIAL", "TRAFFIC_LIMIT_RESET_STRATEGY": "MONTH", "TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY": "NO_RESET",
		"MOBILE_BANKING_ENABLED": "true", "MOBILE_BANKING_PHONE": "09123456789", "GEMINI_API_KEY": "test-not-used",
	} {
		t.Setenv(k, v)
	}
	if err := config.InitConfig(); err != nil {
		t.Fatal(err)
	}

	oldPhone := PhoneKPay
	PhoneKPay = "09123456789"
	t.Cleanup(func() { PhoneKPay = oldPhone })

	var remote map[string]any
	var patches []map[string]any
	creates := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/bottest/"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":42,"type":"private"}}}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/users":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			remote = body
			remote["uuid"] = uuid.NewString()
			remote["subscriptionUrl"] = "https://sub.example.com/unchanged"
			remote["userTraffic"] = map[string]any{"usedTrafficBytes": 0}
			creates++
		case r.Method == http.MethodPatch && r.URL.Path == "/api/users":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			patches = append(patches, body)
			for k, v := range body {
				remote[k] = v
			}
		case strings.HasPrefix(r.URL.Path, "/api/users/by-telegram-id/"):
			users := []any{}
			if remote != nil {
				users = append(users, remote)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"response": users})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/users/"):
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"response": remote})
	}))
	defer server.Close()
	client, err := remnawave.NewClient(server.URL, "test", "remote")
	if err != nil {
		t.Fatal(err)
	}
	telegram, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	service := NewPaymentService(translation.GetInstance(), database.NewPurchaseRepository(pool), client, database.NewCustomerRepository(pool), telegram, database.NewReferralRepository(pool), cache.NewCache(time.Minute), nil, nil, database.NewSubscriptionKeyRepository(pool), nil, database.NewWalletTransactionRepository(pool), nil)

	for i, tc := range []struct {
		name      string
		expired   bool
		unlimited bool
		invoice   bool
		legacy    string
		clearPaid bool
	}{
		{name: "active wallet limited"},
		{name: "expired invoice unlimited", expired: true, unlimited: true, invoice: true},
		{name: "legacy remote-only trial", legacy: "missing", unlimited: true},
		{name: "legacy placeholder trial", legacy: "placeholder", expired: true},
		{name: "clear trial-only paid metadata", clearPaid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.clearPaid {
				t.Setenv("EXTERNAL_SQUAD_UUID", "")
				t.Setenv("REMNAWAVE_TAG", "")
				if err := config.InitConfig(); err != nil {
					t.Fatal(err)
				}
			}
			remote = nil
			patches = nil
			tgID := int64(42 + i)
			var customerID int64
			if err := pool.QueryRow(ctx, "INSERT INTO customer (telegram_id, balance, language) VALUES ($1,10000,'en') RETURNING id", tgID).Scan(&customerID); err != nil {
				t.Fatal(err)
			}
			link, err := service.ActivateTrial(ctx, tgID)
			if err != nil {
				t.Fatal(err)
			}
			keys, err := service.subKeyRepo.FindByCustomerID(ctx, customerID)
			if err != nil || len(keys) != 1 {
				t.Fatalf("trial must persist exactly one key: keys=%v err=%v", keys, err)
			}
			key := keys[0]
			if key.ID <= 0 || key.RemnawaveUUID.String() != remote["uuid"] || key.SubscriptionURL != link || key.TrafficLimitGB != 1 {
				t.Fatalf("trial key identity/traffic not persisted: %+v", key)
			}
			if remote["tag"] != "TRIAL" {
				t.Fatalf("trial not configured: %v", remote)
			}
			if tc.legacy != "" {
				if tc.legacy == "missing" {
					_, err = pool.Exec(ctx, "DELETE FROM subscription_key WHERE id=$1", key.ID)
				} else {
					_, err = pool.Exec(ctx, "UPDATE subscription_key SET remnawave_uuid=$1 WHERE id=$2", uuid.New(), key.ID)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err := service.SyncKeys(ctx, customerID, tgID); err != nil {
					t.Fatal(err)
				}
				reconciled, err := service.subKeyRepo.FindByCustomerID(ctx, customerID)
				if err != nil || len(reconciled) != 1 {
					t.Fatalf("legacy reconciliation duplicated/lost key: %v %v", reconciled, err)
				}
				if tc.legacy == "placeholder" && reconciled[0].ID != key.ID {
					t.Fatal("placeholder reconciliation changed key ID")
				}
				if reconciled[0].RemnawaveUUID != key.RemnawaveUUID || reconciled[0].SubscriptionURL != link {
					t.Fatal("legacy reconciliation lost remote identity")
				}
				key = reconciled[0]
				service.syncCache.Delete(customerID)
				if _, err := service.SyncKeys(ctx, customerID, tgID); err != nil {
					t.Fatal(err)
				}
			}
			base := *key.ExpireAt
			if tc.expired {
				base = time.Now().UTC().Add(-24 * time.Hour)
				remote["expireAt"] = base.Format(time.RFC3339Nano)
				if err := service.subKeyRepo.UpdateExpiry(ctx, key.ID, base); err != nil {
					t.Fatal(err)
				}
				if err := service.subKeyRepo.UpdateStatus(ctx, key.ID, "expired"); err != nil {
					t.Fatal(err)
				}
				base = time.Now().UTC()
			}
			customer, err := service.customerRepository.FindById(ctx, customerID)
			if err != nil {
				t.Fatal(err)
			}
			traffic, wantTraffic := 10, 11
			if tc.unlimited {
				traffic, wantTraffic = 0, 0
			}
			idemCtx := WithIdempotencyKey(ctx, uuid.New())
			invoiceType := database.InvoiceTypeWalletPayment
			if tc.invoice {
				invoiceType = database.InvoiceTypeMobileBanking
			}
			_, purchaseID, err := service.CreatePurchaseWithExtendForInvoice(idemCtx, 5000, 30, traffic, customer, invoiceType, "", key.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.invoice {
				if err := service.ProcessPurchaseById(ctx, purchaseID); err != nil {
					t.Fatal(err)
				}
			}
			if err := service.ProcessPurchaseById(ctx, purchaseID); err != nil {
				t.Fatal(err)
			}
			if _, retryID, err := service.CreatePurchaseWithExtendForInvoice(idemCtx, 5000, 30, traffic, customer, invoiceType, "", key.ID); err != nil || retryID != purchaseID {
				t.Fatalf("idempotent retry: id=%d err=%v", retryID, err)
			}
			if len(patches) != 1 {
				t.Fatalf("retry extended remote %d times", len(patches))
			}
			patch := patches[0]
			squads, _ := patch["activeInternalSquads"].([]any)
			var wantExternal, wantTag any = paidExternal, "PAID"
			if tc.clearPaid {
				wantExternal, wantTag = nil, nil
			}
			_, hasExternal := patch["externalSquadUuid"]
			_, hasTag := patch["tag"]
			if len(squads) != 1 || squads[0] != paidSquad || !hasExternal || !hasTag || patch["externalSquadUuid"] != wantExternal || patch["tag"] != wantTag || patch["trafficLimitStrategy"] != "MONTH" {
				t.Fatalf("trial not upgraded to paid configuration: %v", patch)
			}
			updated, err := service.subKeyRepo.FindByID(ctx, key.ID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.RemnawaveUUID != key.RemnawaveUUID || updated.SubscriptionURL != link || updated.Status != "active" || updated.TrafficLimitGB != wantTraffic {
				t.Fatalf("renewal changed identity or lost quota: %+v", updated)
			}
			wantExpiry := base.AddDate(0, 0, 30)
			if delta := updated.ExpireAt.Sub(wantExpiry); delta < -2*time.Second || delta > 2*time.Second {
				t.Fatalf("expiry=%s want=%s", updated.ExpireAt, wantExpiry)
			}
			customer, err = service.customerRepository.FindById(ctx, customerID)
			if err != nil {
				t.Fatal(err)
			}
			if customer.TrialUsedAt == nil || customer.SubscriptionLink == nil || *customer.SubscriptionLink != link || !customer.ExpireAt.Equal(*updated.ExpireAt) {
				t.Fatalf("canonical state or durable trial marker lost: %+v", customer)
			}
			wantBalance := float64(5000)
			if tc.invoice {
				wantBalance = 10000
			}
			if customer.Balance != wantBalance {
				t.Fatalf("balance=%v want=%v", customer.Balance, wantBalance)
			}
			if _, err := service.ActivateTrial(ctx, tgID); !errors.Is(err, ErrTrialAlreadyUsed) {
				t.Fatalf("second trial: %v", err)
			}
			// A remote user must not resurrect a locally deleted key.
			if err := service.subKeyRepo.UpdateStatus(ctx, key.ID, "deleted"); err != nil {
				t.Fatal(err)
			}
			service.syncCache.Delete(customerID)
			if _, err := service.SyncKeys(ctx, customerID, tgID); err != nil {
				t.Fatal(err)
			}
			deleted, err := service.subKeyRepo.FindByID(ctx, key.ID)
			if err != nil || deleted.Status != "deleted" {
				t.Fatalf("sync resurrected deleted key: %v %v", deleted, err)
			}
			if _, err := service.ActivateTrial(ctx, tgID); !errors.Is(err, ErrTrialAlreadyUsed) {
				t.Fatalf("second trial after deletion: %v", err)
			}
			// Tombstones from the old migration may still hold placeholder UUIDs.
			if _, err := pool.Exec(ctx, "UPDATE subscription_key SET remnawave_uuid=$1 WHERE id=$2", uuid.New(), key.ID); err != nil {
				t.Fatal(err)
			}
			service.syncCache.Delete(customerID)
			if _, err := service.SyncKeys(ctx, customerID, tgID); err != nil {
				t.Fatal(err)
			}
			remaining, err := service.subKeyRepo.FindByCustomerID(ctx, customerID)
			if err != nil || len(remaining) != 1 || remaining[0].Status != "deleted" {
				t.Fatalf("sync recreated legacy tombstone: %v %v", remaining, err)
			}
		})
	}
	if creates != 5 {
		t.Fatalf("paid renewals created replacement users: %d creates", creates)
	}
}

func TestSyncExpiryStatusDoesNotResurrectDeletedKey(t *testing.T) {
	pool := trialTestPool(t)
	ctx := context.Background()

	var customerID int64
	if err := pool.QueryRow(ctx, "INSERT INTO customer (telegram_id, balance, language) VALUES ($1, 0, 'en') RETURNING id", int64(88888)).Scan(&customerID); err != nil {
		t.Fatal(err)
	}

	repo := database.NewSubscriptionKeyRepository(pool)
	initialExpiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)
	keyID, err := repo.Create(ctx, &database.SubscriptionKey{
		CustomerID:      customerID,
		RemnawaveUUID:   uuid.New(),
		Username:        "test_sync_tombstone",
		SubscriptionURL: "https://sub.example.com/test_sync_tombstone",
		ExpireAt:        &initialExpiry,
		Status:          "active",
		Label:           "Test",
		TrafficLimitGB:  5,
	})
	if err != nil {
		t.Fatal(err)
	}

	notifiedAt := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Microsecond)
	if err := repo.MarkExpirationNotified(ctx, keyID, notifiedAt); err != nil {
		t.Fatalf("failed to mark expiration notified: %v", err)
	}

	// Hold a stale row snapshot
	staleKey, err := repo.FindByID(ctx, keyID)
	if err != nil || staleKey == nil {
		t.Fatalf("failed to load stale row snapshot: %v", err)
	}

	// Tombstone it
	if err := repo.UpdateStatus(ctx, staleKey.ID, "deleted"); err != nil {
		t.Fatalf("failed to tombstone key: %v", err)
	}

	// Invoke the conditional sync write with changed expiry and active status
	newExpiry := initialExpiry.Add(48 * time.Hour)
	affected, err := repo.SyncExpiryStatus(ctx, staleKey.ID, newExpiry, "active")
	if err != nil {
		t.Fatalf("unexpected error from SyncExpiryStatus: %v", err)
	}
	if affected {
		t.Fatalf("expected affected=false when updating tombstoned key, got true")
	}

	// Assert no resurrection and unchanged deleted state/expiry
	after, err := repo.FindByID(ctx, staleKey.ID)
	if err != nil || after == nil {
		t.Fatalf("failed to re-query key: %v", err)
	}
	if after.Status != "deleted" {
		t.Fatalf("resurrected deleted key: status=%q, want 'deleted'", after.Status)
	}
	if after.ExpireAt == nil || !after.ExpireAt.Equal(*staleKey.ExpireAt) {
		t.Fatalf("expiry was changed on tombstoned key: got %v, want %v", after.ExpireAt, staleKey.ExpireAt)
	}
	if after.ExpirationNotifiedAt == nil || !after.ExpirationNotifiedAt.Equal(notifiedAt) {
		t.Fatalf("notification timestamp was altered on tombstoned key: got %v, want %v", after.ExpirationNotifiedAt, notifiedAt)
	}

	// Also invoke conditional sync write with expired status
	affectedExpired, err := repo.SyncExpiryStatus(ctx, staleKey.ID, newExpiry, "expired")
	if err != nil {
		t.Fatalf("unexpected error from SyncExpiryStatus: %v", err)
	}
	if affectedExpired {
		t.Fatalf("expected affected=false when updating tombstoned key with expired status, got true")
	}

	afterExpired, err := repo.FindByID(ctx, staleKey.ID)
	if err != nil || afterExpired == nil {
		t.Fatalf("failed to re-query key after expired write attempt: %v", err)
	}
	if afterExpired.Status != "deleted" {
		t.Fatalf("resurrected deleted key: status=%q, want 'deleted'", afterExpired.Status)
	}
	if afterExpired.ExpireAt == nil || !afterExpired.ExpireAt.Equal(*staleKey.ExpireAt) {
		t.Fatalf("expiry was changed on tombstoned key: got %v, want %v", afterExpired.ExpireAt, staleKey.ExpireAt)
	}

	// Verify that SyncExpiryStatus updates active keys and resets notification only on change
	activeKeyID, err := repo.Create(ctx, &database.SubscriptionKey{
		CustomerID:      customerID,
		RemnawaveUUID:   uuid.New(),
		Username:        "test_sync_active",
		SubscriptionURL: "https://sub.example.com/test_sync_active",
		ExpireAt:        &initialExpiry,
		Status:          "active",
		Label:           "Active Test",
		TrafficLimitGB:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkExpirationNotified(ctx, activeKeyID, notifiedAt); err != nil {
		t.Fatalf("failed to mark notification on active key: %v", err)
	}

	// Unchanged write: notification timestamp is preserved
	affectedUnchanged, err := repo.SyncExpiryStatus(ctx, activeKeyID, initialExpiry, "active")
	if err != nil || !affectedUnchanged {
		t.Fatalf("unchanged sync write failed: affected=%v err=%v", affectedUnchanged, err)
	}
	activeUnchanged, err := repo.FindByID(ctx, activeKeyID)
	if err != nil || activeUnchanged == nil {
		t.Fatalf("failed to re-query active key for unchanged check: %v", err)
	}
	if activeUnchanged.ExpirationNotifiedAt == nil || !activeUnchanged.ExpirationNotifiedAt.Equal(notifiedAt) {
		t.Fatalf("unchanged write should preserve notification timestamp: got %v, want %v", activeUnchanged.ExpirationNotifiedAt, notifiedAt)
	}

	// Changed write: notification timestamp is reset to nil
	affectedChanged, err := repo.SyncExpiryStatus(ctx, activeKeyID, newExpiry, "active")
	if err != nil || !affectedChanged {
		t.Fatalf("changed sync write failed: affected=%v err=%v", affectedChanged, err)
	}
	activeChanged, err := repo.FindByID(ctx, activeKeyID)
	if err != nil || activeChanged == nil {
		t.Fatalf("failed to re-query active key for changed check: %v", err)
	}
	if activeChanged.ExpirationNotifiedAt != nil {
		t.Fatalf("changed write should reset notification timestamp: got %v, want nil", activeChanged.ExpirationNotifiedAt)
	}
	if activeChanged.ExpireAt == nil || !activeChanged.ExpireAt.Equal(newExpiry) {
		t.Fatalf("changed write did not update expiry: got %v, want %v", activeChanged.ExpireAt, newExpiry)
	}
}

func TestHiddenKey_SyncKeysReconciliationAndRenewalContract(t *testing.T) {
	pool := trialTestPool(t)
	ctx := context.Background()

	paidSquad, trialSquad := uuid.NewString(), uuid.NewString()
	paidExternal, trialExternal := uuid.NewString(), uuid.NewString()
	for k, v := range map[string]string{
		"DISABLE_ENV_FILE": "true", "ADMIN_TELEGRAM_ID": "1", "TELEGRAM_TOKEN": "test", "TRIAL_TRAFFIC_LIMIT": "1", "TRIAL_DAYS": "1",
		"REMNAWAVE_URL": "http://localhost", "REMNAWAVE_TOKEN": "test", "DATABASE_URL": "postgres://unused", "REFERRAL_DAYS": "0",
		"SQUAD_UUIDS": paidSquad, "TRIAL_INTERNAL_SQUADS": trialSquad, "EXTERNAL_SQUAD_UUID": paidExternal, "TRIAL_EXTERNAL_SQUAD_UUID": trialExternal,
		"REMNAWAVE_TAG": "PAID", "TRIAL_REMNAWAVE_TAG": "TRIAL", "TRAFFIC_LIMIT_RESET_STRATEGY": "MONTH", "TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY": "NO_RESET",
		"MOBILE_BANKING_ENABLED": "true", "MOBILE_BANKING_PHONE": "09123456789", "GEMINI_API_KEY": "test-not-used",
	} {
		t.Setenv(k, v)
	}
	if err := config.InitConfig(); err != nil {
		t.Fatal(err)
	}

	remoteUUID := uuid.NewString()
	subURL := "https://sub.example.com/hidden-user"
	pastExpiry := time.Now().UTC().Add(-24 * time.Hour)
	remoteUser := map[string]any{
		"uuid":            remoteUUID,
		"username":        "hidden_user",
		"subscriptionUrl": subURL,
		"expireAt":        pastExpiry.Format(time.RFC3339Nano),
		"tag":             "TRIAL",
		"userTraffic":     map[string]any{"usedTrafficBytes": 0},
	}

	var patches []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/bottest/"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":42,"type":"private"}}}`))
			return
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/users"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			patches = append(patches, body)
			for k, v := range body {
				remoteUser[k] = v
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"response": remoteUser})
			return
		case strings.HasPrefix(r.URL.Path, "/api/users/by-telegram-id/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"response": []any{remoteUser}})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/users/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"response": remoteUser})
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := remnawave.NewClient(server.URL, "test", "remote")
	if err != nil {
		t.Fatal(err)
	}
	telegram, err := bot.New("test", bot.WithSkipGetMe(), bot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}

	service := NewPaymentService(
		translation.GetInstance(),
		database.NewPurchaseRepository(pool),
		client,
		database.NewCustomerRepository(pool),
		telegram,
		database.NewReferralRepository(pool),
		cache.NewCache(time.Minute),
		nil, nil,
		database.NewSubscriptionKeyRepository(pool),
		nil,
		database.NewWalletTransactionRepository(pool),
		nil,
	)

	tgID := int64(777123)
	var customerID int64
	if err := pool.QueryRow(ctx, "INSERT INTO customer (telegram_id, balance, language) VALUES ($1, 10000, 'en') RETURNING id", tgID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}

	// Create initial expired key with auto_renew=true
	keyID, err := service.subKeyRepo.Create(ctx, &database.SubscriptionKey{
		CustomerID:      customerID,
		RemnawaveUUID:   uuid.MustParse(remoteUUID),
		Username:        "hidden_user",
		SubscriptionURL: subURL,
		ExpireAt:        &pastExpiry,
		Status:          "expired",
		Label:           "Test Key",
		TrafficLimitGB:  5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Hide the expired key
	if err := service.subKeyRepo.HideExpiredKey(ctx, keyID, customerID); err != nil {
		t.Fatalf("HideExpiredKey failed: %v", err)
	}

	hiddenKey, err := service.subKeyRepo.FindByID(ctx, keyID)
	if err != nil || hiddenKey == nil || !hiddenKey.IsHidden {
		t.Fatalf("key was not hidden: %+v, err=%v", hiddenKey, err)
	}

	// 2. SyncKeys with remote user returning the same user
	stats, err := service.SyncKeys(ctx, customerID, tgID)
	if err != nil {
		t.Fatalf("SyncKeys failed: %v", err)
	}
	// Invariant: stats does not expose hidden key
	if len(stats) != 0 {
		t.Fatalf("SyncKeys returned %d stats, want 0 for hidden key", len(stats))
	}

	// Invariant: DB row remains hidden and no duplicate row was created
	keys, err := service.subKeyRepo.FindByCustomerID(ctx, customerID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected exactly 1 key, got %d: %v", len(keys), err)
	}
	if !keys[0].IsHidden {
		t.Fatal("SyncKeys cleared is_hidden flag, want true")
	}

	// 3. Placeholder UUID repair on hidden key:
	// Update local key with random placeholder UUID
	placeholderUUID := uuid.New()
	if _, err := pool.Exec(ctx, "UPDATE subscription_key SET remnawave_uuid = $1 WHERE id = $2", placeholderUUID, keyID); err != nil {
		t.Fatal(err)
	}
	service.syncCache.Delete(customerID)
	if _, err := service.SyncKeys(ctx, customerID, tgID); err != nil {
		t.Fatalf("SyncKeys with placeholder failed: %v", err)
	}
	keysAfterRepair, err := service.subKeyRepo.FindByCustomerID(ctx, customerID)
	if err != nil || len(keysAfterRepair) != 1 {
		t.Fatalf("expected exactly 1 key after placeholder repair, got %d", len(keysAfterRepair))
	}
	if keysAfterRepair[0].RemnawaveUUID.String() != remoteUUID {
		t.Fatalf("placeholder UUID was not repaired: got %s, want %s", keysAfterRepair[0].RemnawaveUUID, remoteUUID)
	}
	if !keysAfterRepair[0].IsHidden {
		t.Fatal("placeholder repair cleared is_hidden flag, want true")
	}

	// 4. Extend hidden key via CreatePurchaseWithExtend
	customer, err := service.customerRepository.FindById(ctx, customerID)
	if err != nil || customer == nil {
		t.Fatal(err)
	}
	_, purchaseID, err := service.CreatePurchaseWithExtend(
		WithIdempotencyKey(ctx, uuid.New()),
		5000, 30, 10, customer, keyID, "",
	)
	if err != nil {
		t.Fatalf("CreatePurchaseWithExtend on hidden key failed: %v", err)
	}
	if purchaseID <= 0 {
		t.Fatalf("expected positive purchaseID, got %d", purchaseID)
	}
	if len(patches) != 1 {
		t.Fatalf("expected 1 remote patch on extension, got %d", len(patches))
	}

	// Invariant: Key extended on remote and locally with original UUID and URL, remains hidden
	extendedKey, err := service.subKeyRepo.FindByID(ctx, keyID)
	if err != nil || extendedKey == nil {
		t.Fatal(err)
	}
	if extendedKey.RemnawaveUUID.String() != remoteUUID {
		t.Fatalf("extension altered remote UUID: %s vs %s", extendedKey.RemnawaveUUID, remoteUUID)
	}
	if extendedKey.SubscriptionURL != subURL {
		t.Fatalf("extension altered subscription URL: %s vs %s", extendedKey.SubscriptionURL, subURL)
	}
	if !extendedKey.IsHidden {
		t.Fatal("extension cleared is_hidden, visibility must remain independent")
	}
	if extendedKey.Status != "active" {
		t.Fatalf("extended key status = %s, want active", extendedKey.Status)
	}

	// Invariant: customer subscription_link does NOT leak hidden key
	updatedCustomer, err := service.customerRepository.FindById(ctx, customerID)
	if err != nil || updatedCustomer == nil {
		t.Fatal(err)
	}
	if updatedCustomer.SubscriptionLink != nil {
		t.Fatalf("customer.SubscriptionLink leaked hidden key: %s", *updatedCustomer.SubscriptionLink)
	}
}
