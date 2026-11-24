package remnawave

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

// panelUserJSON renders one minimal panel user payload the strict decoder
// accepts. Reconciliation only needs uuid/telegramId.
func panelUserJSON(id uuid.UUID, telegramID int) string {
	return fmt.Sprintf(`{
		"uuid": %q,
		"id": %d,
		"shortUuid": "short%d",
		"username": "user_%d",
		"expireAt": "2026-04-18T00:00:00Z",
		"telegramId": %d,
		"email": null,
		"description": null,
		"tag": null,
		"hwidDeviceLimit": null,
		"externalSquadUuid": null,
		"trojanPassword": "t",
		"vlessUuid": %q,
		"ssPassword": "s",
		"subRevokedAt": null,
		"subLastUserAgent": null,
		"subLastOpenedAt": null,
		"lastTrafficResetAt": null,
		"createdAt": "2026-01-01T00:00:00Z",
		"updatedAt": "2026-01-01T00:00:00Z",
		"subscriptionUrl": "https://sub.example.com/%s",
		"activeInternalSquads": [],
		"userTraffic": {"usedTrafficBytes": 0, "lifetimeUsedTrafficBytes": 0, "onlineAt": null, "firstConnectedAt": null, "lastConnectedNodeUuid": null}
	}`, id.String(), telegramID, telegramID, telegramID, telegramID, id.String(), id.String())
}

// fleetPanel serves a panel fleet whose page size is clamped to maxPage, which
// is exactly the situation that used to truncate the walk silently.
func fleetPanel(t *testing.T, fleet []uuid.UUID, maxPage int, totalOverride *float64) *httptest.Server {
	t.Helper()

	var totalRequests int64

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&totalRequests, 1)
		if r.URL.Path != "/api/users" {
			t.Errorf("request path = %q, want /api/users", r.URL.Path)
		}

		start, _ := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
		requestedSize, _ := strconv.ParseFloat(r.URL.Query().Get("size"), 64)
		// The SDK renders pagination as floats ("250.0000000000").
		pageSize := int(requestedSize)
		if pageSize > maxPage {
			pageSize = maxPage
		}

		end := int(start) + pageSize
		if end > len(fleet) {
			end = len(fleet)
		}
		if int(start) > len(fleet) {
			start = float64(len(fleet))
			end = len(fleet)
		}

		total := float64(len(fleet))
		if totalOverride != nil {
			total = *totalOverride
		}

		users := make([]string, 0, end-int(start))
		for i := int(start); i < end; i++ {
			users = append(users, panelUserJSON(fleet[i], 100000+i))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"response":{"users":[%s],"total":%d}}`, strings.Join(users, ","), int(total))))
	}))
}

// TestGetUsersPagesAgainstReportedTotal is the A3 pagination regression test.
// The panel clamps the requested page size, so a walk that stops on a short page
// would return only the first clamped page and leave the rest of the fleet unseen.
func TestGetUsersPagesAgainstReportedTotal(t *testing.T) {
	fleet := make([]uuid.UUID, 0, 30)
	for i := 0; i < 30; i++ {
		fleet = append(fleet, uuid.New())
	}

	server := fleetPanel(t, fleet, 10, nil)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	users, err := client.GetUsers(context.Background())
	if err != nil {
		t.Fatalf("GetUsers() error = %v", err)
	}
	if users == nil {
		t.Fatal("GetUsers() = nil, want the full fleet")
	}
	if len(*users) != len(fleet) {
		t.Fatalf("GetUsers() returned %d users, want %d", len(*users), len(fleet))
	}

	seen := make(map[uuid.UUID]bool, len(fleet))
	for _, user := range *users {
		seen[user.UUID] = true
	}
	for _, want := range fleet {
		if !seen[want] {
			t.Fatalf("GetUsers() is missing panel user %s", want)
		}
	}
}

// TestGetUsersRefusesMismatchedTotal is the A3 guard: a read that cannot be
// reconciled with the panel's own Total must be refused, not silently accepted.
func TestGetUsersRefusesMismatchedTotal(t *testing.T) {
	fleet := []uuid.UUID{uuid.New(), uuid.New()}
	reportedTotal := float64(len(fleet) + 5)

	server := fleetPanel(t, fleet, 10, &reportedTotal)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	users, err := client.GetUsers(context.Background())
	if err == nil {
		t.Fatalf("GetUsers() error = nil, want a partial-read error (users = %v)", users)
	}
	if users != nil {
		t.Fatalf("GetUsers() users = %v, want nil on a mismatched read", users)
	}
	if !strings.Contains(err.Error(), "refusing partial fleet read") {
		t.Fatalf("GetUsers() error = %q, want a partial fleet read refusal", err)
	}
}

// TestGetUsersReturnsEmptyWhenPanelIsEmpty keeps an empty panel distinct from a
// failed read: Sync treats zero users as "nothing to do", so it must not error.
func TestGetUsersReturnsEmptyWhenPanelIsEmpty(t *testing.T) {
	server := fleetPanel(t, nil, 10, nil)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	users, err := client.GetUsers(context.Background())
	if err != nil {
		t.Fatalf("GetUsers() error = %v", err)
	}
	if users == nil || len(*users) != 0 {
		t.Fatalf("GetUsers() = %v, want an empty fleet", users)
	}
}

var _ = remapi.NewNilInt
