package handler

import (
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// TestNormalizeSupportedLanguage pins the settled language direction at the tag
// level: Burmese and English are supported, Russian is not.
func TestNormalizeSupportedLanguage(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"en", "en"},
		{"EN", "en"},
		{"en-US", "en"},
		{"en_GB", "en"},
		{" my ", "my"},
		{"my-MM", "my"},
		{"ru", ""},
		{"ru-RU", ""},
		{"de", ""},
		{"", ""},
	}

	for _, tt := range tests {
		if got := normalizeSupportedLanguage(tt.in); got != tt.want {
			t.Fatalf("normalizeSupportedLanguage(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestLanguageNeedsStorage proves the stored choice is no longer overwritten on
// every update: only a row with no supported language is written to.
func TestLanguageNeedsStorage(t *testing.T) {
	tests := []struct {
		name     string
		customer *database.Customer
		want     bool
	}{
		{"nil customer", nil, false},
		{"empty language", &database.Customer{}, true},
		{"stored english", &database.Customer{Language: "en"}, false},
		{"stored burmese", &database.Customer{Language: "my"}, false},
		{"stored regional tag", &database.Customer{Language: "en-US"}, false},
		{"stored russian only", &database.Customer{Language: "ru"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := languageNeedsStorage(tt.customer); got != tt.want {
				t.Fatalf("languageNeedsStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFormatExpiryDateTimeKeepsTheEnglishArm guards the formatter the settled
// direction makes reachable: an English customer must read the English layout,
// and it must still be live code rather than an unreachable branch.
func TestFormatExpiryDateTimeKeepsTheEnglishArm(t *testing.T) {
	expireAt := time.Date(2026, time.November, 28, 9, 5, 0, 0, time.UTC)

	if got := formatExpiryDateTime(expireAt, "en"); got != "Nov 28, 2026 09:05" {
		t.Fatalf("formatExpiryDateTime(en) = %q, want %q", got, "Nov 28, 2026 09:05")
	}
	if got := formatExpiryDateTime(expireAt, "en-US"); got != "Nov 28, 2026 09:05" {
		t.Fatalf("formatExpiryDateTime(en-US) = %q, want %q", got, "Nov 28, 2026 09:05")
	}
	if got := formatExpiryDateTime(expireAt, "my"); got != "28.11.2026 09:05" {
		t.Fatalf("formatExpiryDateTime(my) = %q, want %q", got, "28.11.2026 09:05")
	}
}
