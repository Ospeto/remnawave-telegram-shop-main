package handler

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

type botCommandDef struct {
	Command string
	English string
	Burmese string
}

type fallbackCommandDef struct {
	Usage       string
	Description string
}

// The bot publishes exactly two description sets: Burmese, which is the product
// default, and English, which is genuinely supported. Russian used to carry a
// third set that no longer described a language the product speaks.
var publicBotCommandDefs = []botCommandDef{
	{Command: "start", English: "Open the main menu", Burmese: "ပင်မမီနူး ဖွင့်ရန်"},
	{Command: "connect", English: "Open connection options", Burmese: "ချိတ်ဆက်နည်း ကြည့်ရန်"},
	{Command: "help", English: "Show help", Burmese: "အကူအညီ ကြည့်ရန်"},
}

var adminBotCommandDefs = []botCommandDef{
	{Command: "admin", English: "Open admin dashboard", Burmese: "Admin dashboard ဖွင့်ရန်"},
	{Command: "healthbot", English: "Run bot E2E healthcheck", Burmese: "Bot E2E healthcheck လုပ်ရန်"},
	{Command: "help", English: "Show admin help", Burmese: "Admin အကူအညီ ကြည့်ရန်"},
}

var adminFallbackCommandDefs = []fallbackCommandDef{
	{Usage: "Core", Description: "Primary admin entrypoints"},
	{Usage: "/admin", Description: "Open the operator dashboard"},
	{Usage: "/healthbot run", Description: "Run a synthetic end-to-end bot healthcheck"},
	{Usage: "/help", Description: "Show grouped admin help"},
	{Usage: "Reporting", Description: "Read-only business and provider summaries"},
	{Usage: "/revenue", Description: "Show revenue summary"},
	{Usage: "/transactions [N]", Description: "Show the latest paid purchases"},
	{Usage: "/phones", Description: "Show provider/payment receiver status"},
	{Usage: "Promos", Description: "Promo code management fallbacks"},
	{Usage: "/addpromo <code> <discount%> <durationdays> <maxusescode>", Description: "Create a promo code"},
	{Usage: "/listpromos", Description: "List all promo codes"},
	{Usage: "/deletepromo <code>", Description: "Delete a promo code"},
	{Usage: "Payments", Description: "Provider settings and referral controls"},
	{Usage: "/setreferralbonus <amount>", Description: "Update the referral bonus"},
	{Usage: "/setphone <provider> <number>", Description: "Update provider phone number"},
	{Usage: "/setname <provider> <name>", Description: "Update provider account name"},
	{Usage: "/disablephone <provider>", Description: "Hide a provider from checkout"},
	{Usage: "/disablename <provider>", Description: "Clear a provider account name"},
	{Usage: "Operations", Description: "Runtime tasks and user nudges"},
	{Usage: "/sync", Description: "Run a user sync immediately"},
	{Usage: "/notify <telegram_id>", Description: "Send a subscription notification to one user"},
	{Usage: "/test enable|disable", Description: "Toggle payment test mode"},
	{Usage: "/trialreset", Description: "Reset admin customer trial status for end-to-end testing"},
	{Usage: "Backup", Description: "Backup and restore fallbacks"},
	{Usage: "/backup now|status|list|enable|disable|schedule HH:MM", Description: "Backup controls when you need to operate without the dashboard"},
	{Usage: "/restore list", Description: "Show local backup files for offline/manual restore"},
}

var adminDashboardSections = []string{
	"Overview: revenue, transactions, provider status, backup health",
	"Payments: update provider details, disable providers, set referral bonus",
	"Promos: create, list, and delete promo codes",
	"Backups: run backups, inspect status, manage schedule, view restore guidance",
	"Operations: sync users, run the E2E canary, send notifications, manage test mode, view fallback commands",
}

func PublicBotCommands(lang string) []models.BotCommand {
	return botCommandsForLanguage(publicBotCommandDefs, lang)
}

func AdminBotCommands(lang string) []models.BotCommand {
	return botCommandsForLanguage(adminBotCommandDefs, lang)
}

func botCommandsForLanguage(defs []botCommandDef, lang string) []models.BotCommand {
	commands := make([]models.BotCommand, 0, len(defs))
	for _, def := range defs {
		commands = append(commands, models.BotCommand{
			Command:     def.Command,
			Description: resolveCommandDescription(def, lang),
		})
	}
	return commands
}

// resolveCommandDescription picks the Burmese or English description. Burmese is
// returned for the default and for any Burmese locale; everything else reads
// English, which is the second supported language.
func resolveCommandDescription(def botCommandDef, lang string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "my") {
		return def.Burmese
	}
	return def.English
}

func renderAdminHelpText() string {
	var sb strings.Builder
	sb.WriteString("🛠 <b>Admin Help</b>\n\n")
	sb.WriteString("Use <code>/admin</code> for day-to-day operations. The dashboard is now the primary admin surface.\n\n")
	sb.WriteString("<b>Dashboard Sections</b>\n")
	for _, section := range adminDashboardSections {
		sb.WriteString("• " + section + "\n")
	}
	sb.WriteString("\n<b>Hidden Fallback Commands</b>\n")
	for _, command := range adminFallbackCommandDefs {
		if !strings.HasPrefix(command.Usage, "/") {
			sb.WriteString(fmt.Sprintf("\n<b>%s</b>\n%s\n", command.Usage, command.Description))
			continue
		}
		sb.WriteString(fmt.Sprintf("<code>%s</code> — %s\n", command.Usage, command.Description))
	}
	return sb.String()
}

// renderUserHelpText describes this product and the ways out of it: the Mini App
// where a customer buys and reads their key, and the support account where a
// human answers.
//
// It used to be one hard-coded English sentence, so a Burmese-default customer
// was handed English help with no support path at all.
func (h Handler) renderUserHelpText(langCode string) string {
	supportURL := config.SupportURL()
	if h.translation == nil {
		return fmt.Sprintf("Use /start to open the bot, /connect for your connection options, or %s for support.", supportURL)
	}
	return fmt.Sprintf(h.translation.GetText(langCode, "user_help_text"), supportURL)
}
