package remnawave

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/utils"
	"strconv"
	"strings"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

type Client struct {
	client     *remapi.ClientExt
	httpClient *http.Client
	baseURL    string
	token      string
}

type headerTransport struct {
	base    http.RoundTripper
	local   bool
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())

	if t.local {
		r.Header.Set("x-forwarded-for", "127.0.0.1")
		r.Header.Set("x-forwarded-proto", "https")
	}

	for key, value := range t.headers {
		r.Header.Set(key, value)
	}

	return t.base.RoundTrip(r)
}

func NewClient(baseURL, token, mode string) *Client {
	local := mode == "local"
	headers := config.RemnawaveHeaders()

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &headerTransport{
			base:    http.DefaultTransport,
			local:   local,
			headers: headers,
		},
	}

	api, err := remapi.NewClient(baseURL, remapi.StaticToken{Token: token}, remapi.WithClient(client))
	if err != nil {
		panic(err)
	}
	return &Client{
		client:     remapi.NewClientExt(api),
		httpClient: client,
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
	}
}

func (r *Client) Ping(ctx context.Context) error {
	_, err := r.client.Users().GetAllUsers(ctx, 1, 0)
	if err == nil {
		return nil
	}
	if !isDecodeResponseError(err) {
		return err
	}

	payload, statusCode, rawErr := r.doRawJSONRequest(ctx, http.MethodGet, "/api/users?size=1&start=0", nil)
	if rawErr != nil {
		return fmt.Errorf("ping fallback request: %w", rawErr)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("ping fallback status: %d", statusCode)
	}

	unwrapped, unwrapErr := unwrapResponseEnvelope(payload)
	if unwrapErr != nil {
		return fmt.Errorf("ping fallback decode: %w", unwrapErr)
	}

	response := asMap(unwrapped)
	if response == nil {
		return errors.New("ping fallback response is not an object")
	}
	if users, ok := response["users"]; !ok || asSlice(users) == nil {
		return errors.New("ping fallback response missing users list")
	}

	return nil
}

func (r *Client) GetUsers(ctx context.Context) (*[]remapi.User, error) {
	pager := remapi.NewPaginationHelper(250)
	users := make([]remapi.User, 0)

	for {
		resp, err := r.client.Users().GetAllUsers(ctx, float64(pager.Limit), float64(pager.Offset))
		if err != nil {
			return nil, err
		}

		response := resp.(*remapi.GetAllUsersResponseDto).GetResponse()
		users = append(users, response.Users...)

		if len(response.Users) < pager.Limit {
			break
		}

		if !pager.NextPage() {
			break
		}
	}

	return &users, nil
}

func (r *Client) GetUsersByTelegramId(ctx context.Context, telegramId int64) ([]remapi.User, error) {
	resp, err := r.client.Users().GetUserByTelegramId(ctx, strconv.FormatInt(telegramId, 10))
	if err != nil {
		if isDecodeResponseError(err) {
			slog.Warn("Remnawave decode failure on GetUserByTelegramId, falling back to permissive parser", "telegram_id", telegramId, "error", err)
			users, fallbackErr := r.getUsersByTelegramIDFallback(ctx, telegramId)
			if fallbackErr == nil {
				return users, nil
			}
			slog.Error("Remnawave fallback failed on GetUserByTelegramId", "telegram_id", telegramId, "error", fallbackErr)
		}
		return nil, err
	}

	usersResp, ok := resp.(*remapi.UsersResponse)
	if !ok {
		return nil, errors.New("unknown response type from Remnawave API")
	}

	return usersResp.GetResponse(), nil
}

func (r *Client) DecreaseSubscription(ctx context.Context, telegramId int64, trafficLimit int, days int) (*time.Time, error) {
	users, err := r.GetUsersByTelegramId(ctx, telegramId)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user with telegramId %d not found", telegramId)
	}

	var existingUser *remapi.User
	suffix := fmt.Sprintf("_%d", telegramId)

	for i := range users {
		if strings.Contains(users[i].Username, suffix) {
			existingUser = &users[i]
			break
		}
	}

	if existingUser == nil {
		existingUser = &users[0]
	}

	updated, err := r.updateUser(ctx, existingUser, trafficLimit, days)
	if err != nil {
		return nil, err
	}

	return &updated.ExpireAt, nil
}

func (r *Client) CreateOrUpdateUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool) (*remapi.User, error) {
	users, err := r.GetUsersByTelegramId(ctx, telegramId)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return r.createUser(ctx, customerId, telegramId, trafficLimit, days, isTrialUser, 0, "")
	}

	var existingUser *remapi.User
	suffix := fmt.Sprintf("_%d", telegramId)

	for i := range users {
		if strings.Contains(users[i].Username, suffix) {
			existingUser = &users[i]
			break
		}
	}

	if existingUser == nil {
		existingUser = &users[0]
	}

	return r.updateUser(ctx, existingUser, trafficLimit, days)
}

// ForceCreateNewUser always creates a brand new Remnawave user with a unique indexed username.
// keyIndex should be the count of existing keys + 1 (e.g. if user has 2 keys, pass 3).
// txnID is an optional transaction ID — last 4 chars will be appended to the username.
func (r *Client) ForceCreateNewUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, keyIndex int, txnID string) (*remapi.User, error) {
	return r.createUser(ctx, customerId, telegramId, trafficLimit, days, false, keyIndex, txnID)
}

// ExtendUser extends a specific Remnawave user by UUID.
// Days are added to the current expiry (or from now if expired).
// Traffic is accumulated (added to existing limit), not replaced.
// If additionalTraffic is 0 (unlimited plan), traffic limit stays 0 (unlimited).
func (r *Client) ExtendUser(ctx context.Context, userUUID uuid.UUID, additionalTraffic int, days int) (*remapi.User, error) {
	// Fetch the specific user by UUID
	resp, err := r.client.Users().GetUserByUuid(ctx, userUUID.String())
	if err != nil {
		if isDecodeResponseError(err) {
			slog.Warn("Remnawave decode failure on GetUserByUuid, falling back to permissive parser", "user_uuid", userUUID.String(), "error", err)
			fallbackUser, fallbackErr := r.getUserByUUIDFallback(ctx, userUUID)
			if fallbackErr == nil && fallbackUser != nil {
				resp = &remapi.UserResponse{Response: *fallbackUser}
			} else if fallbackErr != nil {
				return nil, fmt.Errorf("failed to get user %s (fallback): %w", userUUID, fallbackErr)
			} else {
				return nil, fmt.Errorf("failed to get user %s: not found after fallback", userUUID)
			}
		} else {
			return nil, fmt.Errorf("failed to get user %s: %w", userUUID, err)
		}
	}
	userResp, ok := resp.(*remapi.UserResponse)
	if !ok {
		return nil, errors.New("unknown response type from GetUserByUuid")
	}
	existingUser := &userResp.Response

	// Calculate new traffic limit
	newTraffic := additionalTraffic
	existingTraffic := 0
	if existingUser.TrafficLimitBytes.IsSet() {
		existingTraffic = existingUser.TrafficLimitBytes.Value
	}

	if existingTraffic == 0 {
		// Existing key is unlimited — stay unlimited regardless of extension plan
		newTraffic = 0
	} else if additionalTraffic == 0 {
		// Extending with an unlimited plan — upgrade to unlimited
		newTraffic = 0
	} else {
		// Both are limited — accumulate
		newTraffic = existingTraffic + additionalTraffic
	}

	return r.updateUser(ctx, existingUser, newTraffic, days)
}

func (r *Client) DeleteUser(ctx context.Context, userUUID uuid.UUID) error {
	_, err := r.client.Users().DeleteUser(ctx, userUUID.String())
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
		return nil
	}

	return err
}

func (r *Client) updateUser(ctx context.Context, existingUser *remapi.User, trafficLimit int, days int) (*remapi.User, error) {

	newExpire := getNewExpire(days, existingUser.ExpireAt)

	squadId, err := r.resolveInternalSquadUUIDs(ctx, false)
	if err != nil {
		return nil, err
	}

	userUpdate := &remapi.UpdateUserRequestDto{
		UUID:                 remapi.NewOptUUID(existingUser.UUID),
		ExpireAt:             remapi.NewOptDateTime(newExpire),
		Status:               remapi.NewOptUpdateUserRequestDtoStatus(remapi.UpdateUserRequestDtoStatusACTIVE),
		TrafficLimitBytes:    remapi.NewOptInt(trafficLimit),
		ActiveInternalSquads: squadId,
		TrafficLimitStrategy: remapi.NewOptUpdateUserRequestDtoTrafficLimitStrategy(getUpdateStrategy(config.TrafficLimitResetStrategy())),
	}

	externalSquad := config.ExternalSquadUUID()
	if externalSquad != uuid.Nil {
		userUpdate.ExternalSquadUuid = remapi.NewOptNilUUID(externalSquad)
	}

	tag := config.RemnawaveTag()
	if tag != "" {
		userUpdate.Tag = remapi.NewOptNilString(tag)
	}

	if desc, ok := ctx.Value("description").(string); ok && desc != "" {
		userUpdate.Description = remapi.NewOptNilString(desc)
	} else if ctx.Value("username") != nil {
		userUpdate.Description = remapi.NewOptNilString(ctx.Value("username").(string))
	}

	updateUser, err := r.client.Users().UpdateUser(ctx, userUpdate)
	if err != nil {
		if isDecodeResponseError(err) {
			slog.Warn("Remnawave decode failure on UpdateUser, attempting user lookup fallback", "user_uuid", existingUser.UUID.String(), "error", err)
			fallbackUser, fallbackErr := r.getUserByUUIDFallback(ctx, existingUser.UUID)
			if fallbackErr == nil && fallbackUser != nil {
				tgid, _ := fallbackUser.TelegramId.Get()
				slog.Info("updated user (fallback)", "telegramId", utils.MaskHalf(strconv.Itoa(tgid)), "username", utils.MaskHalf(fallbackUser.Username), "days", days)
				return fallbackUser, nil
			}
			if fallbackErr != nil {
				return nil, fmt.Errorf("update user decode fallback failed: %w", fallbackErr)
			}
		}
		return nil, err
	}
	if value, ok := updateUser.(*remapi.InternalServerError); ok {
		return nil, errors.New("error while updating user. message: " + value.GetMessage().Value + ". code: " + value.GetErrorCode().Value)
	}

	tgid, _ := existingUser.TelegramId.Get()
	slog.Info("updated user", "telegramId", utils.MaskHalf(strconv.Itoa(tgid)), "username", utils.MaskHalf(existingUser.Username), "days", days)
	return &updateUser.(*remapi.UserResponse).Response, nil
}

func (r *Client) createUser(ctx context.Context, customerId int64, telegramId int64, trafficLimit int, days int, isTrialUser bool, keyIndex int, txnID string) (*remapi.User, error) {
	expireAt := time.Now().UTC().AddDate(0, 0, days)

	// Build systematic username: {tg_username}_{last4_customerId}_{telegramId}[_{keyIndex}][_{last4_txn}]
	var tgUsername string
	if ctx.Value("username") != nil {
		tgUsername = ctx.Value("username").(string)
	}
	username := generateUsername(tgUsername, customerId, telegramId, keyIndex, txnID)

	// Idempotency check: if user already exists with this exact username, return it
	existingUsers, err := r.GetUsersByTelegramId(ctx, telegramId)
	if err == nil {
		for _, u := range existingUsers {
			if strings.EqualFold(u.Username, username) {
				slog.Info("Idempotency match: User already exists", "username", username, "telegramId", telegramId)
				return &u, nil
			}
		}
	} else {
		slog.Warn("Failed to check existing users for idempotency", "error", err)
	}

	squadId, err := r.resolveInternalSquadUUIDs(ctx, isTrialUser)
	if err != nil {
		return nil, err
	}

	externalSquad := config.ExternalSquadUUID()
	if isTrialUser {
		externalSquad = config.TrialExternalSquadUUID()
	}

	strategy := config.TrafficLimitResetStrategy()
	if isTrialUser {
		strategy = config.TrialTrafficLimitResetStrategy()
	}

	createUserRequestDto := remapi.CreateUserRequestDto{
		Username:             username,
		ActiveInternalSquads: squadId,
		Status:               remapi.NewOptCreateUserRequestDtoStatus(remapi.CreateUserRequestDtoStatusACTIVE),
		TelegramId:           remapi.NewOptNilInt(int(telegramId)),
		ExpireAt:             expireAt,
		TrafficLimitStrategy: remapi.NewOptCreateUserRequestDtoTrafficLimitStrategy(getCreateStrategy(strategy)),
		TrafficLimitBytes:    remapi.NewOptInt(trafficLimit),
	}
	if externalSquad != uuid.Nil {
		createUserRequestDto.ExternalSquadUuid = remapi.NewOptNilUUID(externalSquad)
	}
	tag := config.RemnawaveTag()
	if isTrialUser {
		tag = config.TrialRemnawaveTag()
	}
	if tag != "" {
		createUserRequestDto.Tag = remapi.NewOptNilString(tag)
	}

	if desc, ok := ctx.Value("description").(string); ok && desc != "" {
		createUserRequestDto.Description = remapi.NewOptString(desc)
	} else if tgUsername != "" {
		createUserRequestDto.Description = remapi.NewOptString(tgUsername)
	}

	userCreate, err := r.client.Users().CreateUser(ctx, &createUserRequestDto)
	if err != nil {
		if isDecodeResponseError(err) {
			slog.Warn("Remnawave decode failure on CreateUser, attempting lookup fallback", "telegram_id", telegramId, "username", username, "error", err)
			fallbackUsers, fallbackErr := r.getUsersByTelegramIDFallback(ctx, telegramId)
			if fallbackErr == nil {
				for i := range fallbackUsers {
					if strings.EqualFold(fallbackUsers[i].Username, username) {
						slog.Info("created user (fallback)", "telegramId", utils.MaskHalf(strconv.FormatInt(telegramId, 10)), "username", utils.MaskHalf(tgUsername), "key", username, "days", days)
						return &fallbackUsers[i], nil
					}
				}
			} else {
				return nil, fmt.Errorf("create user decode fallback failed: %w", fallbackErr)
			}
		}
		return nil, err
	}
	slog.Info("created user", "telegramId", utils.MaskHalf(strconv.FormatInt(telegramId, 10)), "username", utils.MaskHalf(tgUsername), "key", username, "days", days)
	return &userCreate.(*remapi.UserResponse).Response, nil
}

func isDecodeResponseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "decode response") {
		return true
	}
	if strings.Contains(msg, "unable to decode") || strings.Contains(msg, "decode field") {
		return true
	}
	return false
}

func (r *Client) doRawJSONRequest(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal body: %w", err)
		}
		requestBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, requestBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return payload, resp.StatusCode, nil
}

func unwrapResponseEnvelope(payload []byte) (any, error) {
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}

	if response, ok := envelope["response"]; ok {
		return response, nil
	}
	return envelope, nil
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func asSlice(value any) []any {
	if value == nil {
		return nil
	}
	if s, ok := value.([]any); ok {
		return s
	}
	return nil
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		return ""
	}
}

func asInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func asFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func asTime(value any) (time.Time, bool) {
	str := asString(value)
	if str == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, str); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func parseUserLoose(data any) (*remapi.User, error) {
	raw := asMap(data)
	if raw == nil {
		return nil, errors.New("user payload is not an object")
	}

	user := &remapi.User{}

	uuidStr := asString(raw["uuid"])
	if uuidStr == "" {
		uuidStr = asString(raw["UUID"])
	}
	if uuidStr != "" {
		if parsedUUID, err := uuid.Parse(uuidStr); err == nil {
			user.UUID = parsedUUID
		}
	}

	user.Username = asString(raw["username"])
	if user.Username == "" {
		user.Username = asString(raw["userName"])
	}

	user.SubscriptionUrl = asString(raw["subscriptionUrl"])
	if user.SubscriptionUrl == "" {
		user.SubscriptionUrl = asString(raw["subscription_url"])
	}

	if expireAt, ok := asTime(raw["expireAt"]); ok {
		user.ExpireAt = expireAt
	} else if expireAt, ok := asTime(raw["expire_at"]); ok {
		user.ExpireAt = expireAt
	}

	if telegramID, ok := asInt(raw["telegramId"]); ok {
		user.TelegramId = remapi.NewNilInt(telegramID)
	} else if telegramID, ok := asInt(raw["telegram_id"]); ok {
		user.TelegramId = remapi.NewNilInt(telegramID)
	}

	if trafficLimit, ok := asInt(raw["trafficLimitBytes"]); ok {
		user.TrafficLimitBytes = remapi.NewOptInt(trafficLimit)
	} else if trafficLimit, ok := asInt(raw["traffic_limit_bytes"]); ok {
		user.TrafficLimitBytes = remapi.NewOptInt(trafficLimit)
	}

	userTraffic := asMap(raw["userTraffic"])
	if userTraffic == nil {
		userTraffic = asMap(raw["user_traffic"])
	}
	if userTraffic != nil {
		if used, ok := asFloat(userTraffic["usedTrafficBytes"]); ok {
			user.UserTraffic.UsedTrafficBytes = used
		} else if used, ok := asFloat(userTraffic["used_traffic_bytes"]); ok {
			user.UserTraffic.UsedTrafficBytes = used
		}
		if lifetime, ok := asFloat(userTraffic["lifetimeUsedTrafficBytes"]); ok {
			user.UserTraffic.LifetimeUsedTrafficBytes = lifetime
		}
	}

	return user, nil
}

func collectConfiguredSquads(selected map[uuid.UUID]uuid.UUID) []uuid.UUID {
	if len(selected) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(selected))
	for key := range selected {
		ids = append(ids, key)
	}
	return ids
}

func parseInternalSquadUUIDsLoose(payload []byte) []uuid.UUID {
	unwrapped, err := unwrapResponseEnvelope(payload)
	if err != nil {
		return nil
	}

	candidates := asSlice(unwrapped)
	if candidates == nil {
		if m := asMap(unwrapped); m != nil {
			candidates = asSlice(m["internalSquads"])
			if candidates == nil {
				candidates = asSlice(m["internal_squads"])
			}
			if candidates == nil {
				candidates = asSlice(m["squads"])
			}
		}
	}

	if candidates == nil {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(candidates))
	for _, candidate := range candidates {
		squad := asMap(candidate)
		if squad == nil {
			continue
		}
		uuidStr := asString(squad["uuid"])
		if uuidStr == "" {
			continue
		}
		if parsedUUID, parseErr := uuid.Parse(uuidStr); parseErr == nil {
			ids = append(ids, parsedUUID)
		}
	}
	return ids
}

func (r *Client) resolveInternalSquadUUIDs(ctx context.Context, isTrialUser bool) ([]uuid.UUID, error) {
	selectedSquads := config.SquadUUIDs()
	if isTrialUser {
		selectedSquads = config.TrialInternalSquads()
	}

	// Prefer configured UUIDs directly; this avoids brittle decode failures from
	// list endpoints and still preserves explicit operator intent.
	if configured := collectConfiguredSquads(selectedSquads); len(configured) > 0 {
		return configured, nil
	}

	resp, err := r.client.InternalSquad().GetInternalSquads(ctx)
	if err != nil {
		if isDecodeResponseError(err) {
			slog.Warn("Remnawave decode failure on GetInternalSquads, falling back to permissive parser", "error", err)
			payload, statusCode, rawErr := r.doRawJSONRequest(ctx, http.MethodGet, "/api/internal-squads", nil)
			if rawErr != nil {
				return nil, fmt.Errorf("fallback get internal squads failed: %w", rawErr)
			}
			if statusCode >= 400 {
				return nil, fmt.Errorf("fallback get internal squads status %d: %s", statusCode, strings.TrimSpace(string(payload)))
			}
			if ids := parseInternalSquadUUIDsLoose(payload); len(ids) > 0 {
				return ids, nil
			}
		}
		return nil, err
	}

	internalSquadsResp, ok := resp.(*remapi.InternalSquadsResponse)
	if !ok {
		return nil, errors.New("unknown response type from GetInternalSquads")
	}

	squads := internalSquadsResp.GetResponse()
	ids := make([]uuid.UUID, 0, len(squads.GetInternalSquads()))
	for _, squad := range squads.GetInternalSquads() {
		ids = append(ids, squad.UUID)
	}
	return ids, nil
}

func (r *Client) getUsersByTelegramIDFallback(ctx context.Context, telegramID int64) ([]remapi.User, error) {
	path := "/api/users/by-telegram-id/" + url.PathEscape(strconv.FormatInt(telegramID, 10))
	payload, statusCode, err := r.doRawJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("fallback get users by telegram id status %d: %s", statusCode, strings.TrimSpace(string(payload)))
	}

	unwrapped, err := unwrapResponseEnvelope(payload)
	if err != nil {
		return nil, err
	}

	items := asSlice(unwrapped)
	if items == nil {
		if m := asMap(unwrapped); m != nil {
			items = asSlice(m["users"])
			if items == nil {
				if singleUser := asMap(m["user"]); singleUser != nil {
					items = []any{singleUser}
				}
			}
		}
	}
	if items == nil {
		return []remapi.User{}, nil
	}

	users := make([]remapi.User, 0, len(items))
	for _, item := range items {
		parsedUser, parseErr := parseUserLoose(item)
		if parseErr != nil {
			continue
		}
		users = append(users, *parsedUser)
	}
	return users, nil
}

func (r *Client) getUserByUUIDFallback(ctx context.Context, userUUID uuid.UUID) (*remapi.User, error) {
	path := "/api/users/" + url.PathEscape(userUUID.String())
	payload, statusCode, err := r.doRawJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if statusCode >= 400 {
		return nil, fmt.Errorf("fallback get user by uuid status %d: %s", statusCode, strings.TrimSpace(string(payload)))
	}

	unwrapped, err := unwrapResponseEnvelope(payload)
	if err != nil {
		return nil, err
	}

	user, err := parseUserLoose(unwrapped)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// generateUsername creates a subscription key name.
// Format: wavy_{txnToken}_{telegramId}[_keyIndex]
// txnToken keeps up to the last 8 alphanumeric characters to reduce collisions.
// Examples: wavy_AB12CD34_987654321, wavy_00010001_987654321
func generateUsername(tgUsername string, customerId int64, telegramId int64, keyIndex int, txnID string) string {
	mid := transactionToken(txnID, customerId)
	base := fmt.Sprintf("wavy_%s_%d", mid, telegramId)
	if keyIndex > 1 {
		base = fmt.Sprintf("%s_%d", base, keyIndex)
	}
	return base
}

func transactionToken(txnID string, customerID int64) string {
	normalized := strings.ToUpper(strings.TrimSpace(txnID))
	if normalized == "" {
		normalized = fmt.Sprintf("CUS%d", customerID)
	}

	var filtered strings.Builder
	for _, ch := range normalized {
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			filtered.WriteRune(ch)
		}
	}

	token := filtered.String()
	if token == "" {
		token = fmt.Sprintf("CUS%d", customerID)
	}
	if len(token) > 8 {
		token = token[len(token)-8:]
	}
	return token
}

// sanitizeUsername makes a Telegram username safe for use as a subscription key part.
func sanitizeUsername(username string) string {
	username = strings.ToLower(strings.TrimSpace(username))
	var b strings.Builder
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

func getNewExpire(daysToAdd int, currentExpire time.Time) time.Time {
	if daysToAdd <= 0 {
		if currentExpire.AddDate(0, 0, daysToAdd).Before(time.Now()) {
			return time.Now().UTC().AddDate(0, 0, 1)
		} else {
			return currentExpire.AddDate(0, 0, daysToAdd)
		}
	}

	if currentExpire.Before(time.Now().UTC()) || currentExpire.IsZero() {
		return time.Now().UTC().AddDate(0, 0, daysToAdd)
	}

	return currentExpire.AddDate(0, 0, daysToAdd)
}

func getCreateStrategy(s string) remapi.CreateUserRequestDtoTrafficLimitStrategy {
	switch s {
	case "DAY":
		return remapi.CreateUserRequestDtoTrafficLimitStrategyDAY
	case "WEEK":
		return remapi.CreateUserRequestDtoTrafficLimitStrategyWEEK
	case "NO_RESET":
		return remapi.CreateUserRequestDtoTrafficLimitStrategyNORESET
	default:
		return remapi.CreateUserRequestDtoTrafficLimitStrategyMONTH
	}
}

func getUpdateStrategy(s string) remapi.UpdateUserRequestDtoTrafficLimitStrategy {
	switch s {
	case "DAY":
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyDAY
	case "WEEK":
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyWEEK
	case "NO_RESET":
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyNORESET
	default:
		return remapi.UpdateUserRequestDtoTrafficLimitStrategyMONTH
	}
}
