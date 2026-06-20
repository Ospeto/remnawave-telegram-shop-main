package remnawave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

func TestGenerateUsernameUsesMoreThanLastFourTxnChars(t *testing.T) {
	// These transaction IDs share the same last 4 chars.
	// Username generation must still avoid collisions.
	u1 := generateUsername("", 1, 123456789, 0, "AAAA0001")
	u2 := generateUsername("", 1, 123456789, 0, "BBBB0001")

	if u1 == u2 {
		t.Fatalf("generateUsername() collision: %q == %q", u1, u2)
	}
}

func TestParseUserLooseSupportsMinimalPayload(t *testing.T) {
	id := uuid.New()
	expireAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	user, err := parseUserLoose(map[string]any{
		"uuid":            id.String(),
		"username":        "wavy_test_user",
		"subscriptionUrl": "https://sub.example.com/a",
		"expireAt":        expireAt,
		"telegramId":      float64(123456),
		"userTraffic": map[string]any{
			"usedTrafficBytes": float64(2048),
		},
	})
	if err != nil {
		t.Fatalf("parseUserLoose() error = %v", err)
	}
	if user == nil {
		t.Fatal("parseUserLoose() returned nil user")
	}
	if user.UUID != id {
		t.Fatalf("user.UUID = %s, want %s", user.UUID, id)
	}
	if user.Username != "wavy_test_user" {
		t.Fatalf("user.Username = %q, want wavy_test_user", user.Username)
	}
	if user.SubscriptionUrl != "https://sub.example.com/a" {
		t.Fatalf("user.SubscriptionUrl = %q, want https://sub.example.com/a", user.SubscriptionUrl)
	}
	if user.UserTraffic.UsedTrafficBytes != 2048 {
		t.Fatalf("user.UserTraffic.UsedTrafficBytes = %v, want 2048", user.UserTraffic.UsedTrafficBytes)
	}
}

func TestValidateUpdatedUserStateRejectsStaleExpiry(t *testing.T) {
	requestedExpire := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)

	err := validateUpdatedUserState(&remapi.User{
		ExpireAt:          requestedExpire.Add(-1 * time.Minute),
		TrafficLimitBytes: remapi.NewOptInt(1000),
	}, requestedExpire, 1000)
	if err == nil {
		t.Fatal("validateUpdatedUserState() error = nil, want stale expiry rejection")
	}
}

func TestValidateUpdatedUserStateAllowsSmallExpirySkew(t *testing.T) {
	requestedExpire := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)

	err := validateUpdatedUserState(&remapi.User{
		ExpireAt:          requestedExpire.Add(-1 * time.Second),
		TrafficLimitBytes: remapi.NewOptInt(1000),
	}, requestedExpire, 1000)
	if err != nil {
		t.Fatalf("validateUpdatedUserState() error = %v, want nil", err)
	}
}

func TestValidateUpdatedUserStateRejectsTrafficMismatch(t *testing.T) {
	requestedExpire := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)

	err := validateUpdatedUserState(&remapi.User{
		ExpireAt:          requestedExpire,
		TrafficLimitBytes: remapi.NewOptInt(500),
	}, requestedExpire, 1000)
	if err == nil {
		t.Fatal("validateUpdatedUserState() error = nil, want traffic mismatch rejection")
	}
}

func TestParseInternalSquadUUIDsLooseFromResponseEnvelope(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	payload := []byte(`{
		"success": true,
		"response": {
			"internalSquads": [
				{"uuid": "` + id1.String() + `"},
				{"uuid": "` + id2.String() + `"}
			]
		}
	}`)

	ids := parseInternalSquadUUIDsLoose(payload)
	if len(ids) != 2 {
		t.Fatalf("parseInternalSquadUUIDsLoose() len = %d, want 2", len(ids))
	}
	if ids[0] != id1 || ids[1] != id2 {
		t.Fatalf("parseInternalSquadUUIDsLoose() = %v, want [%s %s]", ids, id1, id2)
	}
}

func TestPingFallsBackToRawUsersListWhenStrictDecodeDrifts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Fatalf("request path = %q, want /api/users", r.URL.Path)
		}
		if got, err := strconv.ParseFloat(r.URL.Query().Get("size"), 64); err != nil || got != 1 {
			t.Fatalf("size query = %q, want numeric 1", r.URL.Query().Get("size"))
		}
		if got, err := strconv.ParseFloat(r.URL.Query().Get("start"), 64); err != nil || got != 0 {
			t.Fatalf("start query = %q, want numeric 0", r.URL.Query().Get("start"))
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %q, want %q", got, "Bearer token")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"response": {
				"users": [{
					"uuid": "8d271ef0-1f93-4553-85e6-d7ef4f7d84d0",
					"username": "drifted_user",
					"subscriptionUrl": "https://sub.example.com/a",
					"expireAt": "2026-04-18T00:00:00Z",
					"telegramId": 123456
				}],
				"total": 1
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v, want nil", err)
	}
}

func TestNewClientRejectsMissingConnectionConfig(t *testing.T) {
	if _, err := NewClient("", "token", ""); err == nil {
		t.Fatal("NewClient() error = nil for empty URL, want error")
	}
	if _, err := NewClient("https://remnawave.example.com", "", ""); err == nil {
		t.Fatal("NewClient() error = nil for empty token, want error")
	}
}
