package handler

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/payment"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	screenAdminHome              = "home"
	screenAdminOverview          = "overview"
	screenAdminPayments          = "payments"
	screenAdminPromos            = "promos"
	screenAdminPaymentsSetPhone  = "payments_setphone"
	screenAdminPaymentsSetName   = "payments_setname"
	screenAdminPaymentsDisable   = "payments_disablephone"
	screenAdminPaymentsClearName = "payments_disablename"
	screenAdminBackups           = "backups"
	screenAdminOperations        = "operations"
	screenAdminFallbacks         = "fallbacks"

	adminFlowTTL = 10 * time.Minute
)

const (
	adminQuickRevenue          = "💰 Revenue"
	adminQuickTransactions     = "💳 Transactions"
	adminQuickProviders        = "🏦 Provider Status"
	adminQuickSyncUsers        = "🔁 Sync Users"
	adminQuickBackupStatus     = "💾 Backup Status"
	adminQuickBackupList       = "📂 Backup List"
	adminQuickHealthcheck      = "🩺 E2E Check"
	adminQuickEnableTest       = "🧪 Enable Test Mode"
	adminQuickDisableTest      = "🛑 Disable Test Mode"
	adminQuickOpenDashboard    = "🧭 Dashboard"
	adminQuickHelp             = "📘 Help"
	adminQuickHideKeyboard     = "❌ Hide Admin Keyboard"
	adminQuickActionDashboard  = "__dashboard__"
	adminQuickActionHide       = "__hide_keyboard__"
	adminQuickInputPlaceholder = "Tap an admin action card"
)

type adminFlowKind string

const (
	adminFlowReferralBonus  adminFlowKind = "set_referral_bonus"
	adminFlowSetPhone       adminFlowKind = "set_phone"
	adminFlowSetName        adminFlowKind = "set_name"
	adminFlowBackupSchedule adminFlowKind = "backup_schedule"
	adminFlowNotify         adminFlowKind = "notify"
	adminFlowAddPromo       adminFlowKind = "add_promo"
	adminFlowDeletePromo    adminFlowKind = "delete_promo"
)

type adminFlowState struct {
	Kind             adminFlowKind
	Provider         string
	ReturnScreen     string
	DashboardChatID  int64
	DashboardMessage int
	ExpiresAt        time.Time
}

func (h Handler) AdminCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) || update.Message == nil {
		return
	}

	h.clearAdminFlow(update.Message.From.ID)

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "🛠 <b>Admin quick actions enabled</b>\nUse the card-style keyboard below for fast operations.",
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.ReplyKeyboardMarkup{
			Keyboard:              adminQuickActionKeyboard(),
			ResizeKeyboard:        true,
			IsPersistent:          true,
			InputFieldPlaceholder: adminQuickInputPlaceholder,
		},
	})
	if err != nil {
		slog.Warn("failed to send admin quick keyboard", "error", err)
	}

	h.sendAdminDashboard(ctx, b, update.Message.Chat.ID, screenAdminHome)
}

func (h Handler) AdminQuickActionHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) || update.Message == nil || update.Message.From == nil {
		return
	}

	if h.HasPendingAdminFlow(update.Message.From.ID) {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "You are in input mode. Send the value, or type cancel to exit that flow.",
		})
		return
	}

	action, ok := adminQuickActionCommand(update.Message.Text)
	if !ok {
		return
	}

	switch action {
	case adminQuickActionDashboard:
		h.sendAdminDashboard(ctx, b, update.Message.Chat.ID, screenAdminHome)
	case adminQuickActionHide:
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:      update.Message.Chat.ID,
			Text:        "Admin keyboard hidden. Send /admin to show it again.",
			ReplyMarkup: models.ReplyKeyboardRemove{RemoveKeyboard: true},
		})
		if err != nil {
			slog.Warn("failed to hide admin keyboard", "error", err)
		}
	default:
		h.runAdminCommand(ctx, b, update, action)
	}
}

func (h Handler) IsAdminQuickAction(text string) bool {
	_, ok := adminQuickActionCommand(text)
	return ok
}

func (h Handler) AdminCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) || update.CallbackQuery == nil || update.CallbackQuery.Message.Message == nil {
		return
	}

	parts := strings.Split(update.CallbackQuery.Data, ":")
	if len(parts) < 2 || parts[0] != CallbackAdmin {
		return
	}

	switch parts[1] {
	case "screen":
		screen := screenAdminHome
		if len(parts) > 2 {
			screen = strings.Join(parts[2:], ":")
		}
		h.clearAdminFlow(update.CallbackQuery.From.ID)
		h.editAdminDashboard(ctx, b, update.CallbackQuery.Message.Message.Chat.ID, update.CallbackQuery.Message.Message.ID, screen)
	case "action":
		h.handleAdminActionCallback(ctx, b, update, parts[2:])
	case "flow":
		h.handleAdminFlowCallback(ctx, b, update, parts[2:])
	case "confirm":
		h.handleAdminConfirmCallback(ctx, b, update, parts[2:])
	}
}

func (h Handler) AdminFlowInputHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) || update.Message == nil || update.Message.From == nil {
		return
	}

	flow, ok := h.getAdminFlow(update.Message.From.ID)
	if !ok {
		return
	}

	input := strings.TrimSpace(update.Message.Text)
	if input == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Please send a value or tap Cancel in the dashboard.",
		})
		return
	}

	if strings.EqualFold(input, "cancel") {
		h.clearAdminFlow(update.Message.From.ID)
		h.restoreAdminFlowScreen(ctx, b, flow)
		return
	}

	commandText, err := buildAdminInputCommand(flow, input)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ %v", err),
		})
		return
	}

	h.clearAdminFlow(update.Message.From.ID)
	h.runAdminCommand(ctx, b, update, commandText)
	h.restoreAdminFlowScreen(ctx, b, flow)
}

func buildAdminInputCommand(flow adminFlowState, input string) (string, error) {
	switch flow.Kind {
	case adminFlowReferralBonus:
		if _, err := strconv.ParseFloat(input, 64); err != nil {
			return "", fmt.Errorf("invalid amount. Send a number such as 2000")
		}
		return "/setreferralbonus " + input, nil
	case adminFlowSetPhone:
		if strings.Contains(input, " ") {
			return "", fmt.Errorf("phone numbers should not contain spaces")
		}
		return fmt.Sprintf("/setphone %s %s", flow.Provider, input), nil
	case adminFlowSetName:
		return fmt.Sprintf("/setname %s %s", flow.Provider, input), nil
	case adminFlowBackupSchedule:
		if _, err := time.Parse("15:04", input); err != nil {
			return "", fmt.Errorf("invalid time. Use HH:MM in 24-hour format")
		}
		return "/backup schedule " + input, nil
	case adminFlowNotify:
		if _, err := strconv.ParseInt(input, 10, 64); err != nil {
			return "", fmt.Errorf("invalid Telegram ID")
		}
		return "/notify " + input, nil
	case adminFlowAddPromo:
		fields := strings.Fields(input)
		if len(fields) != 4 {
			return "", fmt.Errorf("send: <code>name discount%% durationdays maxusescode</code>")
		}
		return "/addpromo " + strings.Join(fields, " "), nil
	case adminFlowDeletePromo:
		if strings.Contains(input, " ") {
			return "", fmt.Errorf("promo code must be a single token")
		}
		return "/deletepromo " + input, nil
	default:
		return "", fmt.Errorf("unknown admin flow")
	}
}

func (h Handler) HasPendingAdminFlow(userID int64) bool {
	_, ok := h.getAdminFlow(userID)
	return ok
}

func (h Handler) setAdminFlow(userID int64, flow adminFlowState) {
	h.adminFlowsMu.Lock()
	defer h.adminFlowsMu.Unlock()
	h.adminFlows[userID] = flow
}

func (h Handler) getAdminFlow(userID int64) (adminFlowState, bool) {
	h.adminFlowsMu.Lock()
	defer h.adminFlowsMu.Unlock()

	flow, ok := h.adminFlows[userID]
	if !ok {
		return adminFlowState{}, false
	}
	if time.Now().After(flow.ExpiresAt) {
		delete(h.adminFlows, userID)
		return adminFlowState{}, false
	}
	return flow, true
}

func (h Handler) clearAdminFlow(userID int64) {
	h.adminFlowsMu.Lock()
	defer h.adminFlowsMu.Unlock()
	delete(h.adminFlows, userID)
}

func (h Handler) sendAdminDashboard(ctx context.Context, b *bot.Bot, chatID int64, screen string) {
	text, markup := h.renderAdminScreen(ctx, screen)
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: markup,
		},
	})
	if err != nil {
		slog.Error("failed to send admin dashboard", "error", err, "screen", screen)
	}
}

func (h Handler) editAdminDashboard(ctx context.Context, b *bot.Bot, chatID int64, messageID int, screen string) {
	text, markup := h.renderAdminScreen(ctx, screen)
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: markup,
		},
	})
	if err != nil {
		slog.Warn("failed to edit admin dashboard", "error", err, "screen", screen)
	}
}

func (h Handler) renderAdminScreen(ctx context.Context, screen string) (string, [][]models.InlineKeyboardButton) {
	header := h.adminDashboardHeader(ctx)
	switch screen {
	case screenAdminOverview:
		return header + "\n\n<b>Overview</b>\nFast read-only checks for shop health.", [][]models.InlineKeyboardButton{
			{adminButton("Revenue", "action", "revenue"), adminButton("Transactions", "action", "transactions")},
			{adminButton("Providers", "action", "phones"), adminButton("Backup Status", "action", "backup", "status")},
			{adminButton("Sync Now", "action", "sync"), adminButton("Help", "action", "help")},
			{adminButton("Home", "screen", screenAdminHome), adminButton("Refresh", "screen", screenAdminOverview)},
		}
	case screenAdminPayments:
		return header + "\n\n<b>Payments</b>\nManage provider visibility and payment-facing settings.", [][]models.InlineKeyboardButton{
			{adminButton("View Providers", "action", "phones"), adminButton("Referral Bonus", "flow", string(adminFlowReferralBonus))},
			{adminButton("Set Phone", "screen", screenAdminPaymentsSetPhone), adminButton("Set Name", "screen", screenAdminPaymentsSetName)},
			{adminButton("Disable Provider", "screen", screenAdminPaymentsDisable), adminButton("Clear Name", "screen", screenAdminPaymentsClearName)},
			{adminButton("Home", "screen", screenAdminHome), adminButton("Refresh", "screen", screenAdminPayments)},
		}
	case screenAdminPromos:
		return header + "\n\n<b>Promos</b>\nCreate, inspect, and remove promo codes without memorizing fallback syntax.", [][]models.InlineKeyboardButton{
			{adminButton("List Promos", "action", "listpromos"), adminButton("Create Promo", "flow", string(adminFlowAddPromo))},
			{adminButton("Delete Promo", "flow", string(adminFlowDeletePromo)), adminButton("Promo Format", "action", "promos", "guide")},
			{adminButton("Home", "screen", screenAdminHome), adminButton("Refresh", "screen", screenAdminPromos)},
		}
	case screenAdminPaymentsSetPhone:
		return header + "\n\n<b>Set Phone</b>\nChoose a provider, then send the new phone number as your next message.", providerSelectionKeyboard("flow", string(adminFlowSetPhone), screenAdminPayments)
	case screenAdminPaymentsSetName:
		return header + "\n\n<b>Set Name</b>\nChoose a provider, then send the new account name as your next message.", providerSelectionKeyboard("flow", string(adminFlowSetName), screenAdminPayments)
	case screenAdminPaymentsDisable:
		return header + "\n\n<b>Disable Provider</b>\nChoose the provider you want to hide from checkout.", providerSelectionKeyboard("confirm", "disablephone", screenAdminPayments)
	case screenAdminPaymentsClearName:
		return header + "\n\n<b>Clear Name</b>\nChoose the provider whose account name should be cleared.", providerSelectionKeyboard("confirm", "disablename", screenAdminPayments)
	case screenAdminBackups:
		return header + "\n\n<b>Backups</b>\nRun backups, inspect schedule health, and keep restore guidance close at hand.", [][]models.InlineKeyboardButton{
			{adminButton("Status", "action", "backup", "status"), adminButton("List", "action", "backup", "list")},
			{adminButton("Run Backup Now", "confirm", "backup", "now"), adminButton("Set Schedule", "flow", string(adminFlowBackupSchedule))},
			{adminButton("Enable Schedule", "confirm", "backup", "enable"), adminButton("Disable Schedule", "confirm", "backup", "disable")},
			{adminButton("Restore List", "action", "restore", "list"), adminButton("Restore Guidance", "action", "restore", "guidance")},
			{adminButton("Home", "screen", screenAdminHome), adminButton("Refresh", "screen", screenAdminBackups)},
		}
	case screenAdminOperations:
		testLabel := "Enable Test Mode"
		if h.paymentService != nil && h.paymentService.IsTestMode() {
			testLabel = "Disable Test Mode"
		}
		testAction := "enable"
		if h.paymentService != nil && h.paymentService.IsTestMode() {
			testAction = "disable"
		}
		return header + "\n\n<b>Operations</b>\nRun admin tasks that affect runtime behavior.", [][]models.InlineKeyboardButton{
			{adminButton("Sync Now", "action", "sync"), adminButton("Notify User", "flow", string(adminFlowNotify))},
			{adminButton("Run E2E Check", "action", "healthcheck"), adminButton(testLabel, "confirm", "test", testAction)},
			{adminButton("Help", "action", "help")},
			{adminButton("Fallback Commands", "screen", screenAdminFallbacks)},
			{adminButton("Home", "screen", screenAdminHome), adminButton("Refresh", "screen", screenAdminOperations)},
		}
	case screenAdminFallbacks:
		return renderAdminHelpText(), [][]models.InlineKeyboardButton{
			{adminButton("Operations", "screen", screenAdminOperations), adminButton("Home", "screen", screenAdminHome)},
		}
	default:
		return header + "\n\n<b>Dashboard</b>\nChoose a section to operate the bot.", [][]models.InlineKeyboardButton{
			{adminButton("Overview", "screen", screenAdminOverview), adminButton("Payments", "screen", screenAdminPayments)},
			{adminButton("Promos", "screen", screenAdminPromos), adminButton("Backups", "screen", screenAdminBackups)},
			{adminButton("Operations", "screen", screenAdminOperations), adminButton("Help", "action", "help")},
			{adminButton("Refresh", "screen", screenAdminHome)},
		}
	}
}

func providerSelectionKeyboard(mode string, action string, backScreen string) [][]models.InlineKeyboardButton {
	return [][]models.InlineKeyboardButton{
		{adminButton("KPay", mode, action, "kpay"), adminButton("WavePay", mode, action, "wavepay")},
		{adminButton("AYA Pay", mode, action, "ayapay")},
		{adminButton("Back", "screen", backScreen), adminButton("Home", "screen", screenAdminHome)},
	}
}

func adminQuickActionKeyboard() [][]models.KeyboardButton {
	return [][]models.KeyboardButton{
		{{Text: adminQuickOpenDashboard}, {Text: adminQuickHealthcheck}},
		{{Text: adminQuickRevenue}, {Text: adminQuickTransactions}},
		{{Text: adminQuickProviders}, {Text: adminQuickSyncUsers}},
		{{Text: adminQuickBackupStatus}, {Text: adminQuickBackupList}},
		{{Text: adminQuickEnableTest}, {Text: adminQuickDisableTest}},
		{{Text: adminQuickHelp}},
		{{Text: adminQuickHideKeyboard}},
	}
}

func adminQuickActionCommand(text string) (string, bool) {
	switch strings.TrimSpace(text) {
	case adminQuickRevenue:
		return "/revenue", true
	case adminQuickTransactions:
		return "/transactions", true
	case adminQuickProviders:
		return "/phones", true
	case adminQuickSyncUsers:
		return "/sync", true
	case adminQuickBackupStatus:
		return "/backup status", true
	case adminQuickBackupList:
		return "/backup list", true
	case adminQuickHealthcheck:
		return "/healthbot run", true
	case adminQuickEnableTest:
		return "/test enable", true
	case adminQuickDisableTest:
		return "/test disable", true
	case adminQuickOpenDashboard:
		return adminQuickActionDashboard, true
	case adminQuickHelp:
		return "/help", true
	case adminQuickHideKeyboard:
		return adminQuickActionHide, true
	default:
		return "", false
	}
}

func (h Handler) adminDashboardHeader(ctx context.Context) string {
	testMode := "OFF"
	if h.paymentService != nil && h.paymentService.IsTestMode() {
		testMode = "ON"
	}

	providerCount := 0
	for _, phone := range []string{payment.PhoneKPay, payment.PhoneWavePay, payment.PhoneAyaPay} {
		if strings.TrimSpace(phone) != "" {
			providerCount++
		}
	}

	backupSummary := "<i>unavailable</i>"
	if backupService != nil {
		status, err := backupService.Status(ctx)
		if err == nil {
			enabled := "OFF"
			if status.Enabled {
				enabled = "ON"
			}
			backupSummary = fmt.Sprintf("%s at <code>%s</code> (%s)", enabled, html.EscapeString(status.ScheduleTime), html.EscapeString(status.Timezone))
		}
	}

	return fmt.Sprintf(
		"🛠 <b>Admin Dashboard</b>\nTest mode: <b>%s</b>\nProviders: <b>%d/3 enabled</b>\nBackups: %s",
		testMode,
		providerCount,
		backupSummary,
	)
}

func (h Handler) handleAdminActionCallback(ctx context.Context, b *bot.Bot, update *models.Update, parts []string) {
	action := strings.Join(parts, ":")
	switch action {
	case "revenue":
		h.runAdminCommand(ctx, b, update, "/revenue")
	case "transactions":
		h.runAdminCommand(ctx, b, update, "/transactions")
	case "phones":
		h.runAdminCommand(ctx, b, update, "/phones")
	case "backup:status":
		h.runAdminCommand(ctx, b, update, "/backup status")
	case "backup:list":
		h.runAdminCommand(ctx, b, update, "/backup list")
	case "listpromos":
		h.runAdminCommand(ctx, b, update, "/listpromos")
	case "promos:guide":
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    update.CallbackQuery.Message.Message.Chat.ID,
			Text:      "Create promo format:\n<code>name discount% durationdays maxusescode</code>\nExample:\n<code>sale50 50% 10days 100code</code>\n\nDelete promo format:\n<code>PROMOCODE</code>",
			ParseMode: models.ParseModeHTML,
		})
	case "restore:list":
		h.runAdminCommand(ctx, b, update, "/restore list")
	case "restore:guidance":
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.Message.Message.Chat.ID,
			Text:   runtimeRestoreGuidance,
		})
	case "sync":
		h.runAdminCommand(ctx, b, update, "/sync")
	case "healthcheck":
		h.runAdminCommand(ctx, b, update, "/healthbot run")
	case "help":
		h.runAdminCommand(ctx, b, update, "/help")
	}
}

func (h Handler) handleAdminFlowCallback(ctx context.Context, b *bot.Bot, update *models.Update, parts []string) {
	if len(parts) == 0 {
		return
	}

	flowKind := adminFlowKind(parts[0])
	state := adminFlowState{
		Kind:             flowKind,
		ReturnScreen:     screenAdminHome,
		DashboardChatID:  update.CallbackQuery.Message.Message.Chat.ID,
		DashboardMessage: update.CallbackQuery.Message.Message.ID,
		ExpiresAt:        time.Now().Add(adminFlowTTL),
	}

	title := "Send the requested value as your next message."
	switch flowKind {
	case adminFlowReferralBonus:
		state.ReturnScreen = screenAdminPayments
		title = "Send the new referral bonus amount.\nExample: <code>2000</code>"
	case adminFlowBackupSchedule:
		state.ReturnScreen = screenAdminBackups
		title = "Send the new daily backup time in <code>HH:MM</code> format.\nExample: <code>00:10</code>"
	case adminFlowNotify:
		state.ReturnScreen = screenAdminOperations
		title = "Send the target Telegram ID.\nExample: <code>123456789</code>"
	case adminFlowAddPromo:
		state.ReturnScreen = screenAdminPromos
		title = "Send the promo in this format:\n<code>name discount% durationdays maxusescode</code>\nExample:\n<code>sale50 50% 10days 100code</code>"
	case adminFlowDeletePromo:
		state.ReturnScreen = screenAdminPromos
		title = "Send the promo code to delete.\nExample: <code>SALE50</code>"
	case adminFlowSetPhone, adminFlowSetName:
		if len(parts) < 2 {
			return
		}
		state.Provider = parts[1]
		if flowKind == adminFlowSetPhone {
			state.ReturnScreen = screenAdminPayments
			title = fmt.Sprintf("Send the new %s phone number as your next message.", adminProviderLabel(parts[1]))
		} else {
			state.ReturnScreen = screenAdminPayments
			title = fmt.Sprintf("Send the new %s account name as your next message.", adminProviderLabel(parts[1]))
		}
	default:
		return
	}

	h.setAdminFlow(update.CallbackQuery.From.ID, state)
	h.editAdminPrompt(ctx, b, state.DashboardChatID, state.DashboardMessage, "Awaiting Input", title, state.ReturnScreen)
}

func (h Handler) handleAdminConfirmCallback(ctx context.Context, b *bot.Bot, update *models.Update, parts []string) {
	if len(parts) == 0 {
		return
	}

	chatID := update.CallbackQuery.Message.Message.Chat.ID
	messageID := update.CallbackQuery.Message.Message.ID

	switch parts[0] {
	case "backup":
		if len(parts) < 2 {
			return
		}
		switch parts[1] {
		case "now":
			h.editAdminConfirmation(ctx, b, chatID, messageID, "Run backup now?", "A new backup will be created immediately and uploaded to the admin chat.", adminCallback("confirm", "run", "/backup now", screenAdminBackups), screenAdminBackups)
		case "enable":
			h.editAdminConfirmation(ctx, b, chatID, messageID, "Enable scheduled backups?", "This will turn the daily backup scheduler back on.", adminCallback("confirm", "run", "/backup enable", screenAdminBackups), screenAdminBackups)
		case "disable":
			h.editAdminConfirmation(ctx, b, chatID, messageID, "Disable scheduled backups?", "This will stop the daily backup scheduler until you enable it again.", adminCallback("confirm", "run", "/backup disable", screenAdminBackups), screenAdminBackups)
		}
	case "test":
		if len(parts) < 2 {
			return
		}
		action := parts[1]
		title := "Enable test mode?"
		body := "Admin-account screenshot uploads will auto-approve while test mode is on. Shadow verification still runs and is recorded."
		if action == "disable" {
			title = "Disable test mode?"
			body = "The bot will return to normal payment verification."
		}
		h.editAdminConfirmation(ctx, b, chatID, messageID, title, body, adminCallback("confirm", "run", "/test "+action, screenAdminOperations), screenAdminOperations)
	case "disablephone":
		if len(parts) < 2 {
			return
		}
		provider := parts[1]
		body := fmt.Sprintf("%s will no longer appear in checkout until you set a phone again.", adminProviderLabel(provider))
		h.editAdminConfirmation(ctx, b, chatID, messageID, "Disable provider?", body, adminCallback("confirm", "run", "/disablephone "+provider, screenAdminPayments), screenAdminPayments)
	case "disablename":
		if len(parts) < 2 {
			return
		}
		provider := parts[1]
		body := fmt.Sprintf("%s account name will be cleared from customer instructions.", adminProviderLabel(provider))
		h.editAdminConfirmation(ctx, b, chatID, messageID, "Clear account name?", body, adminCallback("confirm", "run", "/disablename "+provider, screenAdminPayments), screenAdminPayments)
	case "run":
		if len(parts) < 3 {
			return
		}
		commandText := parts[1]
		returnScreen := parts[2]
		h.runAdminCommand(ctx, b, update, commandText)
		h.editAdminDashboard(ctx, b, chatID, messageID, returnScreen)
	}
}

func (h Handler) editAdminPrompt(ctx context.Context, b *bot.Bot, chatID int64, messageID int, title string, body string, cancelScreen string) {
	text := fmt.Sprintf("🧭 <b>%s</b>\n\n%s\n\nType <code>cancel</code> or use the button below to leave this flow.", title, body)
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{adminButton("Cancel", "screen", cancelScreen), adminButton("Home", "screen", screenAdminHome)},
			},
		},
	})
	if err != nil {
		slog.Warn("failed to edit admin prompt", "error", err)
	}
}

func (h Handler) editAdminConfirmation(ctx context.Context, b *bot.Bot, chatID int64, messageID int, title string, body string, confirmData string, cancelScreen string) {
	text := fmt.Sprintf("⚠️ <b>%s</b>\n\n%s", html.EscapeString(title), body)
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "Confirm", CallbackData: confirmData}, adminButton("Cancel", "screen", cancelScreen)},
				{adminButton("Home", "screen", screenAdminHome)},
			},
		},
	})
	if err != nil {
		slog.Warn("failed to edit admin confirmation", "error", err)
	}
}

func (h Handler) restoreAdminFlowScreen(ctx context.Context, b *bot.Bot, flow adminFlowState) {
	if flow.DashboardChatID == 0 || flow.DashboardMessage == 0 {
		return
	}
	h.editAdminDashboard(ctx, b, flow.DashboardChatID, flow.DashboardMessage, flow.ReturnScreen)
}

func (h Handler) runAdminCommand(ctx context.Context, b *bot.Bot, update *models.Update, commandText string) {
	synthetic := syntheticAdminUpdate(update, commandText)
	command := strings.Fields(commandText)
	if len(command) == 0 {
		return
	}

	switch command[0] {
	case "/help":
		h.HelpCommandHandler(ctx, b, synthetic)
	case "/sync":
		h.SyncUsersCommandHandler(ctx, b, synthetic)
	case "/revenue":
		h.RevenueCommandHandler(ctx, b, synthetic)
	case "/transactions":
		h.TransactionsCommandHandler(ctx, b, synthetic)
	case "/phones":
		h.PhonesCommandHandler(ctx, b, synthetic)
	case "/addpromo":
		h.AddPromoCommandHandler(ctx, b, synthetic)
	case "/listpromos":
		h.ListPromosCommandHandler(ctx, b, synthetic)
	case "/deletepromo":
		h.DeletePromoCommandHandler(ctx, b, synthetic)
	case "/setreferralbonus":
		h.SetReferralBonusCommandHandler(ctx, b, synthetic)
	case "/setphone":
		h.SetPhoneCommandHandler(ctx, b, synthetic)
	case "/setname":
		h.SetNameCommandHandler(ctx, b, synthetic)
	case "/disablephone":
		h.DisablePhoneCommandHandler(ctx, b, synthetic)
	case "/disablename":
		h.DisableNameCommandHandler(ctx, b, synthetic)
	case "/backup":
		h.BackupCommandHandler(ctx, b, synthetic)
	case "/restore":
		h.RestoreCommandHandler(ctx, b, synthetic)
	case "/test":
		h.TestCommandHandler(ctx, b, synthetic)
	case "/healthbot":
		h.HealthcheckCommandHandler(ctx, b, synthetic)
	case "/notify", "/noti":
		h.NotiCommandHandler(ctx, b, synthetic)
	}
}

func syntheticAdminUpdate(update *models.Update, commandText string) *models.Update {
	userID := updateUserID(update)
	chatID := updateChatID(update)
	lang := updateLanguageCode(update)

	return &models.Update{
		Message: &models.Message{
			From: &models.User{
				ID:           userID,
				LanguageCode: lang,
			},
			Chat: models.Chat{
				ID: chatID,
			},
			Text: commandText,
		},
	}
}

func adminProviderLabel(provider string) string {
	cfg, ok := resolveProviderConfig(provider)
	if !ok {
		return provider
	}
	return cfg.label
}

func adminButton(text string, parts ...string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{
		Text:         text,
		CallbackData: adminCallback(parts...),
	}
}

func adminCallback(parts ...string) string {
	return strings.Join(append([]string{CallbackAdmin}, parts...), ":")
}
