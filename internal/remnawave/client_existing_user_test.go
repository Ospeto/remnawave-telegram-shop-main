package remnawave

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// panelUserPayload renders one complete panel user payload so the strict SDK
// decoder accepts it. The panel's schema marks most traffic fields required.
func panelUserPayload(userUUID uuid.UUID, telegramID int64, username, subURL, expireAt string, trafficBytes int) map[string]any {
	return map[string]any{
		"uuid":                 userUUID.String(),
		"id":                   telegramID,
		"shortUuid":            "short-" + userUUID.String(),
		"username":             username,
		"expireAt":             expireAt,
		"telegramId":           telegramID,
		"email":                nil,
		"description":          nil,
		"tag":                  nil,
		"hwidDeviceLimit":      nil,
		"externalSquadUuid":    nil,
		"trojanPassword":       "t",
		"vlessUuid":            userUUID.String(),
		"ssPassword":           "s",
		"subRevokedAt":         nil,
		"subLastUserAgent":     nil,
		"subLastOpenedAt":      nil,
		"lastTrafficResetAt":   nil,
		"createdAt":            "2026-01-01T00:00:00Z",
		"updatedAt":            "2026-01-01T00:00:00Z",
		"subscriptionUrl":      subURL,
		"activeInternalSquads": []any{},
		"trafficLimitBytes":    trafficBytes,
		"trafficLimitStrategy": "NO_RESET",
		"userTraffic": map[string]any{
			"usedTrafficBytes":         0,
			"lifetimeUsedTrafficBytes": 0,
			"onlineAt":                 nil,
			"firstConnectedAt":         nil,
			"lastConnectedNodeUuid":    nil,
		},
	}
}

// existingUserPanel serves a panel that already holds one user for the requested
// Telegram id. The returned slice collects every PATCH body the client sends.
func existingUserPanel(t *testing.T, existingUUID uuid.UUID, telegramID int64) (*httptest.Server, *[]map[string]any) {
	t.Helper()

	remote := panelUserPayload(existingUUID, telegramID, "existing_1001", "https://sub.example.com/existing", "2026-05-01T00:00:00Z", 1024)
	patches := make([]map[string]any, 0, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/internal-squads":
			_, _ = w.Write([]byte(`{"response":{"total":0,"internalSquads":[]}}`))
			return
		case strings.HasPrefix(r.URL.Path, "/api/users/by-telegram-id/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"response": []any{remote}})
			return
		case r.Method == http.MethodPatch && r.URL.Path == "/api/users":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			patches = append(patches, body)
			for k, v := range body {
				remote[k] = v
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"response": remote})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/api/users":
			t.Error("trial path must not create a second panel user")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))

	return server, &patches
}

// TestCreateOrUpdateUserRefusesTrialOnExistingPanelUser is the A8 regression test.
// A trial for a Telegram id that already exists in the panel used to hand back the
// paid user untouched, so the caller inserted a second subscription_key row on the
// same remnawave_uuid and the trial failed with a unique-index violation.
func TestCreateOrUpdateUserRefusesTrialOnExistingPanelUser(t *testing.T) {
	existingUUID := uuid.New()
	server, patches := existingUserPanel(t, existingUUID, 1001)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	user, err := client.CreateOrUpdateUser(context.Background(), 1, 1001, 1024, 3, true)
	if err == nil {
		t.Fatalf("CreateOrUpdateUser(trial) error = nil, want ErrExistingPanelUser (user = %v)", user)
	}
	if !errors.Is(err, ErrExistingPanelUser) {
		t.Fatalf("CreateOrUpdateUser(trial) error = %v, want ErrExistingPanelUser", err)
	}
	if user != nil {
		t.Fatalf("CreateOrUpdateUser(trial) user = %v, want nil", user)
	}
	if len(*patches) != 0 {
		t.Fatalf("trial path mutated the existing panel user %d times, want 0", len(*patches))
	}
}

// TestCreateOrUpdateUserPaidPathStillUpdatesExistingUser guards the non-trial
// branch: refusing trials must not change paid renewal behaviour.
func TestCreateOrUpdateUserPaidPathStillUpdatesExistingUser(t *testing.T) {
	existingUUID := uuid.New()
	server, patches := existingUserPanel(t, existingUUID, 1001)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	user, err := client.CreateOrUpdateUser(context.Background(), 1, 1001, 1024, 30, false)
	if err != nil {
		t.Fatalf("CreateOrUpdateUser(paid) error = %v", err)
	}
	if user == nil || user.UUID != existingUUID {
		t.Fatalf("CreateOrUpdateUser(paid) user = %v, want existing panel user", user)
	}
	if len(*patches) != 1 {
		t.Fatalf("paid path patched the panel %d times, want 1", len(*patches))
	}
}
