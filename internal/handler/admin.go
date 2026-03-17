package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/payment"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// adminOnly is a helper that sends an unauthorized message and returns false if not admin.
func (h Handler) adminOnly(ctx context.Context, b *bot.Bot, update *models.Update) bool {
	if update.Message.From.ID != config.GetAdminTelegramId() {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "⛔ Unauthorized: admin only command.",
		})
		return false
	}
	return true
}

// HelpCommandHandler handles /help — lists all admin commands.
func (h Handler) HelpCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	helpText := `🛠 <b>Admin Commands</b>

<b>General</b>
/start — Show main menu
/help — Show this help message
/sync — Sync users with Remnawave
/apicheck — Check receipt AI API connectivity

<b>Settings</b>
/setreferralbonus &lt;amount&gt; — Change the referral bonus amount (e.g. /setreferralbonus 2000)

<b>Transactions</b>
/transactions — Last 10 paid transactions
/transactions 25 — Last N paid transactions (max 50)

<b>Promo Codes</b>
/addpromo &lt;code&gt; &lt;discount%&gt; &lt;Ndays&gt; &lt;Ncode&gt;
  Example: /addpromo SALE50 50% 10days 100code
/listpromos — List all promo codes
/deletepromo &lt;code&gt; — Delete a promo code`

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      helpText,
		ParseMode: models.ParseModeHTML,
	})
}

// SetReferralBonusCommandHandler handles /setreferralbonus <amount>
func (h Handler) SetReferralBonusCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	args := strings.Fields(update.Message.Text)
	if len(args) != 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /setreferralbonus <amount>\nExample: /setreferralbonus 2000",
		})
		return
	}

	amount, err := strconv.ParseFloat(args[1], 64)
	if err != nil || amount < 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Invalid amount. Must be a positive number.",
		})
		return
	}

	// Update the database
	err = h.appConfigRepository.Set(ctx, "referral_bonus_amount", fmt.Sprintf("%.0f", amount))
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error saving to database: %v", err),
		})
		return
	}

	// Update the in-memory variable
	payment.ReferralBonusAmount = amount

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ Referral bonus successfully updated to <b>%.0f MMK</b>.", amount),
		ParseMode: models.ParseModeHTML,
	})
}

// TransactionsCommandHandler handles /transactions [N] — shows recent paid purchases.
func (h Handler) TransactionsCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	limit := 10
	args := strings.Fields(update.Message.Text)
	if len(args) >= 2 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "❌ Invalid number. Usage: /transactions [N] (e.g. /transactions 20)",
			})
			return
		}
		if n > 50 {
			n = 50
		}
		limit = n
	}

	rows, err := h.purchaseRepository.FindRecentPaid(ctx, limit)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error fetching transactions: %v", err),
		})
		return
	}

	if len(rows) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "📭 No paid transactions found.",
		})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💳 <b>Last %d Paid Transactions</b>\n\n", len(rows)))

	for i, r := range rows {
		method := r.PaymentMethod
		if method == "" {
			method = "unknown"
		}
		paidAt := r.PaidAt.In(time.UTC).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf(
			"<b>%d.</b> #%d | <code>%d</code>\n"+
				"   📦 %s | 💰 %.0f %s\n"+
				"   🏦 %s | 🕐 %s\n\n",
			i+1, r.PurchaseID, r.TelegramID,
			r.PlanLabel, r.Amount, r.Currency,
			method, paidAt,
		))
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      sb.String(),
		ParseMode: models.ParseModeHTML,
	})
}

// ListPromosCommandHandler handles /listpromos — lists all promo codes.
func (h Handler) ListPromosCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	codes, err := h.promoCodeRepository.ListAll(ctx)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error fetching promo codes: %v", err),
		})
		return
	}

	if len(codes) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "📭 No promo codes found.",
		})
		return
	}

	now := time.Now()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎟 <b>Promo Codes (%d)</b>\n\n", len(codes)))

	for _, c := range codes {
		status := "✅ Active"
		if c.ValidUntil.Before(now) {
			status = "❌ Expired"
		} else if c.UsedCount >= c.MaxUses {
			status = "🚫 Exhausted"
		}
		sb.WriteString(fmt.Sprintf(
			"<code>%s</code> — %d%% off\n"+
				"   Uses: %d/%d | Expires: %s\n"+
				"   %s\n\n",
			c.Code, c.DiscountPercent,
			c.UsedCount, c.MaxUses,
			c.ValidUntil.Format("2006-01-02"),
			status,
		))
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      sb.String(),
		ParseMode: models.ParseModeHTML,
	})
}

// DeletePromoCommandHandler handles /deletepromo <code> — deletes a promo code.
func (h Handler) DeletePromoCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	args := strings.Fields(update.Message.Text)
	if len(args) != 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /deletepromo <code>\nExample: /deletepromo SALE50",
		})
		return
	}

	code := args[1]
	err := h.promoCodeRepository.Delete(ctx, code)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ %v", err),
		})
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ Promo code <code>%s</code> deleted successfully.", code),
		ParseMode: models.ParseModeHTML,
	})
}
