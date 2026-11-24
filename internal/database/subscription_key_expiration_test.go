package database

import (
	"strings"
	"testing"
	"time"
)

// TestExpiringKeysSelectionFiltersAlreadyNotified is the U8 regression test.
//
// The expiration reminder used to run once a day at 16:00, so a daily schedule
// was itself the rate limit; the cron now runs hourly to stop a key that expires
// in the morning from waiting most of a day for its reminder. That is only safe
// because the query refuses keys that already carry expiration_notified_at, so
// this test pins the predicate that keeps the hourly run from messaging the same
// customer every hour.
func TestExpiringKeysSelectionFiltersAlreadyNotified(t *testing.T) {
	now := time.Date(2026, time.November, 25, 9, 0, 0, 0, time.UTC)

	query := buildFindExpiringKeysQuery(now, now.Add(3*24*time.Hour))
	sql, args, err := query.ToSql()
	if err != nil {
		t.Fatalf("buildFindExpiringKeysQuery().ToSql() error = %v", err)
	}

	if !strings.Contains(sql, "expiration_notified_at IS NULL") {
		t.Fatalf("expiring-keys SQL = %q, want an expiration_notified_at IS NULL dedupe filter", sql)
	}
	if !strings.Contains(sql, "expire_at IS NOT NULL") {
		t.Fatalf("expiring-keys SQL = %q, want a NULL-expiry guard", sql)
	}
	if containsStringArg(args, "expired") {
		t.Fatalf("expiring-keys args = %#v, want only active keys reminded", args)
	}
	if !containsStringArg(args, "active") {
		t.Fatalf("expiring-keys args = %#v, want active keys selected", args)
	}
	if !containsTimeArg(args, now) || !containsTimeArg(args, now.Add(3*24*time.Hour)) {
		t.Fatalf("expiring-keys args = %#v, want the three-day window bounds", args)
	}
}

// containsTimeArg reports whether any argument of the built query is the wanted
// instant, which is how the reminder window bounds reach the database.
func containsTimeArg(args []any, want time.Time) bool {
	for _, arg := range args {
		if value, ok := arg.(time.Time); ok && value.Equal(want) {
			return true
		}
	}
	return false
}
