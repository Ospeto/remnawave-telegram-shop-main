package handler

import (
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

// TestFormatExpiryDateTimeMatchesReaderLocale is the U9 regression test.
//
// The connect screen rendered every expiry as "02.01.2006 15:04", a day-first
// German-style layout, for every language. An English reader parses 03.04.2026
// as March 4th, and this shop sells 30-day subscriptions, so a misread date is a
// wrong accusation about expiring early. English now gets a month name.
func TestFormatExpiryDateTimeMatchesReaderLocale(t *testing.T) {
	expireAt := time.Date(2026, time.April, 3, 15, 4, 0, 0, time.UTC)

	if got := formatExpiryDateTime(expireAt, "en"); got != "Apr 3, 2026 15:04" {
		t.Fatalf("formatExpiryDateTime(en) = %q, want an unambiguous English month name", got)
	}
	if got := formatExpiryDateTime(expireAt, "en-US"); got != "Apr 3, 2026 15:04" {
		t.Fatalf("formatExpiryDateTime(en-US) = %q, want the English form for a hyphenated locale", got)
	}
	// Myanmar readers keep the day-first numeric form they already read.
	if got := formatExpiryDateTime(expireAt, "my"); got != "03.04.2026 15:04" {
		t.Fatalf("formatExpiryDateTime(my) = %q, want the day-first numeric form", got)
	}
	if got := formatExpiryDateTime(expireAt, ""); got != "03.04.2026 15:04" {
		t.Fatalf("formatExpiryDateTime(default) = %q, want the day-first numeric form", got)
	}
}

// TestConnectScreenNoLongerUsesOneHardcodedLayout pins the call site, because a
// helper nobody calls would still leave every customer on the old format.
func TestConnectScreenNoLongerUsesOneHardcodedLayout(t *testing.T) {
	expireAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	customer := &database.Customer{ID: 1, TelegramID: 42, ExpireAt: &expireAt}

	text := buildConnectText(customer, "en")
	if !strings.Contains(text, expireAt.Format("Jan 2, 2006")) {
		t.Fatalf("buildConnectText(en) = %q, want the English date form %q", text, expireAt.Format("Jan 2, 2006"))
	}
	if strings.Contains(text, expireAt.Format("02.01.2006")) {
		t.Fatalf("buildConnectText(en) = %q, want the day-first form gone from the English screen", text)
	}
}
