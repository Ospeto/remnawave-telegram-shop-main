package database

import (
	"strings"
	"testing"
)

func TestBuildRevenueSummaryQueryExcludesAdminTelegramID(t *testing.T) {
	query := buildRevenueSummaryQuery()

	if !strings.Contains(query, "JOIN customer c ON c.id = p.customer_id") {
		t.Fatalf("buildRevenueSummaryQuery() missing customer join: %s", query)
	}
	if !strings.Contains(query, "($2::bigint = 0 OR c.telegram_id <> $2)") {
		t.Fatalf("buildRevenueSummaryQuery() missing admin exclusion clause: %s", query)
	}
	if !strings.Contains(query, "AT TIME ZONE 'Asia/Yangon'") {
		t.Fatalf("buildRevenueSummaryQuery() missing Yangon timezone bucketing: %s", query)
	}
	if strings.Contains(query, "AT TIME ZONE 'UTC'") {
		t.Fatalf("buildRevenueSummaryQuery() should not regress to UTC bucketing: %s", query)
	}
}
