package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
)

func TestNormalizeAdjustmentAmount_TwoDecimals(t *testing.T) {
	got, err := normalizeAdjustmentAmount(10.005)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 10.01 {
		t.Fatalf("got %v want 10.01", got)
	}
}

func TestNormalizeAdjustmentAmount_RejectsNonPositive(t *testing.T) {
	if _, err := normalizeAdjustmentAmount(0); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := normalizeAdjustmentAmount(-1); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestBuildCreateFinancialAdjustmentSQL_UsesIdempotencyConflict(t *testing.T) {
	sql := buildCreateFinancialAdjustmentSQL()
	if !strings.Contains(sql, "INSERT INTO financial_adjustment") {
		t.Fatalf("missing insert: %s", sql)
	}
	if !strings.Contains(sql, "ON CONFLICT (idempotency_key) DO NOTHING") {
		t.Fatalf("missing idempotency conflict clause: %s", sql)
	}
}

func TestBuildSumRefundsByPeriodSQL_YangonAndAdminExclusion(t *testing.T) {
	sql, err := buildSumRefundsByPeriodSQL(RevenuePeriodDay)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Asia/Yangon",
		"adjustment_type = 'refund'",
		"telegram_id",
		"effective_at >= $1",
		"effective_at < $2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in %s", want, sql)
		}
	}
}

func TestCreateFinancialAdjustmentInput_RequiresIdempotencyKey(t *testing.T) {
	_, err := validateCreateFinancialAdjustmentInput(CreateFinancialAdjustmentInput{
		AdjustmentType: FinancialAdjustmentTypeRefund,
		Amount:         100,
		Currency:       "MMK",
		EffectiveAt:    time.Now(),
		CreatedBy:      "admin:1",
		IdempotencyKey: "",
	})
	if err == nil {
		t.Fatal("expected missing idempotency key error")
	}
}

func validCreateInput() CreateFinancialAdjustmentInput {
	return CreateFinancialAdjustmentInput{
		AdjustmentType: FinancialAdjustmentTypeRefund,
		Amount:         100.005,
		Currency:       "MMK",
		EffectiveAt:    time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC),
		Reason:         "customer refund",
		ExternalRef:    "ref-1",
		CreatedBy:      "admin:1",
		IdempotencyKey: "idem-key-1",
	}
}

type stubRow struct {
	err  error
	vals []interface{}
}

func (r stubRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	if len(r.vals) != len(dest) {
		return fmt.Errorf("stubRow: dest len %d != vals len %d", len(dest), len(r.vals))
	}
	for i := range dest {
		switch d := dest[i].(type) {
		case *int64:
			*d = r.vals[i].(int64)
		case **int64:
			switch v := r.vals[i].(type) {
			case nil:
				*d = nil
			case *int64:
				*d = v
			case int64:
				vv := v
				*d = &vv
			default:
				return fmt.Errorf("stubRow: **int64 got %T at %d", r.vals[i], i)
			}
		case *float64:
			*d = r.vals[i].(float64)
		case *string:
			*d = r.vals[i].(string)
		case *FinancialAdjustmentType:
			*d = r.vals[i].(FinancialAdjustmentType)
		case *time.Time:
			*d = r.vals[i].(time.Time)
		default:
			return fmt.Errorf("stubRow: unsupported dest type %T at %d", dest[i], i)
		}
	}
	return nil
}

type scriptedQuerier struct {
	calls []struct {
		sql  string
		args []interface{}
		row  pgx.Row
	}
	i int
}

func (q *scriptedQuerier) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if q.i >= len(q.calls) {
		return stubRow{err: fmt.Errorf("unexpected QueryRow call %d sql=%s", q.i, sql)}
	}
	call := q.calls[q.i]
	q.i++
	call.sql = sql
	call.args = append([]interface{}(nil), args...)
	q.calls[q.i-1] = call
	return call.row
}

func sampleAdjustmentRow(id int64, amount float64, key string) stubRow {
	now := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	return stubRow{vals: []interface{}{
		id,
		(*int64)(nil),
		FinancialAdjustmentTypeRefund,
		amount,
		"MMK",
		now,
		"customer refund",
		"ref-1",
		"admin:1",
		key,
		now,
	}}
}

func TestCreateFinancialAdjustment_FirstInsertCreatedTrue(t *testing.T) {
	q := &scriptedQuerier{calls: []struct {
		sql  string
		args []interface{}
		row  pgx.Row
	}{
		{row: sampleAdjustmentRow(42, 100.01, "idem-key-1")},
	}}

	got, created, err := createFinancialAdjustment(context.Background(), q, validCreateInput())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if got == nil || got.ID != 42 {
		t.Fatalf("got %+v", got)
	}
	if got.Amount != 100.01 {
		t.Fatalf("amount got %v want 100.01 (normalized once)", got.Amount)
	}
	if q.i != 1 {
		t.Fatalf("expected 1 QueryRow, got %d", q.i)
	}
	if !strings.Contains(q.calls[0].sql, "INSERT INTO financial_adjustment") {
		t.Fatalf("expected insert SQL, got %s", q.calls[0].sql)
	}
	// Normalized amount is the 3rd bind after purchase_id and adjustment_type.
	if len(q.calls[0].args) < 3 || q.calls[0].args[2] != 100.01 {
		t.Fatalf("expected normalized amount 100.01 in args, got %#v", q.calls[0].args)
	}
}

func TestCreateFinancialAdjustment_DuplicateIdempotencyKeyCreatedFalse(t *testing.T) {
	q := &scriptedQuerier{calls: []struct {
		sql  string
		args []interface{}
		row  pgx.Row
	}{
		{row: stubRow{err: pgx.ErrNoRows}},
		{row: sampleAdjustmentRow(7, 50.00, "idem-key-1")},
	}}

	got, created, err := createFinancialAdjustment(context.Background(), q, validCreateInput())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if created {
		t.Fatal("expected created=false on idempotent replay")
	}
	if got == nil || got.ID != 7 {
		t.Fatalf("got %+v", got)
	}
	if q.i != 2 {
		t.Fatalf("expected insert + reload, got %d calls", q.i)
	}
	if !strings.Contains(q.calls[1].sql, "WHERE idempotency_key = $1") {
		t.Fatalf("expected reload by idempotency key, got %s", q.calls[1].sql)
	}
	if len(q.calls[1].args) != 1 || q.calls[1].args[0] != "idem-key-1" {
		t.Fatalf("reload args: %#v", q.calls[1].args)
	}
}

func TestCreateFinancialAdjustment_ReloadFailurePropagates(t *testing.T) {
	reloadErr := errors.New("connection reset")
	q := &scriptedQuerier{calls: []struct {
		sql  string
		args []interface{}
		row  pgx.Row
	}{
		{row: stubRow{err: pgx.ErrNoRows}},
		{row: stubRow{err: reloadErr}},
	}}

	got, created, err := createFinancialAdjustment(context.Background(), q, validCreateInput())
	if err == nil {
		t.Fatal("expected reload error")
	}
	if created {
		t.Fatal("expected created=false on error")
	}
	if got != nil {
		t.Fatalf("expected nil row, got %+v", got)
	}
	if !errors.Is(err, reloadErr) {
		t.Fatalf("expected wrapped reload cause, got %v", err)
	}
	if !strings.Contains(err.Error(), "load existing financial_adjustment") {
		t.Fatalf("expected load existing prefix, got %v", err)
	}
}

func TestClassifyFinancialAdjustmentError_PostgreSQLConstraints(t *testing.T) {
	cases := []struct {
		name       string
		pg         *pgconn.PgError
		wantSentinel error
	}{
		{
			name:         "foreign key",
			pg:           &pgconn.PgError{Code: "23503", ConstraintName: "financial_adjustment_purchase_id_fkey"},
			wantSentinel: ErrFinancialAdjustmentForeignKey,
		},
		{
			name:         "check constraint",
			pg:           &pgconn.PgError{Code: "23514", ConstraintName: "financial_adjustment_amount_check"},
			wantSentinel: ErrFinancialAdjustmentCheck,
		},
		{
			name:         "unrelated unique",
			pg:           &pgconn.PgError{Code: "23505", ConstraintName: "some_other_unique"},
			wantSentinel: ErrFinancialAdjustmentUnique,
		},
		{
			name:         "idempotency unique distinct",
			pg:           &pgconn.PgError{Code: "23505", ConstraintName: "financial_adjustment_idempotency_key_uidx"},
			wantSentinel: ErrFinancialAdjustmentIdempotencyConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Errorf("db: %w", tc.pg)
			got := classifyFinancialAdjustmentError(raw)
			if !errors.Is(got, tc.wantSentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", got, tc.wantSentinel)
			}
			var pgErr *pgconn.PgError
			if !errors.As(got, &pgErr) {
				t.Fatalf("expected *pgconn.PgError cause preserved, got %v", got)
			}
			if pgErr.Code != tc.pg.Code {
				t.Fatalf("code %s want %s", pgErr.Code, tc.pg.Code)
			}
		})
	}

	plain := errors.New("network blip")
	if got := classifyFinancialAdjustmentError(plain); !errors.Is(got, plain) {
		t.Fatalf("non-pg error should pass through, got %v", got)
	}
}

func TestCreateFinancialAdjustment_ClassifiesInsertPostgreSQLErrors(t *testing.T) {
	cases := []struct {
		name         string
		pg           *pgconn.PgError
		wantSentinel error
	}{
		{
			name:         "fk",
			pg:           &pgconn.PgError{Code: "23503", ConstraintName: "financial_adjustment_purchase_id_fkey"},
			wantSentinel: ErrFinancialAdjustmentForeignKey,
		},
		{
			name:         "check",
			pg:           &pgconn.PgError{Code: "23514", ConstraintName: "financial_adjustment_amount_check"},
			wantSentinel: ErrFinancialAdjustmentCheck,
		},
		{
			name:         "unique other",
			pg:           &pgconn.PgError{Code: "23505", ConstraintName: "other_uidx"},
			wantSentinel: ErrFinancialAdjustmentUnique,
		},
		{
			name:         "unique idempotency",
			pg:           &pgconn.PgError{Code: "23505", ConstraintName: "financial_adjustment_idempotency_key_uidx"},
			wantSentinel: ErrFinancialAdjustmentIdempotencyConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &scriptedQuerier{calls: []struct {
				sql  string
				args []interface{}
				row  pgx.Row
			}{
				{row: stubRow{err: tc.pg}},
			}}
			got, created, err := createFinancialAdjustment(context.Background(), q, validCreateInput())
			if err == nil {
				t.Fatal("expected error")
			}
			if created || got != nil {
				t.Fatalf("created=%v got=%+v", created, got)
			}
			if !errors.Is(err, tc.wantSentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tc.wantSentinel)
			}
			if !strings.Contains(err.Error(), "insert financial_adjustment") {
				t.Fatalf("expected insert prefix, got %v", err)
			}
			// Idempotency 23505 must not fall through to the reload path.
			if q.i != 1 {
				t.Fatalf("expected single insert call, got %d", q.i)
			}
		})
	}
}
