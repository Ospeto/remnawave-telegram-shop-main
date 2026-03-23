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
	keys, err := s.getExpiringSubscriptions()
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

		ctx := context.Background()
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

		notifiedCount++
		slog.Info("Notification sent successfully",
			"key_id", key.ID,
			"customer_id", customer.ID)
	}

	slog.Info(fmt.Sprintf("Sent notifications for %d expiring subscriptions", notifiedCount))
	return nil
}

func (s *SubscriptionService) getExpiringSubscriptions() ([]database.SubscriptionKey, error) {
	now := time.Now()

	// Notify strictly for keys expiring exactly between 2 and 3 days from now
	// This prevents spamming the user every day (3 days, 2 days, 1 day)
	startDate := now.Add(2 * 24 * time.Hour)
	endDate := now.Add(3 * 24 * time.Hour)

	return s.subKeyRepo.FindExpiringKeys(context.Background(), startDate, endDate)
}

func (s *SubscriptionService) SendNotification(ctx context.Context, key database.SubscriptionKey, customer database.Customer) error {
	if key.ExpireAt == nil {
		return fmt.Errorf("subscription key %d has no expiration date", key.ID)
	}

	expireDate := key.ExpireAt.Format("02.01.2006")

	messageText := fmt.Sprintf(
		s.tm.GetText(customer.Language, "subscription_expiring_key"),
		key.Label,
		expireDate,
	)

	// Fallback to legacy string if specific key translation is missing
	if messageText == "subscription_expiring_key" {
		messageText = fmt.Sprintf(
			s.tm.GetText(customer.Language, "subscription_expiring"),
			expireDate,
		)
	}

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
