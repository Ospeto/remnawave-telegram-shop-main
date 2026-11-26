package handler

import (
	"context"

	"log/slog"
	"runtime/debug"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/utils"

	"golang.org/x/time/rate"
)

func (h Handler) AcknowledgeCallbackQueryMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.CallbackQuery != nil {
			_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: update.CallbackQuery.ID,
			})
			if err != nil {
				slog.Warn("failed to acknowledge callback query", "error", err)
			}
		}

		next(ctx, b, update)
	}
}

// PanicRecoveryMiddleware contains a panic raised while handling one update so a
// single failed update cannot terminate the bot process for every customer.
// The telegram/bot library has no recover of its own: a panic inside a handler
// goroutine crashes the whole process.
//
// It is a package-level function because the bot is constructed before the
// Handler instance exists, and global middlewares can only be supplied at
// construction time.
func PanicRecoveryMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("recovered panic while handling update",
					"panic", recovered,
					"stack", string(debug.Stack()),
					"update_id", updateID(update),
				)
			}
		}()

		next(ctx, b, update)
	}
}

func updateID(update *models.Update) int64 {
	if update == nil {
		return 0
	}
	return update.ID
}

func (h Handler) CreateCustomerIfNotExistMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var telegramId int64
		var langCode string
		if update.Message != nil {
			telegramId = update.Message.From.ID
			langCode = update.Message.From.LanguageCode
		} else if update.CallbackQuery != nil {
			telegramId = update.CallbackQuery.From.ID
			langCode = update.CallbackQuery.From.LanguageCode
		}
		existingCustomer, err := h.customerRepository.FindByTelegramId(ctx, telegramId)
		if err != nil {
			slog.Error("error finding customer by telegram id", "error", err)
			return
		}

		effectiveLang := resolveEffectiveLanguage(existingCustomer, langCode)
		if existingCustomer == nil {
			existingCustomer, err = h.customerRepository.Create(ctx, &database.Customer{
				TelegramID: telegramId,
				Language:   effectiveLang,
			})
			if err != nil {
				slog.Error("error creating customer", "error", err)
				return
			}
		} else {
			// Only teach the row its language when it has none yet. This used to
			// rewrite the stored choice on every single update, so `/api/me` could
			// never carry a real choice and the Mini App and the bot drifted apart.
			if languageNeedsStorage(existingCustomer) && existingCustomer.Language != effectiveLang {
				updates := map[string]interface{}{
					"language": effectiveLang,
				}

				err = h.customerRepository.UpdateFields(ctx, existingCustomer.ID, updates)
				if err != nil {
					slog.Error("Error updating customer", "error", err)
					return
				}
				existingCustomer.Language = effectiveLang
			}
		}

		next(ctx, b, update)
	}
}

func (h Handler) SuspiciousUserFilterMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var username, firstName, lastName *string
		var userID int64
		var chatID int64
		var langCode string

		if update.Message != nil {
			username = &update.Message.From.Username
			firstName = &update.Message.From.FirstName
			lastName = &update.Message.From.LastName
			userID = update.Message.From.ID
			chatID = update.Message.Chat.ID
			langCode = update.Message.From.LanguageCode
		} else if update.CallbackQuery != nil {
			username = &update.CallbackQuery.From.Username
			firstName = &update.CallbackQuery.From.FirstName
			lastName = &update.CallbackQuery.From.LastName
			userID = update.CallbackQuery.From.ID
			chatID = update.CallbackQuery.Message.Message.Chat.ID
			langCode = update.CallbackQuery.From.LanguageCode
		} else {
			next(ctx, b, update)
			return
		}

		// Rate Limit Check
		if h.limitersMu != nil && h.limiters != nil {
			h.limitersMu.Lock()
			limiter, exists := h.limiters[userID]
			if !exists {
				// Allow 2 events per second with burst of 5
				limiter = rate.NewLimiter(2, 5)
				h.limiters[userID] = limiter
			}
			h.limitersMu.Unlock()

			if !limiter.Allow() {
				slog.Warn("rate limit exceeded", "userId", utils.MaskHalfInt64(userID))
				return
			}
		}

		if config.GetBlockedTelegramIds()[userID] {
			slog.Warn("blocked user by telegram id", "userId", utils.MaskHalfInt64(userID))
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      h.translation.GetText(langCode, "access_denied"),
				ParseMode: models.ParseModeHTML,
			})
			if err != nil {
				slog.Error("error sending blocked user message", "error", err)
			}
			return
		}

		if config.GetWhitelistedTelegramIds()[userID] {
			slog.Info("whitelisted user allowed", "userId", utils.MaskHalfInt64(userID))
			next(ctx, b, update)
			return
		}

		if utils.IsSuspiciousUser(username, firstName, lastName) {
			slog.Warn("suspicious user blocked", "userId", utils.MaskHalfInt64(userID))
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      h.translation.GetText(langCode, "access_denied"),
				ParseMode: models.ParseModeHTML,
			})
			if err != nil {
				slog.Error("error sending suspicious user message", "error", err)
			}
			return
		}

		next(ctx, b, update)
	}
}
