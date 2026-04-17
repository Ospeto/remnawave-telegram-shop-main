package healthcheck

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/gemini"
)

type StepStatus string

const (
	StepPass StepStatus = "pass"
	StepFail StepStatus = "fail"
	StepWarn StepStatus = "warn"
	StepSkip StepStatus = "skip"
)

type StepResult struct {
	Name   string
	Status StepStatus
	Detail string
}

type Report struct {
	Success     bool
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Steps       []StepResult
}

type customerStore interface {
	FindByTelegramId(ctx context.Context, telegramID int64) (*database.Customer, error)
	Create(ctx context.Context, customer *database.Customer) (*database.Customer, error)
	UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error
}

type paymentRunner interface {
	CreatePurchase(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, invoiceType database.InvoiceType, promoCode string) (string, int64, error)
}

type subscriptionKeyStore interface {
	FindByCustomerID(ctx context.Context, customerID int64) ([]database.SubscriptionKey, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
}

type remnawaveUserManager interface {
	DeleteUser(ctx context.Context, userUUID uuid.UUID) error
}

type ServiceOptions struct {
	Analyzer            gemini.Analyzer
	Customers           customerStore
	Payments            paymentRunner
	SubscriptionKeys    subscriptionKeyStore
	RemnawaveUsers      remnawaveUserManager
	SyntheticTelegramID int64
	CanaryDays          int
	CanaryTrafficGB     int
	Now                 func() time.Time
}

type Service struct {
	analyzer            gemini.Analyzer
	customers           customerStore
	payments            paymentRunner
	subscriptionKeys    subscriptionKeyStore
	remnawaveUsers      remnawaveUserManager
	syntheticTelegramID int64
	canaryDays          int
	canaryTrafficGB     int
	now                 func() time.Time
	running             atomic.Bool
}

func NewService(opts ServiceOptions) *Service {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	days := opts.CanaryDays
	if days <= 0 {
		days = 1
	}
	traffic := opts.CanaryTrafficGB
	if traffic <= 0 {
		traffic = 1
	}

	return &Service{
		analyzer:            opts.Analyzer,
		customers:           opts.Customers,
		payments:            opts.Payments,
		subscriptionKeys:    opts.SubscriptionKeys,
		remnawaveUsers:      opts.RemnawaveUsers,
		syntheticTelegramID: opts.SyntheticTelegramID,
		canaryDays:          days,
		canaryTrafficGB:     traffic,
		now:                 nowFn,
	}
}

func DefaultSyntheticTelegramID(adminTelegramID int64) int64 {
	if adminTelegramID < 0 {
		adminTelegramID = -adminTelegramID
	}
	return 9_000_000_000_000 + (adminTelegramID % 1_000_000_000_000)
}

func (s *Service) Run(ctx context.Context) *Report {
	report := &Report{StartedAt: s.now()}
	if !s.running.CompareAndSwap(false, true) {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Execution Lock",
			Status: StepFail,
			Detail: "another synthetic healthcheck run is already in progress",
		})
		report.CompletedAt = s.now()
		report.Duration = report.CompletedAt.Sub(report.StartedAt)
		return report
	}
	defer s.running.Store(false)
	defer func() {
		report.CompletedAt = s.now()
		report.Duration = report.CompletedAt.Sub(report.StartedAt)
	}()

	if !s.runAnalyzerStep(ctx, report) {
		return report
	}

	customer, ok := s.ensureSyntheticCustomer(ctx, report)
	if !ok {
		return report
	}

	if !s.cleanupExistingKeys(ctx, report, customer, "Preflight Cleanup", StepFail) {
		return report
	}

	_, purchaseID, err := s.payments.CreatePurchase(ctx, 0, s.canaryDays, s.canaryTrafficGB, customer, database.InvoiceTypeWalletPayment, "")
	if err != nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Workflow Canary",
			Status: StepFail,
			Detail: fmt.Sprintf("synthetic purchase failed: %v", err),
		})
		return report
	}

	key, ok := s.findActiveKey(ctx, customer.ID)
	if !ok {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Workflow Canary",
			Status: StepFail,
			Detail: fmt.Sprintf("purchase #%d completed without an active subscription key", purchaseID),
		})
		return report
	}

	report.Steps = append(report.Steps, StepResult{
		Name:   "Workflow Canary",
		Status: StepPass,
		Detail: fmt.Sprintf("purchase #%d fulfilled and created key #%d", purchaseID, key.ID),
	})

	if !s.cleanupKey(ctx, report, customer, key) {
		return report
	}

	report.Success = true
	return report
}

func (s *Service) runAnalyzerStep(ctx context.Context, report *Report) bool {
	if s.analyzer == nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Analyzer Readiness",
			Status: StepSkip,
			Detail: "screenshot verification is disabled in this runtime",
		})
		return true
	}

	readiness := s.analyzer.Readiness(ctx)
	if readiness.Status != "ok" {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Analyzer Readiness",
			Status: StepFail,
			Detail: fmt.Sprintf("vision providers degraded: %s", formatProviderReadiness(readiness.Providers)),
		})
		return false
	}

	report.Steps = append(report.Steps, StepResult{
		Name:   "Analyzer Readiness",
		Status: StepPass,
		Detail: formatProviderReadiness(readiness.Providers),
	})
	return true
}

func formatProviderReadiness(providers map[string]string) string {
	if len(providers) == 0 {
		return "no configured providers reported"
	}

	parts := make([]string, 0, len(providers))
	for name, status := range providers {
		parts = append(parts, fmt.Sprintf("%s=%s", name, status))
	}
	return strings.Join(parts, ", ")
}

func (s *Service) ensureSyntheticCustomer(ctx context.Context, report *Report) (*database.Customer, bool) {
	customer, err := s.customers.FindByTelegramId(ctx, s.syntheticTelegramID)
	if err != nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Synthetic Customer",
			Status: StepFail,
			Detail: fmt.Sprintf("customer lookup failed: %v", err),
		})
		return nil, false
	}
	if customer == nil {
		customer, err = s.customers.Create(ctx, &database.Customer{
			TelegramID: s.syntheticTelegramID,
			Language:   "en",
		})
		if err != nil {
			report.Steps = append(report.Steps, StepResult{
				Name:   "Synthetic Customer",
				Status: StepFail,
				Detail: fmt.Sprintf("customer create failed: %v", err),
			})
			return nil, false
		}
		report.Steps = append(report.Steps, StepResult{
			Name:   "Synthetic Customer",
			Status: StepPass,
			Detail: fmt.Sprintf("created synthetic customer #%d", customer.ID),
		})
		return customer, true
	}

	report.Steps = append(report.Steps, StepResult{
		Name:   "Synthetic Customer",
		Status: StepPass,
		Detail: fmt.Sprintf("reused synthetic customer #%d", customer.ID),
	})
	return customer, true
}

func (s *Service) cleanupExistingKeys(ctx context.Context, report *Report, customer *database.Customer, stepName string, failureStatus StepStatus) bool {
	keys, err := s.subscriptionKeys.FindByCustomerID(ctx, customer.ID)
	if err != nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   stepName,
			Status: failureStatus,
			Detail: fmt.Sprintf("failed to list existing canary keys: %v", err),
		})
		return false
	}

	activeKeys := make([]database.SubscriptionKey, 0, len(keys))
	for _, key := range keys {
		if key.Status == "active" {
			activeKeys = append(activeKeys, key)
		}
	}
	if len(activeKeys) == 0 {
		report.Steps = append(report.Steps, StepResult{
			Name:   stepName,
			Status: StepPass,
			Detail: "no active synthetic keys needed cleanup",
		})
		return true
	}

	for _, key := range activeKeys {
		if err := s.remnawaveUsers.DeleteUser(ctx, key.RemnawaveUUID); err != nil {
			report.Steps = append(report.Steps, StepResult{
				Name:   stepName,
				Status: failureStatus,
				Detail: fmt.Sprintf("failed to delete stale Remnawave user for key #%d: %v", key.ID, err),
			})
			return false
		}
		if err := s.subscriptionKeys.UpdateStatus(ctx, key.ID, "deleted"); err != nil {
			report.Steps = append(report.Steps, StepResult{
				Name:   stepName,
				Status: failureStatus,
				Detail: fmt.Sprintf("failed to mark stale key #%d deleted: %v", key.ID, err),
			})
			return false
		}
	}

	if err := s.resetCustomerState(ctx, customer.ID); err != nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   stepName,
			Status: failureStatus,
			Detail: fmt.Sprintf("failed to reset synthetic customer state: %v", err),
		})
		return false
	}

	report.Steps = append(report.Steps, StepResult{
		Name:   stepName,
		Status: StepPass,
		Detail: fmt.Sprintf("cleaned %d stale synthetic key(s)", len(activeKeys)),
	})
	return true
}

func (s *Service) findActiveKey(ctx context.Context, customerID int64) (*database.SubscriptionKey, bool) {
	keys, err := s.subscriptionKeys.FindByCustomerID(ctx, customerID)
	if err != nil {
		return nil, false
	}

	for i := range keys {
		if keys[i].Status == "active" {
			return &keys[i], true
		}
	}
	return nil, false
}

func (s *Service) cleanupKey(ctx context.Context, report *Report, customer *database.Customer, key *database.SubscriptionKey) bool {
	if err := s.remnawaveUsers.DeleteUser(ctx, key.RemnawaveUUID); err != nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Cleanup",
			Status: StepWarn,
			Detail: fmt.Sprintf("cleanup failed deleting Remnawave user for key #%d: %v", key.ID, err),
		})
		return false
	}
	if err := s.subscriptionKeys.UpdateStatus(ctx, key.ID, "deleted"); err != nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Cleanup",
			Status: StepWarn,
			Detail: fmt.Sprintf("cleanup failed marking key #%d deleted: %v", key.ID, err),
		})
		return false
	}
	if err := s.resetCustomerState(ctx, customer.ID); err != nil {
		report.Steps = append(report.Steps, StepResult{
			Name:   "Cleanup",
			Status: StepWarn,
			Detail: fmt.Sprintf("cleanup failed resetting synthetic customer state: %v", err),
		})
		return false
	}

	report.Steps = append(report.Steps, StepResult{
		Name:   "Cleanup",
		Status: StepPass,
		Detail: fmt.Sprintf("deleted Remnawave user and retired key #%d", key.ID),
	})
	return true
}

func (s *Service) resetCustomerState(ctx context.Context, customerID int64) error {
	return s.customers.UpdateFields(ctx, customerID, map[string]interface{}{
		"subscription_link": nil,
		"expire_at":         nil,
	})
}
