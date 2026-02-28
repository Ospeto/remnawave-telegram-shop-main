package remnawave

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/utils"
	"strconv"
	"strings"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
)

type Client struct {
	client *remapi.ClientExt
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
	return &Client{client: remapi.NewClientExt(api)}
}

func (r *Client) Ping(ctx context.Context) error {
	_, err := r.client.Users().GetAllUsers(ctx, 1, 0)
	return err
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
		return nil, err
	}

	usersResp, ok := resp.(*remapi.UsersResponse)
	if !ok {
		return nil, errors.New("unknown response type from Remnawave API")
	}

	return usersResp.GetResponse(), nil
}

func (r *Client) DecreaseSubscription(ctx context.Context, telegramId int64, trafficLimit int, days int) (*time.Time, error) {

	resp, err := r.client.Users().GetUserByTelegramId(ctx, strconv.FormatInt(telegramId, 10))
	if err != nil {
		return nil, err
	}

	usersResp, ok := resp.(*remapi.UsersResponse)
	if !ok {
		return nil, errors.New("unknown response type")
	}

	users := usersResp.GetResponse()
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
	resp, err := r.client.Users().GetUserByTelegramId(ctx, strconv.FormatInt(telegramId, 10))
	if err != nil {
		return nil, err
	}

	usersResp, ok := resp.(*remapi.UsersResponse)
	if !ok {
		return nil, errors.New("unknown response type")
	}

	users := usersResp.GetResponse()
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
		return nil, fmt.Errorf("failed to get user %s: %w", userUUID, err)
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

func (r *Client) updateUser(ctx context.Context, existingUser *remapi.User, trafficLimit int, days int) (*remapi.User, error) {

	newExpire := getNewExpire(days, existingUser.ExpireAt)

	resp, err := r.client.InternalSquad().GetInternalSquads(ctx)
	if err != nil {
		return nil, err
	}

	squads := resp.(*remapi.InternalSquadsResponse).GetResponse()

	selectedSquads := config.SquadUUIDs()

	squadId := make([]uuid.UUID, 0, len(selectedSquads))
	for _, squad := range squads.GetInternalSquads() {
		if selectedSquads != nil && len(selectedSquads) > 0 {
			if _, isExist := selectedSquads[squad.UUID]; !isExist {
				continue
			} else {
				squadId = append(squadId, squad.UUID)
			}
		} else {
			squadId = append(squadId, squad.UUID)
		}
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

	resp, err := r.client.InternalSquad().GetInternalSquads(ctx)
	if err != nil {
		return nil, err
	}

	squads := resp.(*remapi.InternalSquadsResponse).GetResponse()

	selectedSquads := config.SquadUUIDs()
	if isTrialUser {
		selectedSquads = config.TrialInternalSquads()
	}

	squadId := make([]uuid.UUID, 0, len(selectedSquads))
	for _, squad := range squads.GetInternalSquads() {
		if selectedSquads != nil && len(selectedSquads) > 0 {
			if _, isExist := selectedSquads[squad.UUID]; !isExist {
				continue
			} else {
				squadId = append(squadId, squad.UUID)
			}
		} else {
			squadId = append(squadId, squad.UUID)
		}
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
		return nil, err
	}
	slog.Info("created user", "telegramId", utils.MaskHalf(strconv.FormatInt(telegramId, 10)), "username", utils.MaskHalf(tgUsername), "key", username, "days", days)
	return &userCreate.(*remapi.UserResponse).Response, nil
}

// generateUsername creates a subscription key name.
// Format: wavy_{last4_txnID}_{telegramId}
// Examples: wavy_A1B2_987654321, wavy_0001_987654321
func generateUsername(tgUsername string, customerId int64, telegramId int64, keyIndex int, txnID string) string {
	var mid string
	if len(txnID) >= 4 {
		mid = txnID[len(txnID)-4:]
	} else if len(txnID) > 0 {
		mid = fmt.Sprintf("%04s", txnID)
	} else {
		mid = fmt.Sprintf("%04d", customerId%10000)
	}
	base := fmt.Sprintf("wavy_%s_%d", mid, telegramId)
	if keyIndex > 1 {
		base = fmt.Sprintf("%s_%d", base, keyIndex)
	}
	return base
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
