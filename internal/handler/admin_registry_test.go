package handler

import (
	"strings"
	"testing"

	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

// TestPublicBotCommandsSpeakBurmeseAndEnglish pins the settled language
// direction: Burmese is the default, English is genuinely supported.
//
// The old test asserted that a Burmese locale resolved to English, because
// English was the only description set besides Russian. That is no longer the
// truth: a Burmese speaker now reads Burmese, and English speakers read English.
func TestPublicBotCommandsSpeakBurmeseAndEnglish(t *testing.T) {
	burmese := PublicBotCommands("my-MM")
	if len(burmese) != 3 {
		t.Fatalf("PublicBotCommands(my-MM) len = %d, want 3", len(burmese))
	}
	if burmese[0].Command != "start" {
		t.Fatalf("PublicBotCommands() first command = %q, want start", burmese[0].Command)
	}
	if burmese[0].Description != "ပင်မမီနူး ဖွင့်ရန်" {
		t.Fatalf("PublicBotCommands(my-MM) start = %q, want the Burmese description", burmese[0].Description)
	}

	english := PublicBotCommands("en-US")
	if english[0].Description != "Open the main menu" {
		t.Fatalf("PublicBotCommands(en-US) start = %q, want the English description", english[0].Description)
	}

	// Every description set must be distinct from the command itself, so a
	// customer never reads a raw command name where prose belongs.
	for _, commands := range [][]models.BotCommand{burmese, english} {
		for _, command := range commands {
			if command.Description == "" || command.Description == command.Command {
				t.Fatalf("command %q has a placeholder description %q", command.Command, command.Description)
			}
		}
	}
}

// TestAdminBotCommandsDescribeEveryEntry covers the admin scope, which follows
// the same rule.
func TestAdminBotCommandsDescribeEveryEntry(t *testing.T) {
	for _, lang := range []string{"my", "en"} {
		commands := AdminBotCommands(lang)
		if len(commands) != 3 {
			t.Fatalf("AdminBotCommands(%s) len = %d, want 3", lang, len(commands))
		}
		if commands[0].Command != "admin" {
			t.Fatalf("AdminBotCommands(%s) first command = %q, want admin", lang, commands[0].Command)
		}
		if commands[0].Description == "" {
			t.Fatalf("AdminBotCommands(%s) admin description is empty", lang)
		}
		if commands[1].Command != "healthbot" || commands[2].Command != "help" {
			t.Fatalf("AdminBotCommands(%s) = %#v, want admin/healthbot/help", lang, commands)
		}
	}
}

// TestNoRussianSurvivesInTheCommandList proves the dropped language is gone
// rather than merely unreferenced: no Cyrillic description may be emitted for
// any language code, Russian included.
func TestNoRussianSurvivesInTheCommandList(t *testing.T) {
	for _, lang := range []string{"ru", "ru-RU", "en", "my", "de"} {
		for _, commands := range [][]models.BotCommand{PublicBotCommands(lang), AdminBotCommands(lang)} {
			for _, command := range commands {
				if containsCyrillic(command.Description) {
					t.Fatalf("lang %q still returns a Russian description %q for %q", lang, command.Description, command.Command)
				}
			}
		}
	}
}

// TestUserHelpTextNamesTheShopAndTheSupportDoor replaces the old single
// hard-coded English sentence.
func TestUserHelpTextNamesTheShopAndTheSupportDoor(t *testing.T) {
	restore := config.SetSupportURLForTesting("")
	defer restore()

	h := Handler{translation: loadHandlerTestTranslations(t)}

	for _, lang := range []string{"en", "my"} {
		text := h.renderUserHelpText(lang)
		if !strings.Contains(text, config.DefaultSupportURL) {
			t.Fatalf("%s help text = %q, want the support account to be reachable", lang, text)
		}
		if strings.Contains(text, "%s") {
			t.Fatalf("%s help text carries an unformatted verb: %q", lang, text)
		}
	}

	restoreCustom := config.SetSupportURLForTesting("https://t.me/custom_help")
	defer restoreCustom()
	if text := h.renderUserHelpText("en"); !strings.Contains(text, "https://t.me/custom_help") {
		t.Fatalf("help text = %q, want the configured SUPPORT_URL", text)
	}
}

func containsCyrillic(s string) bool {
	for _, r := range s {
		if r >= '\u0400' && r <= '\u04FF' {
			return true
		}
	}
	return false
}
