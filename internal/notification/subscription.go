package notification

import (
	"context"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/translation"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type customerRepository interface {
	FindById(ctx context.Context, id int64) (*database.Customer, error)
}

type subscriptionKeyRepository interface {
	FindExpiringKeys(ctx context.Context, startDate, endDate time.Time) ([]database.SubscriptionKey, error)
	MarkExpirationNotified(ctx context.Context, keyID int64, notifiedAt time.Time) error
}

type SubscriptionService struct {
	subKeyRepo         subscriptionKeyRepository
	customerRepository customerRepository
	telegramBot        *bot.Bot
	tm                 *translation.Manager
	notify             func(context.Context, database.SubscriptionKey, database.Customer) error
}

func NewSubscriptionService(
	subKeyRepo subscriptionKeyRepository,
	customerRepository customerRepository,
	telegramBot *bot.Bot,
	tm *translation.Manager) *SubscriptionService {
	svc := &SubscriptionService{
		subKeyRepo:         subKeyRepo,
		customerRepository: customerRepository,
		telegramBot:        telegramBot,
		tm:                 tm,
	}
	svc.notify = svc.SendNotification
	return svc
}

func (s *SubscriptionService) ProcessSubscriptionExpiration() error {
	return s.ProcessSubscriptionExpirationWithContext(context.Background())
}

func (s *SubscriptionService) ProcessSubscriptionExpirationWithContext(ctx context.Context) error {
	keys, err := s.getExpiringSubscriptions(ctx)
	if err != nil {
		slog.Error("Failed to get expiring subscription keys", "error", err)
		return err
	}

	slog.Info(fmt.Sprintf("Found %d keys expiring soon", len(keys)))
	if len(keys) == 0 {
		return nil
	}

	notifiedCount := 0

	for _, key := range keys {
		// Only notify if NOT auto-renewing (auto-renew keys get handled by autorenew cron)
		if key.AutoRenew {
			continue
		}

		customer, err := s.customerRepository.FindById(ctx, key.CustomerID)
		if err != nil || customer == nil {
			slog.Error("Customer not found for expiring key", "key_id", key.ID, "customer_id", key.CustomerID)
			continue
		}

		send := s.notify
		if send == nil {
			send = s.SendNotification
		}

		err = send(ctx, key, *customer)
		if err != nil {
			slog.Error("Failed to send notification",
				"key_id", key.ID,
				"customer_id", customer.ID,
				"error", err)
			continue
		}

		if err := s.subKeyRepo.MarkExpirationNotified(ctx, key.ID, time.Now()); err != nil {
			slog.Error("Failed to persist expiration notification marker",
				"key_id", key.ID,
				"customer_id", customer.ID,
				"error", err)
			continue
		}

		notifiedCount++
		slog.Info("Notification sent successfully",
			"key_id", key.ID,
			"customer_id", customer.ID)
	}

	slog.Info(fmt.Sprintf("Sent notifications for %d expiring subscriptions", notifiedCount))
	return nil
}

func (s *SubscriptionService) getExpiringSubscriptions(ctx context.Context) ([]database.SubscriptionKey, error) {
	now := time.Now()

	// Notify once when the key enters the next-3-days window.
	// Keys are filtered durably by expiration_notified_at in the repository,
	// so a missed cron run can still notify later without daily spam.
	startDate := now
	endDate := now.Add(3 * 24 * time.Hour)

	return s.subKeyRepo.FindExpiringKeys(ctx, startDate, endDate)
}

func (s *SubscriptionService) SendNotification(ctx context.Context, key database.SubscriptionKey, customer database.Customer) error {
	if key.ExpireAt == nil {
		return fmt.Errorf("subscription key %d has no expiration date", key.ID)
	}

	messageText := s.notificationMessageText(key, customer)

	var replyMarkup *models.InlineKeyboardMarkup
	if config.GetMiniAppURL() != "" {
		replyMarkup = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text: s.tm.GetText(customer.Language, "renew_subscription_button"),
						WebApp: &models.WebAppInfo{
							URL: config.GetMiniAppURL() + "/plans?extend=" + fmt.Sprintf("%d", key.ID),
						},
					},
				},
			},
		}
	} else if customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
		replyMarkup = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text: s.tm.GetText(customer.Language, "renew_subscription_button"),
						URL:  *customer.SubscriptionLink,
					},
				},
			},
		}
	}

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      customer.TelegramID,
		Text:        messageText,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: replyMarkup,
	})

	return err
}

func (s *SubscriptionService) notificationMessageText(key database.SubscriptionKey, customer database.Customer) string {
	expireDate := key.ExpireAt.Format("02.01.2006")

	keyTemplate := s.tm.GetText(customer.Language, "subscription_expiring_key")
	legacyTemplate := s.tm.GetText(customer.Language, "subscription_expiring")

	if keyTemplate != "subscription_expiring_key" {
		return fmt.Sprintf(keyTemplate, key.Label, expireDate)
	}

	return fmt.Sprintf(legacyTemplate, expireDate)
}
