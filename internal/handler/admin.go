package handler

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/service/backup"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var backupService *backup.Service

func SetBackupService(service *backup.Service) {
	backupService = service
}

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
/setname &lt;provider&gt; &lt;name&gt; — Set account name for a provider (e.g. /setname wave Aung Aung)
/disablephone &lt;provider&gt; — Disable a provider (e.g. /disablephone aya)
/disablename &lt;provider&gt; — Clear a provider account name (e.g. /disablename aya)
/phones — Show all configured payment receivers

<b>Transactions</b>
/transactions — Last 10 paid transactions
/transactions 25 — Last N paid transactions (max 50)
/revenue — Revenue summary (today + last 7 days)
/backup now — Create DB backup and send it to admin chat
/backup status — Show backup scheduler and last backup status
/backup list — Show recent local backup files
/backup enable — Enable scheduled backups
/backup disable — Disable scheduled backups
/backup schedule [HH:MM] — Show or set daily backup time
/restore list — Show restorable backup files
/restore latest — Prepare restore using latest local backup
/restore file &lt;name&gt; — Prepare restore using a named local backup
/restore confirm &lt;token&gt; — Confirm pending restore
/restore cancel — Cancel pending restore

<b>Promo Codes</b>
/addpromo &lt;code&gt; &lt;discount%&gt; &lt;Ndays&gt; &lt;Ncode&gt;
  Example: /addpromo SALE50 50% 10days 100code
/listpromos — List all promo codes
/deletepromo &lt;code&gt; — Delete a promo code

<b>Testing &amp; Notifications</b>
/test — Test mode commands
/noti — Send subscription notifications`

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      helpText,
		ParseMode: models.ParseModeHTML,
	})
}

type providerConfig struct {
	phoneKey string
	nameKey  string
	label    string
	phonePtr *string
	namePtr  *string
}

func resolveProviderConfig(provider string) (providerConfig, bool) {
	switch payment.NormalizeProviderKey(provider) {
	case "kpay":
		return providerConfig{
			phoneKey: "phone_kpay",
			nameKey:  "name_kpay",
			label:    "KPay",
			phonePtr: &payment.PhoneKPay,
			namePtr:  &payment.AccountNameKPay,
		}, true
	case "wavepay":
		return providerConfig{
			phoneKey: "phone_wavepay",
			nameKey:  "name_wavepay",
			label:    "WavePay",
			phonePtr: &payment.PhoneWavePay,
			namePtr:  &payment.AccountNameWave,
		}, true
	case "ayapay":
		return providerConfig{
			phoneKey: "phone_ayapay",
			nameKey:  "name_ayapay",
			label:    "AYA Pay",
			phonePtr: &payment.PhoneAyaPay,
			namePtr:  &payment.AccountNameAya,
		}, true
	default:
		return providerConfig{}, false
	}
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

	phone := args[2]
	cfg, ok := resolveProviderConfig(args[1])
	if !ok {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Unknown provider. Use: kpay, wave, or aya",
		})
		return
	}

	// Save to database
	if err := h.appConfigRepository.Set(ctx, cfg.phoneKey, phone); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error saving to database: %v", err),
		})
		return
	}

	// Update in-memory
	*cfg.phonePtr = phone

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ %s phone updated to <code>%s</code>", cfg.label, html.EscapeString(phone)),
		ParseMode: models.ParseModeHTML,
	})
}

// SetNameCommandHandler handles /setname <provider> <name>.
func (h Handler) SetNameCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	args := strings.SplitN(strings.TrimSpace(update.Message.Text), " ", 3)
	if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /setname <provider> <name>\nProviders: kpay, wave, aya\nExample: /setname wave Aung Aung",
		})
		return
	}

	cfg, ok := resolveProviderConfig(args[1])
	if !ok {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Unknown provider. Use: kpay, wave, or aya",
		})
		return
	}

	name := strings.TrimSpace(args[2])
	if err := h.appConfigRepository.Set(ctx, cfg.nameKey, name); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error saving to database: %v", err),
		})
		return
	}

	*cfg.namePtr = name

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ %s account name updated to <code>%s</code>", cfg.label, html.EscapeString(name)),
		ParseMode: models.ParseModeHTML,
	})
}

// PhonesCommandHandler handles /phones — shows all configured payment receivers.
func (h Handler) PhonesCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	formatValue := func(value string) string {
		if strings.TrimSpace(value) == "" {
			return "<i>not set</i>"
		}
		return fmt.Sprintf("<code>%s</code>", html.EscapeString(value))
	}

	formatProvider := func(cfg providerConfig) string {
		status := "❌ <i>disabled</i>"
		if strings.TrimSpace(*cfg.phonePtr) != "" {
			status = "✅ <i>enabled</i>"
		}
		return fmt.Sprintf("<b>%s</b> — %s\nPhone: %s\nName: %s",
			cfg.label,
			status,
			formatValue(*cfg.phonePtr),
			formatValue(*cfg.namePtr),
		)
	}

	text := fmt.Sprintf("📱 <b>Payment Receivers</b>\n\n%s\n\n%s\n\n%s\n\nUse /setphone and /setname to update. Use /disablephone to hide a provider from checkout.",
		formatProvider(providerConfig{label: "KPay", phonePtr: &payment.PhoneKPay, namePtr: &payment.AccountNameKPay}),
		formatProvider(providerConfig{label: "WavePay", phonePtr: &payment.PhoneWavePay, namePtr: &payment.AccountNameWave}),
		formatProvider(providerConfig{label: "AYA Pay", phonePtr: &payment.PhoneAyaPay, namePtr: &payment.AccountNameAya}),
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

	cfg, ok := resolveProviderConfig(args[1])
	if !ok {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Unknown provider. Use: kpay, wave, or aya",
		})
		return
	}

	// Clear in database
	if err := h.appConfigRepository.Set(ctx, cfg.phoneKey, ""); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error saving to database: %v", err),
		})
		return
	}

	// Clear in-memory
	*cfg.phonePtr = ""

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ %s has been disabled and will no longer appear in checkout.", cfg.label),
		ParseMode: models.ParseModeHTML,
	})
}

// DisableNameCommandHandler handles /disablename <provider>.
func (h Handler) DisableNameCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	args := strings.Fields(update.Message.Text)
	if len(args) != 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /disablename <provider>\nProviders: kpay, wave, aya\nExample: /disablename aya",
		})
		return
	}

	cfg, ok := resolveProviderConfig(args[1])
	if !ok {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Unknown provider. Use: kpay, wave, or aya",
		})
		return
	}

	if err := h.appConfigRepository.Set(ctx, cfg.nameKey, ""); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error saving to database: %v", err),
		})
		return
	}

	*cfg.namePtr = ""

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      fmt.Sprintf("✅ %s account name cleared.", cfg.label),
		ParseMode: models.ParseModeHTML,
	})
}

// RevenueCommandHandler handles /revenue — shows revenue summary.
func (h Handler) RevenueCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}

	rows, err := h.purchaseRepository.GetRevenueSummary(ctx, 7)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error fetching revenue: %v", err),
		})
		return
	}

	if len(rows) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "📊 No revenue data for the last 7 days.",
		})
		return
	}

	// Group by day
	today := time.Now().Format("2006-01-02")
	var todayLines []string
	var todayTotal float64
	var todayTxns int

	// map[day] => aggregated line
	type daySummary struct {
		Revenue  float64
		Txns     int
		Users    int
		Currency string
	}
	dayMap := make(map[string]*daySummary)
	var dayOrder []string

	for _, r := range rows {
		method := r.PaymentMethod
		if method == "" {
			method = "unknown"
		}
		currency := r.Currency
		if currency == "" {
			currency = "MMK"
		}

		if r.Day == today {
			todayLines = append(todayLines, fmt.Sprintf("  %s: %s %s (%d txns)",
				method, formatNumber(r.TotalRevenue), currency, r.TotalPurchases))
			todayTotal += r.TotalRevenue
			todayTxns += r.TotalPurchases
		}

		if _, ok := dayMap[r.Day]; !ok {
			dayMap[r.Day] = &daySummary{Currency: currency}
			dayOrder = append(dayOrder, r.Day)
		}
		d := dayMap[r.Day]
		d.Revenue += r.TotalRevenue
		d.Txns += r.TotalPurchases
		if r.UniqueCustomers > d.Users {
			d.Users = r.UniqueCustomers
		}
	}

	var sb strings.Builder
	sb.WriteString("📊 <b>Revenue Summary</b>\n\n")

	// Today section
	sb.WriteString("<b>Today</b>\n")
	if len(todayLines) > 0 {
		for _, l := range todayLines {
			sb.WriteString(l + "\n")
		}
		sb.WriteString(fmt.Sprintf("  <b>Total: %s (%d txns)</b>\n", formatNumber(todayTotal), todayTxns))
	} else {
		sb.WriteString("  No sales yet today\n")
	}

	// 7-day breakdown
	sb.WriteString("\n<b>Last 7 Days</b>\n")
	for _, day := range dayOrder {
		d := dayMap[day]
		label := day
		if day == today {
			label = day + " (today)"
		}
		sb.WriteString(fmt.Sprintf("  %s: %s %s (%d txns, %d users)\n",
			label, formatNumber(d.Revenue), d.Currency, d.Txns, d.Users))
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      sb.String(),
		ParseMode: models.ParseModeHTML,
	})
}

// formatNumber returns a human-readable number string with commas.
func formatNumber(n float64) string {
	if n == float64(int64(n)) {
		// Integer — no decimals
		s := strconv.FormatInt(int64(n), 10)
		return addCommas(s)
	}
	s := fmt.Sprintf("%.2f", n)
	parts := strings.SplitN(s, ".", 2)
	return addCommas(parts[0]) + "." + parts[1]
}

func addCommas(s string) string {
	neg := ""
	if len(s) > 0 && s[0] == '-' {
		neg = "-"
		s = s[1:]
	}
	if len(s) <= 3 {
		return neg + s
	}
	var result []byte
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(ch))
	}
	return neg + string(result)
}

func (h Handler) BackupCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}
	if backupService == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Backup service is not configured.",
		})
		return
	}

	args := strings.Fields(strings.TrimSpace(update.Message.Text))
	if len(args) < 2 {
		h.sendBackupStatus(ctx, b, update)
		return
	}

	switch strings.ToLower(args[1]) {
	case "now":
		result, err := backupService.CreateBackup(ctx, "manual")
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Backup failed: %v", err),
			})
			return
		}
		if err := backupService.SendBackupDocument(ctx, b, update.Message.Chat.ID, result, "Manual backup complete"); err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("⚠️ Backup created but upload failed: %v\nLocal file: %s", err, result.File.Name),
			})
			return
		}
	case "status":
		h.sendBackupStatus(ctx, b, update)
	case "list":
		backups, err := backupService.ListBackups()
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Failed to list backups: %v", err),
			})
			return
		}
		if len(backups) == 0 {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "📭 No local backups found.",
			})
			return
		}
		var sb strings.Builder
		sb.WriteString("🗃 <b>Local Backups</b>\n\n")
		for i, file := range backups {
			if i == 10 {
				break
			}
			sb.WriteString(fmt.Sprintf("%d. <code>%s</code>\n   %s | %s\n",
				i+1,
				html.EscapeString(file.Name),
				file.ModTime.Format("2006-01-02 15:04"),
				formatNumber(float64(file.Size))+" B",
			))
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      sb.String(),
			ParseMode: models.ParseModeHTML,
		})
	case "enable":
		if err := backupService.SetEnabled(ctx, true); err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Failed to enable backups: %v", err),
			})
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "✅ Scheduled backups enabled.",
		})
	case "disable":
		if err := backupService.SetEnabled(ctx, false); err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Failed to disable backups: %v", err),
			})
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "✅ Scheduled backups disabled.",
		})
	case "schedule":
		if len(args) == 2 {
			h.sendBackupStatus(ctx, b, update)
			return
		}
		if err := backupService.SetScheduleTime(ctx, args[2]); err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Invalid schedule: %v", err),
			})
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("✅ Backup schedule updated to %s (%s).", args[2], config.BackupTimezone()),
		})
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /backup now|status|list|enable|disable|schedule [HH:MM]",
		})
	}
}

func (h Handler) RestoreCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}
	if backupService == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Backup service is not configured.",
		})
		return
	}

	args := strings.Fields(strings.TrimSpace(update.Message.Text))
	if len(args) < 2 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /restore list|latest|file <name>|confirm <token>|cancel",
		})
		return
	}

	switch strings.ToLower(args[1]) {
	case "list":
		backups, err := backupService.ListBackups()
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Failed to list backups: %v", err),
			})
			return
		}
		if len(backups) == 0 {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "📭 No local backups available for restore.",
			})
			return
		}
		var sb strings.Builder
		sb.WriteString("♻️ <b>Restorable Backups</b>\n\n")
		for i, file := range backups {
			if i == 10 {
				break
			}
			sb.WriteString(fmt.Sprintf("%d. <code>%s</code>\n", i+1, html.EscapeString(file.Name)))
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      sb.String(),
			ParseMode: models.ParseModeHTML,
		})
	case "latest":
		pending, err := backupService.PrepareRestoreLatest(ctx)
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Restore preparation failed: %v", err),
			})
			return
		}
		h.sendRestoreConfirmation(ctx, b, update, pending)
	case "file":
		if len(args) < 3 {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Usage: /restore file <name>",
			})
			return
		}
		fileName := strings.TrimSpace(strings.TrimPrefix(update.Message.Text, "/restore file"))
		pending, err := backupService.PrepareRestoreFile(ctx, strings.TrimSpace(fileName))
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Restore preparation failed: %v", err),
			})
			return
		}
		h.sendRestoreConfirmation(ctx, b, update, pending)
	case "confirm":
		if len(args) != 3 {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Usage: /restore confirm <token>",
			})
			return
		}
		result, err := backupService.ConfirmRestore(ctx, args[2])
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   fmt.Sprintf("❌ Restore failed: %v", err),
			})
			return
		}
		text := fmt.Sprintf("✅ Restore complete.\nSource: <code>%s</code>", html.EscapeString(result.Target.Name))
		if result.SafetyBackup != nil {
			text += fmt.Sprintf("\nSafety backup: <code>%s</code>", html.EscapeString(result.SafetyBackup.Name))
		}
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.Message.Chat.ID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
		})
	case "cancel":
		backupService.CancelPendingRestore()
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "✅ Pending restore cancelled.",
		})
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Usage: /restore list|latest|file <name>|confirm <token>|cancel",
		})
	}
}

func (h Handler) sendBackupStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	status, err := backupService.Status(ctx)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Failed to load backup status: %v", err),
		})
		return
	}

	enabledText := "disabled"
	if status.Enabled {
		enabledText = "enabled"
	}
	sendText := "disabled"
	if status.SendToTelegram {
		sendText = "enabled"
	}
	restoreText := "disabled"
	if status.RestoreEnabled {
		restoreText = "enabled"
	}
	lastSuccess := "<i>never</i>"
	if status.LastSuccessAt != nil {
		lastSuccess = status.LastSuccessAt.Format("2006-01-02 15:04")
	}

	text := fmt.Sprintf(
		"💾 <b>Backup Status</b>\n\nEnabled: <b>%s</b>\nTelegram delivery: <b>%s</b>\nRestore: <b>%s</b>\nSchedule: <code>%s</code> (%s)\nNext run: %s\nLast success: %s\nLast file: <code>%s</code>\nBackups on disk: %d",
		enabledText,
		sendText,
		restoreText,
		status.ScheduleTime,
		html.EscapeString(status.Timezone),
		status.NextRunAt.Format("2006-01-02 15:04"),
		lastSuccess,
		html.EscapeString(status.LastFile),
		status.BackupCount,
	)
	if status.LastError != "" {
		text += fmt.Sprintf("\nLast error: <code>%s</code>", html.EscapeString(status.LastError))
	}
	if status.OperationRunning {
		text += "\n\n⚠️ A backup or restore operation is currently running."
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}

func (h Handler) sendRestoreConfirmation(ctx context.Context, b *bot.Bot, update *models.Update, pending *backup.PendingRestore) {
	text := fmt.Sprintf(
		"⚠️ This will overwrite the current database.\nA safety backup will be created first.\n\nSource: <code>%s</code>\nConfirm before: %s\nRun: <code>/restore confirm %s</code>",
		html.EscapeString(pending.File.Name),
		pending.ExpiresAt.Format("2006-01-02 15:04"),
		html.EscapeString(pending.Token),
	)
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
}
