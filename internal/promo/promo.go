package promo

import (
	"fmt"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type CreateParams struct {
	Code            string `json:"code"`
	DiscountPercent int    `json:"discount_percent"`
	DurationDays    int    `json:"duration_days"`
	MaxUses         int    `json:"max_uses"`
}

const (
	StatusActive    = "active"
	StatusExpired   = "expired"
	StatusExhausted = "exhausted"
)

func ParseCreateCommand(command string) (CreateParams, error) {
	args := strings.Fields(command)
	if len(args) != 5 {
		return CreateParams{}, fmt.Errorf("Usage: /addpromo <code_name> <discount_percent> <duration_days> <max_uses>\nExample: /addpromo sale50 50%% 10days 100code")
	}

	discount, err := parsePositiveInt(strings.TrimSuffix(args[2], "%"))
	if err != nil || discount > 100 {
		return CreateParams{}, fmt.Errorf("Invalid discount percent. Must be 1-100.")
	}

	durationDays, err := parsePositiveInt(strings.TrimSuffix(args[3], "days"))
	if err != nil {
		return CreateParams{}, fmt.Errorf("Invalid duration days.")
	}

	maxUses, err := parsePositiveInt(strings.TrimSuffix(args[4], "code"))
	if err != nil {
		return CreateParams{}, fmt.Errorf("Invalid max uses.")
	}

	return ValidateCreateParams(args[1], discount, durationDays, maxUses)
}

func ValidateCreateParams(code string, discountPercent, durationDays, maxUses int) (CreateParams, error) {
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		return CreateParams{}, fmt.Errorf("Promo code is required.")
	}
	if discountPercent <= 0 || discountPercent > 100 {
		return CreateParams{}, fmt.Errorf("Invalid discount percent. Must be 1-100.")
	}
	if durationDays <= 0 {
		return CreateParams{}, fmt.Errorf("Invalid duration days.")
	}
	if maxUses <= 0 {
		return CreateParams{}, fmt.Errorf("Invalid max uses.")
	}

	return CreateParams{
		Code:            normalizedCode,
		DiscountPercent: discountPercent,
		DurationDays:    durationDays,
		MaxUses:         maxUses,
	}, nil
}

func (p CreateParams) ValidUntilAt(now time.Time) time.Time {
	return now.Add(time.Duration(p.DurationDays) * 24 * time.Hour)
}

type CreateSpec struct {
	Code            string
	DiscountPercent int
	DurationDays    int
	MaxUses         int
	ValidUntil      time.Time
}

func BuildCreateSpec(params CreateParams, now time.Time) (CreateSpec, error) {
	validated, err := ValidateCreateParams(params.Code, params.DiscountPercent, params.DurationDays, params.MaxUses)
	if err != nil {
		return CreateSpec{}, err
	}

	return CreateSpec{
		Code:            validated.Code,
		DiscountPercent: validated.DiscountPercent,
		DurationDays:    validated.DurationDays,
		MaxUses:         validated.MaxUses,
		ValidUntil:      validated.ValidUntilAt(now),
	}, nil
}

func ParseBotCreateFields(fields []string, now time.Time) (CreateSpec, error) {
	if len(fields) != 4 {
		return CreateSpec{}, fmt.Errorf("Usage: /addpromo <code_name> <discount_percent> <duration_days> <max_uses>\nExample: /addpromo sale50 50%% 10days 100code")
	}

	params, err := ParseCreateCommand("/addpromo " + strings.Join(fields, " "))
	if err != nil {
		return CreateSpec{}, err
	}

	return BuildCreateSpec(params, now)
}

func StatusAt(code database.PromoCode, now time.Time) string {
	switch {
	case !code.ValidUntil.After(now):
		return StatusExpired
	case code.UsedCount >= code.MaxUses:
		return StatusExhausted
	default:
		return StatusActive
	}
}

func StatusForCode(code database.PromoCode, now time.Time) string {
	return StatusAt(code, now)
}

func parsePositiveInt(value string) (int, error) {
	var parsed int
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed); err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return parsed, nil
}
