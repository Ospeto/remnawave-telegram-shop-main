package remnawave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

// TestUpdateUserMarksServerErrorAsUnconfirmed is the A7 regression test: a panel
// 5xx may have committed the mutation before failing, so the caller must re-read
// panel state instead of refunding or retrying.
func TestUpdateUserMarksServerErrorAsUnconfirmed(t *testing.T) {
	server := panelRefusalServer(t, 500, panelInternalErrorBody)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	userID := uuid.New()
	existingUser := &remapi.User{
		UUID:     userID,
		Username: "wavy_test_user",
		ExpireAt: time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC),
	}

	updated, err := client.updateUser(context.Background(), existingUser, 1000, 30)
	if err == nil {
		t.Fatalf("updateUser() error = nil, want panel error (updated = %v)", updated)
	}
	if !UpdateOutcomeUnconfirmed(err) {
		t.Fatalf("updateUser() error = %q, want an unconfirmed outcome error", err)
	}
}

// TestUpdateUserKeepsClientRefusalDefinite guards the other half of A7: a 400
// means the panel rejected the request before applying it, so the caller may
// still refund safely.
func TestUpdateUserKeepsClientRefusalDefinite(t *testing.T) {
	server := panelRefusalServer(t, 400, panelBadRequestBody)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	existingUser := &remapi.User{
		UUID:     uuid.New(),
		Username: "wavy_test_user",
		ExpireAt: time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC),
	}

	_, err = client.updateUser(context.Background(), existingUser, 1000, 30)
	if err == nil {
		t.Fatal("updateUser() error = nil, want panel refusal")
	}
	if UpdateOutcomeUnconfirmed(err) {
		t.Fatalf("updateUser() error = %q, want a definite pre-mutation refusal", err)
	}
	if !strings.Contains(err.Error(), "Panel refused the request") {
		t.Fatalf("updateUser() error = %q, want panel message", err)
	}
}

// TestUpdateUserMarksTransportFailureAsUnconfirmed covers a lost response, which
// says nothing about whether the panel applied the update.
func TestUpdateUserMarksTransportFailureAsUnconfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/internal-squads" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"response":{"total":0,"internalSquads":[]}}`))
			return
		}

		// Drop the connection mid-request so the update response never arrives.
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Errorf("response writer does not support hijacking")
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack failed: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	existingUser := &remapi.User{
		UUID:     uuid.New(),
		Username: "wavy_test_user",
		ExpireAt: time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC),
	}

	_, err = client.updateUser(context.Background(), existingUser, 1000, 30)
	if err == nil {
		t.Fatal("updateUser() error = nil, want transport failure after server close")
	}
	if !UpdateOutcomeUnconfirmed(err) {
		t.Fatalf("updateUser() error = %q, want an unconfirmed outcome error", err)
	}
}

// TestExtensionAppliedComparesObservedPanelState proves the re-read decision the
// payment and auto-renew paths rely on to avoid refunding an applied extension.
func TestExtensionAppliedComparesObservedPanelState(t *testing.T) {
	currentExpire := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)
	expectedExpire := ExtendedExpireAfter(currentExpire, 30)

	if got := ExtensionApplied(expectedExpire, &remapi.User{ExpireAt: expectedExpire}); !got {
		t.Fatal("ExtensionApplied() = false for the requested expiry, want true")
	}
	if got := ExtensionApplied(expectedExpire, &remapi.User{ExpireAt: expectedExpire.Add(time.Second)}); !got {
		t.Fatalf("ExtensionApplied() = false for a slightly later expiry (%v), want true", expectedExpire)
	}
	if got := ExtensionApplied(expectedExpire, &remapi.User{ExpireAt: currentExpire}); got {
		t.Fatal("ExtensionApplied() = true for an untouched expiry, want false")
	}
	if got := ExtensionApplied(expectedExpire, nil); got {
		t.Fatal("ExtensionApplied() = true for nil user, want false")
	}
}
