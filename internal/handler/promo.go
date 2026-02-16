package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) AddPromoCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Format: /addpromo <code_name> <discount_percent> <duration_days> <max_uses>
	args := strings.Fields(update.Message.Text)
	if len(args) != 5 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /addpromo <code_name> <discount_percent> <duration_days> <max_uses>\nExample: /addpromo sale50 50% 10days 100code",
		})
		return
	}

	codeName := args[1]
	discountStr := strings.TrimSuffix(args[2], "%")
	durationStr := strings.TrimSuffix(args[3], "days")
	maxUsesStr := strings.TrimSuffix(args[4], "code")

	discount, err := strconv.Atoi(discountStr)
	if err != nil || discount <= 0 || discount > 100 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Invalid discount percent. Must be 1-100.",
		})
		return
	}

	durationDays, err := strconv.Atoi(durationStr)
	if err != nil || durationDays <= 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Invalid duration days.",
		})
		return
	}

	maxUses, err := strconv.Atoi(maxUsesStr)
	if err != nil || maxUses <= 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Invalid max uses.",
		})
		return
	}

	validUntil := time.Now().Add(time.Duration(durationDays) * 24 * time.Hour)

	err = h.promoCodeRepository.Create(ctx, codeName, discount, maxUses, validUntil)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("Error creating promo code: %v", err),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("Promo code '%s' created!\nDiscount: %d%%\nValid until: %s\nMax uses: %d", codeName, discount, validUntil.Format("2006-01-02"), maxUses),
	})
}
