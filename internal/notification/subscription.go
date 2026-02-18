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
	FindByExpirationRange(ctx context.Context, startDate, endDate time.Time) (*[]database.Customer, error)
}

type SubscriptionService struct {
	customerRepository customerRepository
	telegramBot        *bot.Bot
	tm                 *translation.Manager
	notify             func(context.Context, database.Customer) error
}

func NewSubscriptionService(customerRepository customerRepository,
	telegramBot *bot.Bot,
	tm *translation.Manager) *SubscriptionService {
	svc := &SubscriptionService{customerRepository: customerRepository, telegramBot: telegramBot, tm: tm}
	svc.notify = svc.sendNotification
	return svc
}
func (s *SubscriptionService) ProcessSubscriptionExpiration() error {
	customers, err := s.getCustomersWithExpiringSubscriptions()
	if err != nil {
		slog.Error("Failed to get customers with expiring subscriptions", "error", err)
		return err
	}

	slog.Info(fmt.Sprintf("Found %d customers with expiring subscriptions", len(*customers)))
	if len(*customers) == 0 {
		return nil
	}

	for _, customer := range *customers {
		ctx := context.Background()

		send := s.notify
		if send == nil {
			send = s.sendNotification
		}

		err := send(ctx, customer)
		if err != nil {
			slog.Error("Failed to send notification",
				"customer_id", customer.ID,
				"error", err)
			continue
		}

		slog.Info("Notification sent successfully",
			"customer_id", customer.ID)
	}

	slog.Info(fmt.Sprintf("Sent notifications to %d customers with expiring subscriptions", len(*customers)))
	return nil
}

func (s *SubscriptionService) getCustomersWithExpiringSubscriptions() (*[]database.Customer, error) {
	now := time.Now()
	endDate := now.AddDate(0, 0, 3)

	dbCustomers, err := s.customerRepository.FindByExpirationRange(context.Background(), now, endDate)
	if err != nil {
		return nil, err
	}

	return dbCustomers, nil
}

func (s *SubscriptionService) sendNotification(ctx context.Context, customer database.Customer) error {
	expireDate := customer.ExpireAt.Format("02.01.2006")

	messageText := fmt.Sprintf(
		s.tm.GetText(customer.Language, "subscription_expiring"),
		expireDate,
	)

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		Text:      messageText,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text: s.tm.GetText(customer.Language, "renew_subscription_button"),
						WebApp: &models.WebAppInfo{
							URL: config.GetMiniAppURL() + "/plans?extend=" + fmt.Sprintf("%d", customer.ID),
						},
					},
				},
			},
		},
	})

	return err
}
