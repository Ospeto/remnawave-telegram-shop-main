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

<b>Settings</b>
/setreferralbonus &lt;amount&gt; — Change the referral bonus amount (e.g. /setreferralbonus 2000)
/setphone &lt;provider&gt; &lt;number&gt; — Set phone for a provider (e.g. /setphone kpay 09123456789)
/disablephone &lt;provider&gt; — Disable a provider (e.g. /disablephone aya)
/phones — Show all configured payment phones

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

// SetPhoneCommandHandler handles /setphone <provider> <number>
// Providers: kpay, wave (or wavepay), aya (or ayapay)
func (h Handler) SetPhoneCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	args := strings.Fields(update.Message.Text)
	if len(args) != 3 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /setphone <provider> <number>\nProviders: kpay, wave, aya\nExample: /setphone kpay 09123456789",
		})
		return
	}

	provider := strings.ToLower(args[1])
	phone := args[2]

	var dbKey string
	var label string
	switch provider {
	case "kpay", "kbzpay", "kbz":
		dbKey = "phone_kpay"
		label = "KPay"
	case "wave", "wavepay":
		dbKey = "phone_wavepay"
		label = "WavePay"
	case "aya", "ayapay":
		dbKey = "phone_ayapay"
		label = "AYA Pay"
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Unknown provider. Use: kpay, wave, or aya",
		})
		return
	}

	// Save to database
	if err := h.appConfigRepository.Set(ctx, dbKey, phone); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error saving to database: %v", err),
		})
		return
	}

	// Update in-memory
	switch dbKey {
	case "phone_kpay":
		payment.PhoneKPay = phone
	case "phone_wavepay":
		payment.PhoneWavePay = phone
	case "phone_ayapay":
		payment.PhoneAyaPay = phone
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ %s phone updated to <code>%s</code>", label, phone),
		ParseMode: models.ParseModeHTML,
	})
}

// PhonesCommandHandler handles /phones — shows all configured payment phones.
func (h Handler) PhonesCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	format := func(label, phone string) string {
		if phone == "" {
			return fmt.Sprintf("❌ %s: <i>disabled</i>", label)
		}
		return fmt.Sprintf("✅ %s: <code>%s</code>", label, phone)
	}

	text := fmt.Sprintf("📱 <b>Payment Phones</b>\n\n%s\n%s\n%s\n\nUse /setphone to enable, /disablephone to disable.",
		format("KPay", payment.PhoneKPay),
		format("WavePay", payment.PhoneWavePay),
		format("AYA Pay", payment.PhoneAyaPay),
	)

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

// DisablePhoneCommandHandler handles /disablephone <provider>
func (h Handler) DisablePhoneCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	args := strings.Fields(update.Message.Text)
	if len(args) != 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /disablephone <provider>\nProviders: kpay, wave, aya\nExample: /disablephone aya",
		})
		return
	}

	provider := strings.ToLower(args[1])

	var dbKey string
	var label string
	switch provider {
	case "kpay", "kbzpay", "kbz":
		dbKey = "phone_kpay"
		label = "KPay"
	case "wave", "wavepay":
		dbKey = "phone_wavepay"
		label = "WavePay"
	case "aya", "ayapay":
		dbKey = "phone_ayapay"
		label = "AYA Pay"
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Unknown provider. Use: kpay, wave, or aya",
		})
		return
	}

	// Clear in database
	if err := h.appConfigRepository.Set(ctx, dbKey, ""); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error saving to database: %v", err),
		})
		return
	}

	// Clear in-memory
	switch dbKey {
	case "phone_kpay":
		payment.PhoneKPay = ""
	case "phone_wavepay":
		payment.PhoneWavePay = ""
	case "phone_ayapay":
		payment.PhoneAyaPay = ""
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ %s has been disabled and will no longer appear in checkout.", label),
		ParseMode: models.ParseModeHTML,
	})
}
