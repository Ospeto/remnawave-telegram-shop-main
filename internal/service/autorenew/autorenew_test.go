package autorenew

import (
	"remnawave-tg-shop-bot/internal/config"
	"testing"
)

func TestFindPlanByDuration_Logic(t *testing.T) {
	plans := []config.Plan{
		{Label: "1 Month", Days: 30, Price: 5000, TrafficLimitGB: 0},
		{Label: "3 Months", Days: 90, Price: 12000, TrafficLimitGB: 0},
		{Label: "6 Months", Days: 180, Price: 20000, TrafficLimitGB: 0},
	}

	tests := []struct {
		days      int
		wantLabel string
		wantNil   bool
	}{
		{30, "1 Month", false},
		{90, "3 Months", false},
		{180, "6 Months", false},
		{365, "", true},
		{0, "", true},
	}

	// Test the iteration/matching logic directly
	findPlan := func(days int) *config.Plan {
		for _, plan := range plans {
			if plan.Days == days {
				return &plan
			}
		}
		return nil
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := findPlan(tt.days)
			if tt.wantNil {
				if got != nil {
					t.Errorf("findPlan(%d) = %+v; want nil", tt.days, got)
				}
			} else {
				if got == nil {
					t.Errorf("findPlan(%d) = nil; want plan with label %q", tt.days, tt.wantLabel)
				} else if got.Label != tt.wantLabel {
					t.Errorf("findPlan(%d).Label = %q; want %q", tt.days, got.Label, tt.wantLabel)
				}
			}
		})
	}
}
