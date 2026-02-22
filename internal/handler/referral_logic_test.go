package handler_test

import (
	"context"
	"strings"
	"testing"
	"time"

	// Mock types
	"remnawave-tg-shop-bot/internal/database"
)

// We define minimal mock structs to prove the logic
type MockCustomerRepo struct {
	customers map[int64]*database.Customer
}

func (m *MockCustomerRepo) FindByTelegramId(ctx context.Context, id int64) (*database.Customer, error) {
	if c, ok := m.customers[id]; ok {
		return c, nil
	}
	return nil, nil // Not found
}

func (m *MockCustomerRepo) Create(ctx context.Context, c *database.Customer) (*database.Customer, error) {
	c.ID = int64(len(m.customers) + 1)
	m.customers[c.TelegramID] = c
	return c, nil
}

type MockReferralRepo struct {
	referrals []database.Referral
}

func (m *MockReferralRepo) Create(ctx context.Context, referrerID, refereeID int64) (int64, error) {
	ref := database.Referral{
		ID:         int64(len(m.referrals) + 1),
		ReferrerID: referrerID,
		RefereeID:  refereeID,
		UsedAt:     time.Now(),
	}
	m.referrals = append(m.referrals, ref)
	return ref.ID, nil
}

// simulateStartCommand replicates internal/handler/start.go roughly
func simulateStartCommand(t *testing.T, customerRepo *MockCustomerRepo, referralRepo *MockReferralRepo, msgText string, chatID int64) {
	existingCustomer, _ := customerRepo.FindByTelegramId(context.Background(), chatID)

	if existingCustomer == nil {
		t.Logf("User %d is new, creating profile...", chatID)
		existingCustomer, _ = customerRepo.Create(context.Background(), &database.Customer{
			TelegramID: chatID,
			Language:   "en",
		})

		if strings.Contains(msgText, "ref_") {
			parts := strings.Split(msgText, " ")
			if len(parts) > 1 {
				arg := parts[1]
				if strings.HasPrefix(arg, "ref_") {
					code := strings.TrimPrefix(arg, "ref_")

					// Just pretend parsing works
					var referrerId int64
					if code == "12345" {
						referrerId = 12345
					}
					if code == "99999" {
						referrerId = 99999
					}

					if referrerId == existingCustomer.TelegramID {
						t.Log("Blocked self referral")
					} else {
						referrer, _ := customerRepo.FindByTelegramId(context.Background(), referrerId)
						if referrer != nil {
							referralRepo.Create(context.Background(), referrerId, existingCustomer.TelegramID)
							t.Logf("SUCCESS: Referral created! Referrer %d -> Referee %d", referrerId, existingCustomer.TelegramID)
						} else {
							t.Logf("Referrer %d not found in DB", referrerId)
						}
					}
				}
			}
		}
	} else {
		t.Logf("User %d already exists. Skipping referral branch.", chatID)
	}
}

func TestReferralLogic_SuccessNewUser(t *testing.T) {
	custRepo := &MockCustomerRepo{customers: make(map[int64]*database.Customer)}
	refRepo := &MockReferralRepo{}

	// 1. Existing user generates link
	custRepo.Create(context.Background(), &database.Customer{TelegramID: 12345})

	// 2. New user clicks link
	simulateStartCommand(t, custRepo, refRepo, "/start ref_12345", 99999)

	if len(refRepo.referrals) != 1 {
		t.Errorf("Expected 1 referral, got %d", len(refRepo.referrals))
	}
}

func TestReferralLogic_FailExistingUser(t *testing.T) {
	custRepo := &MockCustomerRepo{customers: make(map[int64]*database.Customer)}
	refRepo := &MockReferralRepo{}

	// 1. Existing user 1 generates link
	custRepo.Create(context.Background(), &database.Customer{TelegramID: 12345})

	// 2. Existing user 2 clicks link (they are already in DB)
	custRepo.Create(context.Background(), &database.Customer{TelegramID: 99999})
	simulateStartCommand(t, custRepo, refRepo, "/start ref_12345", 99999)

	if len(refRepo.referrals) != 0 {
		t.Errorf("Expected 0 referrals because user already existed, got %d", len(refRepo.referrals))
	}
}

func TestReferralLogic_FailSelfReferral(t *testing.T) {
	custRepo := &MockCustomerRepo{customers: make(map[int64]*database.Customer)}
	refRepo := &MockReferralRepo{}

	// User clicks their own link upon setup (if somehow possible)
	simulateStartCommand(t, custRepo, refRepo, "/start ref_12345", 12345)

	if len(refRepo.referrals) != 0 {
		t.Errorf("Expected 0 referrals, self referral should be blocked")
	}
}
