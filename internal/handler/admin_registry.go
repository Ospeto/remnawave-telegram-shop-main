package handler

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
)

type botCommandDef struct {
	Command string
	English string
	Russian string
}

type fallbackCommandDef struct {
	Usage       string
	Description string
}

var publicBotCommandDefs = []botCommandDef{
	{Command: "start", English: "Open the main menu", Russian: "Открыть главное меню"},
	{Command: "connect", English: "Open connection options", Russian: "Показать способы подключения"},
	{Command: "help", English: "Show help", Russian: "Показать справку"},
}

var adminBotCommandDefs = []botCommandDef{
	{Command: "admin", English: "Open admin dashboard", Russian: "Открыть панель администратора"},
	{Command: "healthbot", English: "Run bot E2E healthcheck", Russian: "Запустить сквозную проверку бота"},
	{Command: "help", English: "Show admin help", Russian: "Показать помощь администратора"},
	{Command: "start", English: "Open the main menu", Russian: "Открыть главное меню"},
	{Command: "connect", English: "Open connection options", Russian: "Показать способы подключения"},
}

var adminFallbackCommandDefs = []fallbackCommandDef{
	{Usage: "/backup now|status|list|enable|disable|schedule HH:MM", Description: "Backup controls when you need to operate without the dashboard"},
	{Usage: "/restore list", Description: "Show local backup files for offline/manual restore"},
	{Usage: "/sync", Description: "Run a user sync immediately"},
	{Usage: "/test enable|disable", Description: "Toggle payment test mode"},
	{Usage: "/healthbot run", Description: "Run a synthetic end-to-end bot healthcheck"},
	{Usage: "/notify <telegram_id>", Description: "Send a subscription notification to one user"},
	{Usage: "/transactions [N]", Description: "Show the latest paid purchases"},
	{Usage: "/revenue", Description: "Show revenue summary"},
	{Usage: "/phones", Description: "Show provider/payment receiver status"},
	{Usage: "/setreferralbonus <amount>", Description: "Update the referral bonus"},
	{Usage: "/setphone <provider> <number>", Description: "Update provider phone number"},
	{Usage: "/setname <provider> <name>", Description: "Update provider account name"},
	{Usage: "/disablephone <provider>", Description: "Hide a provider from checkout"},
	{Usage: "/disablename <provider>", Description: "Clear a provider account name"},
}

var adminDashboardSections = []string{
	"Overview: revenue, transactions, provider status, backup health",
	"Payments: update provider details, disable providers, set referral bonus",
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

func resolveCommandDescription(def botCommandDef, lang string) string {
	if strings.EqualFold(lang, "ru") {
		return def.Russian
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
		sb.WriteString(fmt.Sprintf("<code>%s</code> — %s\n", command.Usage, command.Description))
	}
	return sb.String()
}

func renderUserHelpText() string {
	return "Use <code>/start</code> to open the bot and <code>/connect</code> to get your connection options."
}
