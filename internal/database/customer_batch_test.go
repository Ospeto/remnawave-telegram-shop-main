package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgconn"
)

type captureExec struct {
	sql  string
	args []interface{}
}

func (c *captureExec) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	c.sql = sql
	c.args = append([]interface{}(nil), arguments...)
	return pgconn.CommandTag{}, nil
}

func TestUpdateBatchUsesTimestamptzCast(t *testing.T) {
	c := &captureExec{}
	expireAt := time.Date(2026, 3, 23, 12, 30, 0, 0, time.FixedZone("MMK", 6*60*60+30*60))

	if err := updateBatchCustomers(context.Background(), c, []Customer{{
		TelegramID:       12345,
		ExpireAt:         &expireAt,
		SubscriptionLink: ptrString("https://example.com/sub"),
	}}); err != nil {
		t.Fatalf("updateBatchCustomers returned error: %v", err)
	}

	if !strings.Contains(c.sql, "::timestamptz") {
		t.Fatalf("expected timestamptz cast in SQL, got %q", c.sql)
	}
}

func ptrString(v string) *string {
	return &v
}
