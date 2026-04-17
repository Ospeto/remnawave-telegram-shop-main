package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

const plansCatalogKey = "plans_catalog"

type appConfigStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
}

type PlanSlot struct {
	LegacyIndex int
	Plan        Plan
}

func ParsePlansEnv(plansStr string) ([]Plan, error) {
	var plans []Plan
	for _, entry := range strings.Split(plansStr, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "|")
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid PLANS entry %q — expected label|days|price|traffic_gb", entry)
		}
		days, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid days in PLANS entry %q: %w", entry, err)
		}
		price, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			return nil, fmt.Errorf("invalid price in PLANS entry %q: %w", entry, err)
		}
		trafficGB, err := strconv.Atoi(strings.TrimSpace(parts[3]))
		if err != nil {
			return nil, fmt.Errorf("invalid traffic_gb in PLANS entry %q: %w", entry, err)
		}
		plans = append(plans, Plan{
			ID:             uuid.NewString(),
			Label:          strings.TrimSpace(parts[0]),
			Days:           days,
			Price:          price,
			TrafficLimitGB: trafficGB,
			SortOrder:      len(plans),
			Active:         true,
		})
	}
	return normalizePlans(plans)
}

func SetPlans(plans []Plan) {
	setPlans(plans)
}

func AllPlans() []Plan {
	conf.plansMu.RLock()
	defer conf.plansMu.RUnlock()
	return copyPlansLocked(conf.plans)
}

func ActivePlans() []Plan {
	conf.plansMu.RLock()
	defer conf.plansMu.RUnlock()

	active := make([]Plan, 0, len(conf.plans))
	for _, plan := range conf.plans {
		if plan.Active {
			active = append(active, plan)
		}
	}
	return active
}

func ActivePlanSlots() []PlanSlot {
	conf.plansMu.RLock()
	defer conf.plansMu.RUnlock()

	slots := make([]PlanSlot, 0, len(conf.plans))
	for i, plan := range conf.plans {
		if !plan.Active {
			continue
		}
		slots = append(slots, PlanSlot{
			LegacyIndex: plan.SortOrder,
			Plan:        conf.plans[i],
		})
	}
	return slots
}

func PlanByID(id string) *Plan {
	if strings.TrimSpace(id) == "" {
		return nil
	}

	conf.plansMu.RLock()
	defer conf.plansMu.RUnlock()

	for _, plan := range conf.plans {
		if plan.ID == id {
			planCopy := plan
			return &planCopy
		}
	}
	return nil
}

func PlanByIndex(idx int) *Plan {
	if idx < 0 {
		return nil
	}

	conf.plansMu.RLock()
	defer conf.plansMu.RUnlock()

	for _, plan := range conf.plans {
		if plan.SortOrder != idx {
			continue
		}
		if !plan.Active {
			return nil
		}
		planCopy := plan
		return &planCopy
	}
	return nil
}

func LoadPlansCatalog(ctx context.Context, store appConfigStore) error {
	if store == nil {
		return nil
	}

	value, err := store.Get(ctx, plansCatalogKey)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("load plans catalog: %w", err)
		}

		plans := AllPlans()
		if len(plans) == 0 {
			return nil
		}
		return SavePlansCatalog(ctx, store, plans)
	}

	if strings.TrimSpace(value) == "" {
		plans := AllPlans()
		if len(plans) == 0 {
			return nil
		}
		return SavePlansCatalog(ctx, store, plans)
	}

	var plans []Plan
	if err := json.Unmarshal([]byte(value), &plans); err != nil {
		return fmt.Errorf("decode plans catalog: %w", err)
	}

	normalized, err := normalizePlans(plans)
	if err != nil {
		return fmt.Errorf("normalize plans catalog: %w", err)
	}

	setPlans(normalized)
	slog.Info("Loaded plans catalog from app_config", "count", len(normalized))
	return nil
}

func SavePlansCatalog(ctx context.Context, store appConfigStore, plans []Plan) error {
	if store == nil {
		return errors.New("plans catalog store is unavailable")
	}

	normalized, err := normalizePlans(plans)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("marshal plans catalog: %w", err)
	}
	if err := store.Set(ctx, plansCatalogKey, string(payload)); err != nil {
		return fmt.Errorf("persist plans catalog: %w", err)
	}

	setPlans(normalized)
	return nil
}

func normalizePlans(plans []Plan) ([]Plan, error) {
	normalized := make([]Plan, 0, len(plans))
	sortOrders := make(map[int]string, len(plans))
	renewalKeys := make(map[string]string, len(plans))
	for i, plan := range plans {
		plan.Label = strings.TrimSpace(plan.Label)
		if plan.Label == "" {
			return nil, fmt.Errorf("plan label is required")
		}
		if plan.Days <= 0 {
			return nil, fmt.Errorf("plan days must be positive")
		}
		if plan.Price <= 0 {
			return nil, fmt.Errorf("plan price must be positive")
		}
		if plan.TrafficLimitGB < 0 {
			return nil, fmt.Errorf("plan traffic limit cannot be negative")
		}
		if plan.SortOrder < 0 {
			return nil, fmt.Errorf("plan sort_order cannot be negative")
		}
		if strings.TrimSpace(plan.ID) == "" {
			plan.ID = uuid.NewString()
		}
		if existingID, exists := sortOrders[plan.SortOrder]; exists && existingID != plan.ID {
			return nil, fmt.Errorf("plan sort_order %d must be unique", plan.SortOrder)
		}
		sortOrders[plan.SortOrder] = plan.ID

		renewalKey := fmt.Sprintf("%d:%d", plan.Days, plan.TrafficLimitGB)
		if existingID, exists := renewalKeys[renewalKey]; exists && existingID != plan.ID {
			return nil, fmt.Errorf("plan duration and traffic combination must be unique")
		}
		renewalKeys[renewalKey] = plan.ID
		if i == 0 && !plan.Active && len(plans) == 1 {
			// Allow a fully archived single-plan catalog while preserving explicit false.
		}
		normalized = append(normalized, plan)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].SortOrder != normalized[j].SortOrder {
			return normalized[i].SortOrder < normalized[j].SortOrder
		}
		if normalized[i].Label != normalized[j].Label {
			return normalized[i].Label < normalized[j].Label
		}
		if normalized[i].Days != normalized[j].Days {
			return normalized[i].Days < normalized[j].Days
		}
		return normalized[i].ID < normalized[j].ID
	})

	return normalized, nil
}

func setPlans(plans []Plan) {
	normalized, err := normalizePlans(plans)
	if err != nil {
		panic(err)
	}

	conf.plansMu.Lock()
	defer conf.plansMu.Unlock()
	conf.plans = normalized
}

func copyPlansLocked(plans []Plan) []Plan {
	copied := make([]Plan, len(plans))
	copy(copied, plans)
	return copied
}
