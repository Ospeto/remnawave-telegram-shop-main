package handler

import (
	"context"
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	appPromo "remnawave-tg-shop-bot/internal/promo"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) AddPromoCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Only admin can create promo codes
	if update.Message.From.ID != config.GetAdminTelegramId() {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Unauthorized: admin only command.",
		})
		return
	}

	params, err := appPromo.ParseCreateCommand(update.Message.Text)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   err.Error(),
		})
		return
	}

	validUntil := params.ValidUntilAt(time.Now())
	if err := h.promoCodeRepository.Create(ctx, params.Code, params.DiscountPercent, params.MaxUses, validUntil); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("Error creating promo code: %v", err),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("Promo code '%s' created!\nDiscount: %d%%\nValid until: %s\nMax uses: %d", params.Code, params.DiscountPercent, validUntil.Format("2006-01-02"), params.MaxUses),
	})
}
