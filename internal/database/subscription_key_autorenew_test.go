package database

import (
	"strings"
	"testing"
	"time"
)

// TestAutoRenewEligibilityIncludesExpiredKeys is the A4 regression test.
//
// SyncKeys marks lapsed keys 'expired', and the 48-hour auto-renew lookback
// window exists exactly to rescue keys that lapsed inside it. A status filter
// of 'active' alone makes those keys unreachable, so both the selection query
// and the claim fence must admit 'expired'.
func TestAutoRenewEligibilityIncludesExpiredKeys(t *testing.T) {
	now := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)

	query := buildFindExpiringAutoRenewKeysQuery(now, now.Add(3*24*time.Hour))
	sql, args, err := query.ToSql()
	if err != nil {
		t.Fatalf("buildFindExpiringAutoRenewKeysQuery().ToSql() error = %v", err)
	}

	if !strings.Contains(sql, "status IN") {
		t.Fatalf("auto-renew selection SQL = %q, want an explicit status IN filter", sql)
	}
	for _, want := range []string{"active", "expired"} {
		if !containsStringArg(args, want) {
			t.Fatalf("auto-renew selection args = %#v, want %q included", args, want)
		}
	}
	if strings.Contains(sql, "status <>") || strings.Contains(sql, "status !=") {
		t.Fatalf("auto-renew selection SQL = %q, want explicit statuses, not a wildcard", sql)
	}
}

// TestAutoRenewClaimFenceIncludesExpiredKeys covers the second half of A4: the
// atomic claim fence must accept the same statuses, otherwise the selected
// expired key is never actually claimed.
func TestAutoRenewClaimFenceIncludesExpiredKeys(t *testing.T) {
	if !strings.Contains(autoRenewClaimSQL, "status IN ('active', 'expired')") {
		t.Fatalf("auto-renew claim SQL = %q, want status IN ('active', 'expired')", autoRenewClaimSQL)
	}
	if !strings.Contains(autoRenewClaimSQL, "auto_renew = TRUE") {
		t.Fatalf("auto-renew claim SQL = %q, want auto_renew guard", autoRenewClaimSQL)
	}
}

// containsStringArg reports whether any argument of the built query carries the
// wanted string, whether as a bare value or inside a slice argument.
func containsStringArg(args []any, want string) bool {
	for _, arg := range args {
		switch value := arg.(type) {
		case string:
			if value == want {
				return true
			}
		case []string:
			for _, item := range value {
				if item == want {
					return true
				}
			}
		case []interface{}:
			for _, item := range value {
				if text, ok := item.(string); ok && text == want {
					return true
				}
			}
		}
	}
	return false
}
