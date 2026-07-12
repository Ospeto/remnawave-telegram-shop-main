package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"remnawave-tg-shop-bot/internal/config"
)

type inMemoryPlanConfigStore struct {
	values map[string]string
}

func (s inMemoryPlanConfigStore) Get(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s inMemoryPlanConfigStore) Set(_ context.Context, key string, value string) error {
	s.values[key] = value
	return nil
}

func TestRegisterHandlersProtectsAdminPlanRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "list plans", method: http.MethodGet, path: "/api/admin/plans"},
		{name: "create plan", method: http.MethodPost, path: "/api/admin/plans"},
		{name: "update plan", method: http.MethodPatch, path: "/api/admin/plans/plan-1"},
		{name: "archive plan", method: http.MethodDelete, path: "/api/admin/plans/plan-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://example.com"+tt.path, nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestListAdminPlansIncludesArchivedPlans(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "active", Label: "Active", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
		{ID: "archived", Label: "Archived", Days: 90, Price: 25000, TrafficLimitGB: 100, SortOrder: 1, Active: false},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/plans", nil)
	rec := httptest.NewRecorder()

	handler.ListAdminPlans(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ListAdminPlans() status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload []AdminPlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("ListAdminPlans() len = %d, want 2", len(payload))
	}
	if payload[1].ID != "archived" || payload[1].Active {
		t.Fatalf("ListAdminPlans() archived payload = %+v, want archived plan", payload[1])
	}
}

func TestCreateAdminPlanPersistsAndReturnsCreatedPlan(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "existing", Label: "Existing", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 4, Active: true},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plans", bytes.NewBufferString(`{"label":"Starter","days":45,"price":15000,"traffic_limit_gb":0,"sort_order":0}`))
	rec := httptest.NewRecorder()

	handler.CreateAdminPlan(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateAdminPlan() status = %d, want %d body=%q", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var payload AdminPlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.ID == "" {
		t.Fatal("CreateAdminPlan() returned empty id")
	}
	if payload.SortOrder != 5 {
		t.Fatalf("CreateAdminPlan() sort_order = %d, want 5", payload.SortOrder)
	}
	if !payload.Active {
		t.Fatal("CreateAdminPlan() should create active plan")
	}

	plans := config.AllPlans()
	if len(plans) != 2 {
		t.Fatalf("AllPlans() len = %d, want 2", len(plans))
	}
}

func TestCreateAdminPlanRejectsInvalidTraffic(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/plans", bytes.NewBufferString(`{"label":"Starter","days":30,"price":10000,"traffic_limit_gb":-1,"sort_order":0}`))
	rec := httptest.NewRecorder()

	handler.CreateAdminPlan(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreateAdminPlan() status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateAdminPlanRejectsDuplicateRenewalSignature(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "starter", Label: "Starter", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	store := inMemoryPlanConfigStore{values: map[string]string{}}
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		return config.SavePlansCatalog(context.Background(), store, plans)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plans", bytes.NewBufferString(`{"label":"Promo Starter","days":30,"price":12000,"traffic_limit_gb":0,"sort_order":1}`))
	rec := httptest.NewRecorder()

	handler.CreateAdminPlan(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("CreateAdminPlan() status = %d, want %d body=%q", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestUpdateAdminPlanPreservesIDAndUpdatesFields(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "plan-1", Label: "Starter", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/plans/plan-1", bytes.NewBufferString(`{"label":"Pro","days":90,"price":25000,"traffic_limit_gb":100,"sort_order":2}`))
	rec := httptest.NewRecorder()

	handler.UpdateAdminPlan(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateAdminPlan() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated := config.PlanByID("plan-1")
	if updated == nil {
		t.Fatal("PlanByID(plan-1) = nil")
	}
	if updated.Label != "Pro" || updated.Days != 90 || updated.Price != 25000 || updated.TrafficLimitGB != 100 || updated.SortOrder != 2 {
		t.Fatalf("updated plan = %+v, want updated values", *updated)
	}
}

func TestCreateAdminPlanWithWholesalePrice(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "existing", Label: "Existing", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plans", bytes.NewBufferString(
		`{"label":"Wholesale Starter","days":45,"price":5000,"traffic_limit_gb":0,"sort_order":1,"wholesale_price":4000}`,
	))
	rec := httptest.NewRecorder()

	handler.CreateAdminPlan(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateAdminPlan() status = %d, want %d body=%q", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var payload AdminPlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.WholesalePrice == nil || *payload.WholesalePrice != 4000 {
		t.Fatalf("CreateAdminPlan() wholesale_price = %v, want 4000", payload.WholesalePrice)
	}
	if payload.Price != 5000 {
		t.Fatalf("CreateAdminPlan() price = %d, want 5000", payload.Price)
	}

	created := config.PlanByID(payload.ID)
	if created == nil || created.WholesalePrice == nil || *created.WholesalePrice != 4000 {
		t.Fatalf("persisted plan wholesale = %+v, want 4000", created)
	}
}

func TestCreateAdminPlanRejectsWholesaleAboveRetail(t *testing.T) {
	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/plans", bytes.NewBufferString(
		`{"label":"Bad Wholesale","days":30,"price":5000,"traffic_limit_gb":0,"sort_order":0,"wholesale_price":6000}`,
	))
	rec := httptest.NewRecorder()

	handler.CreateAdminPlan(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("CreateAdminPlan() status = %d, want %d body=%q", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestUpdateAdminPlanClearsWholesalePrice(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	wholesale := 4000
	config.SetPlans([]config.Plan{
		{
			ID:             "plan-1",
			Label:          "Starter",
			Days:           30,
			Price:          5000,
			TrafficLimitGB: 0,
			SortOrder:      0,
			Active:         true,
			WholesalePrice: &wholesale,
		},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/plans/plan-1", bytes.NewBufferString(
		`{"label":"Starter","days":30,"price":5000,"traffic_limit_gb":0,"sort_order":0,"wholesale_price":null}`,
	))
	rec := httptest.NewRecorder()

	handler.UpdateAdminPlan(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("UpdateAdminPlan() status = %d, want %d body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload AdminPlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.WholesalePrice != nil {
		t.Fatalf("UpdateAdminPlan() wholesale_price = %v, want nil/cleared", payload.WholesalePrice)
	}

	updated := config.PlanByID("plan-1")
	if updated == nil {
		t.Fatal("PlanByID(plan-1) = nil")
	}
	if updated.WholesalePrice != nil {
		t.Fatalf("persisted wholesale_price = %v, want nil", updated.WholesalePrice)
	}
}

func TestDeleteAdminPlanRejectsArchivingLastActivePlan(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "plan-1", Label: "Starter", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plans/plan-1", nil)
	rec := httptest.NewRecorder()

	handler.DeleteAdminPlan(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("DeleteAdminPlan() status = %d, want %d", rec.Code, http.StatusConflict)
	}

	plan := config.PlanByID("plan-1")
	if plan == nil || !plan.Active {
		t.Fatalf("PlanByID(plan-1) = %+v, want active plan preserved", plan)
	}
	if len(config.ActivePlans()) != 1 {
		t.Fatalf("ActivePlans() len = %d, want 1", len(config.ActivePlans()))
	}
}

func TestDeleteAdminPlanArchivesWhenAnotherActivePlanExists(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "plan-1", Label: "Starter", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
		{ID: "plan-2", Label: "Pro", Days: 90, Price: 25000, TrafficLimitGB: 100, SortOrder: 1, Active: true},
	})

	handler := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.savePlansCatalog = func(_ context.Context, plans []config.Plan) error {
		config.SetPlans(plans)
		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/plans/plan-1", nil)
	rec := httptest.NewRecorder()

	handler.DeleteAdminPlan(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DeleteAdminPlan() status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	plan := config.PlanByID("plan-1")
	if plan == nil || plan.Active {
		t.Fatalf("PlanByID(plan-1) = %+v, want archived plan", plan)
	}
	if len(config.ActivePlans()) != 1 {
		t.Fatalf("ActivePlans() len = %d, want 1", len(config.ActivePlans()))
	}
}

func TestResolvePurchasePlanSupportsPlanIDAndLegacyIndex(t *testing.T) {
	original := config.AllPlans()
	t.Cleanup(func() {
		config.SetPlans(original)
	})

	config.SetPlans([]config.Plan{
		{ID: "starter", Label: "Starter", Days: 30, Price: 10000, TrafficLimitGB: 0, SortOrder: 0, Active: true},
		{ID: "archived", Label: "Archived", Days: 90, Price: 25000, TrafficLimitGB: 100, SortOrder: 1, Active: false},
		{ID: "pro", Label: "Pro", Days: 90, Price: 22000, TrafficLimitGB: 200, SortOrder: 2, Active: true},
	})

	plan, err := resolvePurchasePlan(CreatePurchaseRequest{PlanID: "pro"})
	if err != nil || plan == nil || plan.ID != "pro" {
		t.Fatalf("resolvePurchasePlan(plan_id) = %+v, %v, want active plan by id", plan, err)
	}

	plan, err = resolvePurchasePlan(CreatePurchaseRequest{PlanIndex: 1})
	if err == nil || !strings.Contains(err.Error(), "Invalid plan index") {
		t.Fatalf("resolvePurchasePlan(archived index) error = %v, want invalid plan index", err)
	}

	plan, err = resolvePurchasePlan(CreatePurchaseRequest{PlanIndex: 2})
	if err != nil || plan == nil || plan.ID != "pro" {
		t.Fatalf("resolvePurchasePlan(plan_index=2) = %+v, %v, want stable active plan", plan, err)
	}

	if _, err := resolvePurchasePlan(CreatePurchaseRequest{PlanID: "archived"}); err == nil || !strings.Contains(err.Error(), "Invalid plan id") {
		t.Fatalf("resolvePurchasePlan(archived) error = %v, want invalid plan id", err)
	}
}
