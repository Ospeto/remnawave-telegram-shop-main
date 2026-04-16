package remnawave

import (
	"testing"
	"time"

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
