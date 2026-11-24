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

// panelRefusalServer serves the internal squads endpoint so client calls reach
// the endpoint under test, then answers every other request with the given
// status and body.
func panelRefusalServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/api/internal-squads" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"response":{"total":0,"internalSquads":[]}}`))
			return
		}

		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

const panelBadRequestBody = `{
	"message": "Panel refused the request",
	"statusCode": 400,
	"errors": [{"validation": "uuid", "code": "A001", "message": "user is invalid", "path": ["uuid"]}]
}`

const panelInternalErrorBody = `{
	"message": "Panel exploded",
	"errorCode": "E500"
}`

// TestGetUsersReturnsErrorOnPanelRefusal is the A1 regression test: the panel
// client must turn an ordinary 400 into a Go error instead of panicking on an
// unchecked type assertion, because the panic kills the bot for every customer.
func TestGetUsersReturnsErrorOnPanelRefusal(t *testing.T) {
	server := panelRefusalServer(t, http.StatusBadRequest, panelBadRequestBody)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	users, err := client.GetUsers(context.Background())
	if err == nil {
		t.Fatalf("GetUsers() error = nil, want panel refusal error (users = %v)", users)
	}
	if users != nil {
		t.Fatalf("GetUsers() users = %v, want nil", users)
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Panel refused the request") {
		t.Fatalf("GetUsers() error = %q, want status and panel message", err)
	}
}

// TestGetUsersReturnsErrorOnPanelInternalError covers the 500 branch, which
// decodes into *remapi.InternalServerError and previously panicked as well.
func TestGetUsersReturnsErrorOnPanelInternalError(t *testing.T) {
	server := panelRefusalServer(t, http.StatusInternalServerError, panelInternalErrorBody)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := client.GetUsers(context.Background()); err == nil {
		t.Fatal("GetUsers() error = nil, want panel internal error")
	} else if !strings.Contains(err.Error(), "Panel exploded") {
		t.Fatalf("GetUsers() error = %q, want panel message", err)
	}
}

// TestUpdateUserReturnsErrorOnPanelRefusal is the A1 regression test for the
// update path (used by every extension and renewal).
func TestUpdateUserReturnsErrorOnPanelRefusal(t *testing.T) {
	server := panelRefusalServer(t, http.StatusBadRequest, panelBadRequestBody)
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
		t.Fatalf("updateUser() error = nil, want panel refusal error (updated = %v)", updated)
	}
	if updated != nil {
		t.Fatalf("updateUser() user = %v, want nil", updated)
	}
	if !strings.Contains(err.Error(), "Panel refused the request") {
		t.Fatalf("updateUser() error = %q, want panel message", err)
	}
}

// TestCreateUserReturnsErrorOnPanelRefusal is the A1 regression test for the
// create path (used by every new purchase and trial).
func TestCreateUserReturnsErrorOnPanelRefusal(t *testing.T) {
	server := panelRefusalServer(t, http.StatusBadRequest, panelBadRequestBody)
	defer server.Close()

	client, err := NewClient(server.URL, "token", "")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	created, err := client.ForceCreateNewUser(context.Background(), 1, 123456789, 1000, 30, 0, "TX0001")
	if err == nil {
		t.Fatalf("ForceCreateNewUser() error = nil, want panel refusal error (created = %v)", created)
	}
	if created != nil {
		t.Fatalf("ForceCreateNewUser() user = %v, want nil", created)
	}
	if !strings.Contains(err.Error(), "Panel refused the request") {
		t.Fatalf("ForceCreateNewUser() error = %q, want panel message", err)
	}
}

func TestPanelResponseErrorDescribesTypedPanelErrors(t *testing.T) {
	badRequestErr := panelResponseError(&remapi.BadRequestError{
		Message:    "Panel refused the request",
		StatusCode: 400,
		Errors:     []remapi.ValidationError{{Message: "user is invalid"}},
	})
	if !strings.Contains(badRequestErr.Error(), "400") || !strings.Contains(badRequestErr.Error(), "user is invalid") {
		t.Fatalf("panelResponseError(BadRequestError) = %q, want status and validation message", badRequestErr.Error())
	}

	internalErr := panelResponseError(&remapi.InternalServerError{
		Message:   remapi.NewOptString("Panel exploded"),
		ErrorCode: remapi.NewOptString("E500"),
	})
	if !strings.Contains(internalErr.Error(), "Panel exploded") || !strings.Contains(internalErr.Error(), "E500") {
		t.Fatalf("panelResponseError(InternalServerError) = %q, want message and code", internalErr.Error())
	}

	unknownErr := panelResponseError(&remapi.UserResponse{})
	if unknownErr == nil || !strings.Contains(unknownErr.Error(), "unknown response type") {
		t.Fatalf("panelResponseError(unknown) = %v, want unknown response type error", unknownErr)
	}
}
