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
