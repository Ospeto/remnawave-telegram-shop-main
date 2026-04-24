package sync

import (
	"context"
	"log/slog"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/remnawave"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

type remnawaveUserLister interface {
	GetUsers(ctx context.Context) (*[]remapi.User, error)
}

type customerStore interface {
	FindByTelegramIds(ctx context.Context, telegramIDs []int64) ([]database.Customer, error)
	BeginTx(ctx context.Context) (pgx.Tx, error)
	CreateBatchTx(ctx context.Context, tx pgx.Tx, customers []database.Customer) error
	UpdateBatchTx(ctx context.Context, tx pgx.Tx, customers []database.Customer) error
}

type subscriptionKeyStore interface {
	MarkMissingRemoteKeysDeleted(ctx context.Context, remoteUUIDs []uuid.UUID) (int64, error)
}

type SyncService struct {
	client             remnawaveUserLister
	customerRepository customerStore
	subscriptionKeys   subscriptionKeyStore
}

func NewSyncService(client *remnawave.Client, customerRepository *database.CustomerRepository, subscriptionKeys *database.SubscriptionKeyRepository) *SyncService {
	return &SyncService{
		client:             client,
		customerRepository: customerRepository,
		subscriptionKeys:   subscriptionKeys,
	}
}

func (s SyncService) Sync() {
	slog.Info("Starting sync")
	ctx := context.Background()
	var telegramIDs []int64
	telegramIDsSet := make(map[int64]int64)
	var mappedUsers []database.Customer
	users, err := s.client.GetUsers(ctx)
	if err != nil {
		slog.Error("Error while getting users from remnawave", "error", err)
		return
	}
	if users == nil {
		slog.Warn("Remnawave returned no user list during sync")
		emptyUsers := []remapi.User{}
		users = &emptyUsers
	}

	s.markMissingRemoteKeysDeleted(ctx, *users)

	if len(*users) == 0 {
		slog.Warn("No users found in remnawave")
		slog.Info("Synchronization completed")
		return
	}

	for _, user := range *users {
		if user.TelegramId.Null {
			continue
		}
		if _, exists := telegramIDsSet[int64(user.TelegramId.Value)]; exists {
			continue
		}

		telegramIDsSet[int64(user.TelegramId.Value)] = int64(user.TelegramId.Value)

		telegramIDs = append(telegramIDs, int64(user.TelegramId.Value))

		expireAt := user.ExpireAt
		subscriptionURL := user.SubscriptionUrl
		mappedUsers = append(mappedUsers, database.Customer{
			TelegramID:       int64(user.TelegramId.Value),
			ExpireAt:         &expireAt,
			SubscriptionLink: &subscriptionURL,
			Language:         config.DefaultLanguage(),
		})
	}

	if s.customerRepository == nil {
		slog.Error("Customer repository is unavailable during sync")
		return
	}

	existingCustomers, err := s.customerRepository.FindByTelegramIds(ctx, telegramIDs)
	if err != nil {
		slog.Error("Error while searching users by telegram ids")
		return
	}
	existingMap := make(map[int64]database.Customer)
	for _, cust := range existingCustomers {
		existingMap[cust.TelegramID] = cust
	}

	var toCreate []database.Customer
	var toUpdate []database.Customer

	for _, cust := range mappedUsers {
		if existing, found := existingMap[cust.TelegramID]; found {
			cust.ID = existing.ID
			cust.CreatedAt = existing.CreatedAt
			cust.Language = existing.Language
			toUpdate = append(toUpdate, cust)
		} else {
			toCreate = append(toCreate, cust)
		}
	}

	// WARNING: We previously deleted customers here if they weren't in the panel.
	// This is highly destructive because a Customer record holds their Wallet Balance.
	// We MUST NOT delete the Customer just because they currently have no active keys.
	tx, err := s.customerRepository.BeginTx(ctx)
	if err != nil {
		slog.Error("Error while starting sync transaction", "error", err)
		return
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if len(toCreate) > 0 {
		if err := s.customerRepository.CreateBatchTx(ctx, tx, toCreate); err != nil {
			slog.Error("Error while creating users")
			return
		} else {
			slog.Info("Created clients", "count", len(toCreate))
		}
	}

	if len(toUpdate) > 0 {
		if err := s.customerRepository.UpdateBatchTx(ctx, tx, toUpdate); err != nil {
			slog.Error("Error while updating users")
			return
		} else {
			slog.Info("Updated clients", "count", len(toUpdate))
		}
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("Error while committing sync transaction", "error", err)
		return
	}
	slog.Info("Synchronization completed")
}

func (s SyncService) markMissingRemoteKeysDeleted(ctx context.Context, users []remapi.User) {
	if s.subscriptionKeys == nil {
		return
	}

	remoteUUIDs := make([]uuid.UUID, 0, len(users))
	for _, user := range users {
		if user.UUID == uuid.Nil {
			continue
		}
		remoteUUIDs = append(remoteUUIDs, user.UUID)
	}
	if len(users) > 0 && len(remoteUUIDs) == 0 {
		slog.Error("Remnawave sync returned users without UUIDs; skipping deleted key reconciliation")
		return
	}

	deletedCount, err := s.subscriptionKeys.MarkMissingRemoteKeysDeleted(ctx, remoteUUIDs)
	if err != nil {
		slog.Error("Error while marking missing remote subscription keys deleted", "error", err)
		return
	}
	if deletedCount > 0 {
		slog.Info("Marked local subscription keys deleted after Remnawave sync", "count", deletedCount)
	}
}
