package config

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v4"
)

type fakeAppConfigStore struct {
	values map[string]string
}

func (f *fakeAppConfigStore) Get(_ context.Context, key string) (string, error) {
	value, ok := f.values[key]
	if !ok {
		return "", pgx.ErrNoRows
	}
	return value, nil
}

func (f *fakeAppConfigStore) Set(_ context.Context, key string, value string) error {
	if f.values == nil {
		f.values = make(map[string]string)
	}
	f.values[key] = value
	return nil
}

func setBaseConfigEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DISABLE_ENV_FILE", "true")
	for _, key := range []string{
		"PLANS",
		"PRICE_1",
		"PRICE_3",
		"PRICE_6",
		"PRICE_12",
		"MOBILE_BANKING_ENABLED",
		"SQUAD_UUIDS",
		"BLOCKED_TELEGRAM_IDS",
		"WHITELISTED_TELEGRAM_IDS",
		"TRIAL_INTERNAL_SQUADS",
		"TRIAL_EXTERNAL_SQUAD_UUID",
		"VISION_PROVIDER_FALLBACK",
		"REMNAWAVE_MODE",
		"HEALTH_CHECK_PORT",
		"DAYS_IN_MONTH",
		"TRAFFIC_LIMIT",
		"VISION_RETRY_ATTEMPTS",
		"VISION_RETRY_MAX_ATTEMPTS",
		"VISION_MAX_ATTEMPTS",
		"VISION_ACCEPT_CONFIDENCE_THRESHOLD",
		"VISION_REJECT_CONFIDENCE_THRESHOLD",
		"BACKUP_RETENTION_DAYS",
		"BACKUP_MAX_LOCAL_FILES",
		"BACKUP_CONFIRM_TTL_MINUTES",
		"BACKUP_JOB_TIMEOUT_SECONDS",
		"BACKUP_RESTORE_TIMEOUT_SECONDS",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("ADMIN_TELEGRAM_ID", "123456")
	t.Setenv("TELEGRAM_TOKEN", "telegram-token")
	t.Setenv("TRIAL_TRAFFIC_LIMIT", "1")
	t.Setenv("TRIAL_DAYS", "1")
	t.Setenv("REMNAWAVE_URL", "https://remnawave.example.com")
	t.Setenv("REMNAWAVE_TOKEN", "remnawave-token")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/app")
	t.Setenv("REFERRAL_DAYS", "30")
}

func requireInitConfigError(t *testing.T, contains string) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("InitConfig() panicked: %v", recovered)
		}
	}()

	err := InitConfig()
	if err == nil {
		t.Fatalf("InitConfig() error = nil, want error containing %q", contains)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("InitConfig() error = %q, want substring %q", err.Error(), contains)
	}
}

func TestInitConfigReportsMissingRequiredEnvWithoutPanic(t *testing.T) {
	t.Setenv("DISABLE_ENV_FILE", "true")
	t.Setenv("ADMIN_TELEGRAM_ID", "")

	requireInitConfigError(t, "ADMIN_TELEGRAM_ID")
}

func TestInitConfigReportsInvalidNumericEnvWithoutPanic(t *testing.T) {
	setBaseConfigEnv(t)
	t.Setenv("HEALTH_CHECK_PORT", "not-a-port")

	requireInitConfigError(t, "HEALTH_CHECK_PORT")
}

func TestInitConfigReportsInvalidVisionFallbackWithoutPanic(t *testing.T) {
	setBaseConfigEnv(t)
	t.Setenv("MOBILE_BANKING_ENABLED", "true")
	t.Setenv("MOBILE_BANKING_PHONE", "09123456789")
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
	t.Setenv("VISION_PROVIDER_FALLBACK", "bogus")

	requireInitConfigError(t, "unsupported VISION_PROVIDER_FALLBACK")
}

func TestInitConfigLoadsMinimalValidEnvironment(t *testing.T) {
	setBaseConfigEnv(t)

	if err := InitConfig(); err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}
	if got := GetAdminTelegramId(); got != 123456 {
		t.Fatalf("GetAdminTelegramId() = %d, want 123456", got)
	}
	if got := RemnawaveMode(); got != "remote" {
		t.Fatalf("RemnawaveMode() = %q, want remote", got)
	}
}

func TestDefaultLanguageFallback(t *testing.T) {
	old := conf.defaultLanguage
	t.Cleanup(func() {
		conf.defaultLanguage = old
	})

	conf.defaultLanguage = ""
	if got := DefaultLanguage(); got != "en" {
		t.Fatalf("DefaultLanguage() = %q, want %q", got, "en")
	}
}

func TestResolveVisionProvidersOpenRouterPrimaryWithoutGemini(t *testing.T) {
	primary, fallback, err := resolveVisionProviders("", "openrouter-key", "", "")
	if err != nil {
		t.Fatalf("resolveVisionProviders() error = %v", err)
	}
	if primary != "openrouter" {
		t.Fatalf("primary = %q, want %q", primary, "openrouter")
	}
	if fallback != "" {
		t.Fatalf("fallback = %q, want empty", fallback)
	}
}

func TestResolveVisionProvidersSupportsExplicitGeminiFallback(t *testing.T) {
	primary, fallback, err := resolveVisionProviders("gemini-key", "openrouter-key", "", "gemini")
	if err != nil {
		t.Fatalf("resolveVisionProviders() error = %v", err)
	}
	if primary != "openrouter" {
		t.Fatalf("primary = %q, want %q", primary, "openrouter")
	}
	if fallback != "gemini" {
		t.Fatalf("fallback = %q, want %q", fallback, "gemini")
	}
}

func TestResolveVisionProvidersUsesOpenRouterFallbackModelWithoutGemini(t *testing.T) {
	primary, fallback, err := resolveVisionProviders("", "openrouter-key", "google/gemini-3.1-flash-lite-preview", "openrouter")
	if err != nil {
		t.Fatalf("resolveVisionProviders() error = %v", err)
	}
	if primary != "openrouter" {
		t.Fatalf("primary = %q, want %q", primary, "openrouter")
	}
	if fallback != "openrouter" {
		t.Fatalf("fallback = %q, want %q", fallback, "openrouter")
	}
}

func TestResolveVisionProvidersRequiresAtLeastOneConfiguredProvider(t *testing.T) {
	_, _, err := resolveVisionProviders("", "", "", "")
	if err == nil {
		t.Fatal("resolveVisionProviders() error = nil, want error")
	}
}

func TestResolveVisionProvidersRejectsMissingFallbackProviderCredentials(t *testing.T) {
	_, _, err := resolveVisionProviders("", "openrouter-key", "", "gemini")
	if err == nil {
		t.Fatal("resolveVisionProviders() error = nil, want error")
	}
}

func TestResolveVisionProvidersRejectsOpenRouterFallbackWithoutFallbackModel(t *testing.T) {
	_, _, err := resolveVisionProviders("", "openrouter-key", "", "openrouter")
	if err == nil {
		t.Fatal("resolveVisionProviders() error = nil, want error")
	}
}

func TestParsePlansEnvAssignsStableFields(t *testing.T) {
	plans, err := ParsePlansEnv("Unlimited|30|10000|0,Pro|90|25000|100")
	if err != nil {
		t.Fatalf("ParsePlansEnv() error = %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("ParsePlansEnv() len = %d, want 2", len(plans))
	}
	if plans[0].ID == "" || plans[1].ID == "" {
		t.Fatal("ParsePlansEnv() returned empty ids")
	}
	if !plans[0].Active || !plans[1].Active {
		t.Fatal("ParsePlansEnv() should mark env plans active")
	}
	if plans[0].SortOrder != 0 || plans[1].SortOrder != 1 {
		t.Fatalf("ParsePlansEnv() sort orders = %d,%d want 0,1", plans[0].SortOrder, plans[1].SortOrder)
	}
}

func TestLoadPlansCatalogSeedsMissingStoreFromCurrentPlans(t *testing.T) {
	original := AllPlans()
	t.Cleanup(func() {
		SetPlans(original)
	})

	SetPlans([]Plan{{
		ID:             "seed-plan",
		Label:          "Unlimited",
		Days:           30,
		Price:          10000,
		TrafficLimitGB: 0,
		SortOrder:      0,
		Active:         true,
	}})

	store := &fakeAppConfigStore{}
	if err := LoadPlansCatalog(context.Background(), store); err != nil {
		t.Fatalf("LoadPlansCatalog() error = %v", err)
	}
	if got := store.values[plansCatalogKey]; !strings.Contains(got, `"id":"seed-plan"`) {
		t.Fatalf("LoadPlansCatalog() stored %q, want seeded plan payload", got)
	}
}

func TestLoadPlansCatalogReplacesInMemoryPlansFromStore(t *testing.T) {
	original := AllPlans()
	t.Cleanup(func() {
		SetPlans(original)
	})

	SetPlans([]Plan{{
		ID:             "env-plan",
		Label:          "Env",
		Days:           30,
		Price:          10000,
		TrafficLimitGB: 0,
		SortOrder:      0,
		Active:         true,
	}})

	store := &fakeAppConfigStore{
		values: map[string]string{
			plansCatalogKey: `[{"id":"db-plan","label":"DB","days":90,"price":25000,"traffic_limit_gb":100,"sort_order":3,"active":false}]`,
		},
	}
	if err := LoadPlansCatalog(context.Background(), store); err != nil {
		t.Fatalf("LoadPlansCatalog() error = %v", err)
	}

	plans := AllPlans()
	if len(plans) != 1 {
		t.Fatalf("AllPlans() len = %d, want 1", len(plans))
	}
	if plans[0].ID != "db-plan" || plans[0].Label != "DB" || plans[0].Active {
		t.Fatalf("AllPlans() = %+v, want db-backed archived plan", plans[0])
	}
}

func TestSavePlansCatalogRejectsDuplicateRenewalSignature(t *testing.T) {
	store := &fakeAppConfigStore{}
	err := SavePlansCatalog(context.Background(), store, []Plan{
		{ID: "starter", Label: "Starter", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
		{ID: "promo", Label: "Promo", Days: 30, Price: 12000, TrafficLimitGB: 0, SortOrder: 1, Active: true},
	})
	if err == nil || !strings.Contains(err.Error(), "duration and traffic combination must be unique") {
		t.Fatalf("SavePlansCatalog() error = %v, want duplicate renewal signature error", err)
	}
}

func TestPlanByIndexUsesStableSortOrderAndRejectsArchivedPlans(t *testing.T) {
	original := AllPlans()
	t.Cleanup(func() {
		SetPlans(original)
	})

	SetPlans([]Plan{
		{ID: "starter", Label: "Starter", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
		{ID: "archived", Label: "Archived", Days: 90, Price: 25000, TrafficLimitGB: 100, SortOrder: 1, Active: false},
		{ID: "pro", Label: "Pro", Days: 90, Price: 22000, TrafficLimitGB: 200, SortOrder: 2, Active: true},
	})

	if got := PlanByIndex(1); got != nil {
		t.Fatalf("PlanByIndex(1) = %+v, want nil for archived slot", got)
	}
	got := PlanByIndex(2)
	if got == nil || got.ID != "pro" {
		t.Fatalf("PlanByIndex(2) = %+v, want plan \"pro\"", got)
	}
}
