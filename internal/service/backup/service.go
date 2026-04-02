package backup

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"remnawave-tg-shop-bot/internal/database"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
)

const (
	keyBackupEnabled           = "backup_enabled"
	keyBackupScheduleTime      = "backup_schedule_time"
	keyBackupSendToTelegram    = "backup_send_to_telegram"
	keyBackupRestoreEnabled    = "backup_restore_enabled"
	keyBackupLastSuccessAt     = "backup_last_success_at"
	keyBackupLastFile          = "backup_last_file"
	keyBackupLastSizeBytes     = "backup_last_size_bytes"
	keyBackupLastError         = "backup_last_error"
	keyBackupLastScheduledDate = "backup_last_scheduled_date"
	keyBackupScheduledClaim    = "backup_scheduled_claim"
)

var ErrOperationInProgress = errors.New("backup or restore already in progress")

type TelegramClient interface {
	SendMessage(ctx context.Context, params *bot.SendMessageParams) (*models.Message, error)
	SendDocument(ctx context.Context, params *bot.SendDocumentParams) (*models.Message, error)
}

type AppConfigStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	CompareAndSwap(ctx context.Context, key string, expected string, value string) (bool, error)
}

type Options struct {
	DatabaseURL         string
	BackupDir           string
	Timezone            string
	DefaultScheduleTime string
	Enabled             bool
	SendToTelegram      bool
	RestoreEnabled      bool
	RetentionDays       int
	MaxLocalFiles       int
	ConfirmTTL          time.Duration
	JobTimeout          time.Duration
	RestoreTimeout      time.Duration
}

type BackupFile struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

type BackupResult struct {
	File       BackupFile
	Reason     string
	Pruned     []string
	FinishedAt time.Time
}

type Status struct {
	Enabled          bool
	SendToTelegram   bool
	RestoreEnabled   bool
	ScheduleTime     string
	Timezone         string
	NextRunAt        time.Time
	LastSuccessAt    *time.Time
	LastFile         string
	LastSizeBytes    int64
	LastError        string
	BackupCount      int
	OperationRunning bool
}

type PendingRestore struct {
	Token     string
	File      BackupFile
	ExpiresAt time.Time
}

type RestoreResult struct {
	Target       BackupFile
	SafetyBackup *BackupFile
	RestoredAt   time.Time
}

type dumpFunc func(ctx context.Context, databaseURL string, output io.Writer) error
type restoreFunc func(ctx context.Context, databaseURL string, input io.Reader) error

type Service struct {
	appConfig AppConfigStore
	opts      Options

	mu             sync.Mutex
	operationLabel string
	pendingRestore *PendingRestore
	nowFn          func() time.Time
	dumpFn         dumpFunc
	restoreFn      restoreFunc
}

func NewService(appConfig AppConfigStore, opts Options) *Service {
	if opts.BackupDir == "" {
		opts.BackupDir = "/backups"
	}
	if opts.Timezone == "" {
		opts.Timezone = "Asia/Rangoon"
	}
	if opts.DefaultScheduleTime == "" {
		opts.DefaultScheduleTime = "00:10"
	}
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = 7
	}
	if opts.MaxLocalFiles <= 0 {
		opts.MaxLocalFiles = 7
	}
	if opts.ConfirmTTL <= 0 {
		opts.ConfirmTTL = 10 * time.Minute
	}
	if opts.JobTimeout <= 0 {
		opts.JobTimeout = 15 * time.Minute
	}
	if opts.RestoreTimeout <= 0 {
		opts.RestoreTimeout = 30 * time.Minute
	}

	return &Service{
		appConfig: appConfig,
		opts:      opts,
		nowFn:     time.Now,
		dumpFn:    defaultDumpDatabase,
		restoreFn: defaultRestoreDatabase,
	}
}

func (s *Service) CreateBackup(ctx context.Context, reason string) (*BackupResult, error) {
	var result *BackupResult
	err := s.withOperation("backup", func() error {
		created, err := s.createBackupLocked(ctx, reason)
		if err != nil {
			return err
		}
		result = created
		return nil
	})
	return result, err
}

func (s *Service) RunScheduledBackupIfDue(ctx context.Context, tg TelegramClient, chatID int64) error {
	settings, err := s.loadRuntimeSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}

	now := s.nowFn().In(settings.Location)
	scheduledAt, err := scheduledRunTime(now, settings.ScheduleTime, settings.Location)
	if err != nil {
		return err
	}
	if now.Before(scheduledAt) {
		return nil
	}

	todayKey := now.Format("2006-01-02")
	lastRunDate, _ := s.getValue(ctx, keyBackupLastScheduledDate)
	if lastRunDate == todayKey {
		return nil
	}

	claim, claimed, err := s.tryAcquireScheduledBackupClaim(ctx, todayKey, now.UTC())
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	releaseClaim := true
	defer func() {
		if !releaseClaim {
			return
		}
		s.releaseScheduledBackupClaim(context.WithoutCancel(ctx), claim)
	}()

	result, backupErr := s.CreateBackup(ctx, "scheduled")
	if backupErr != nil {
		s.notifyFailure(ctx, tg, chatID, "Scheduled backup failed", backupErr)
		return backupErr
	}

	if settings.SendToTelegram {
		if err := s.SendBackupDocument(ctx, tg, chatID, result, "Scheduled backup complete"); err != nil {
			s.notifyFailure(ctx, tg, chatID, "Scheduled backup upload failed", err)
			return err
		}
	} else {
		_, _ = tg.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Scheduled backup complete: %s", result.File.Name),
		})
	}

	if err := s.completeScheduledBackup(context.WithoutCancel(ctx), claim, todayKey); err != nil {
		releaseClaim = false
		return err
	}
	releaseClaim = false
	return nil
}

func (s *Service) SendBackupDocument(ctx context.Context, tg TelegramClient, chatID int64, result *BackupResult, prefix string) error {
	file, err := os.Open(result.File.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	caption := fmt.Sprintf("%s\n\nFile: <code>%s</code>\nSize: %s\nReason: %s",
		prefix,
		result.File.Name,
		humanSize(result.File.Size),
		result.Reason,
	)
	_, err = tg.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID:    chatID,
		Document:  &models.InputFileUpload{Filename: result.File.Name, Data: file},
		Caption:   caption,
		ParseMode: models.ParseModeHTML,
	})
	if err == nil {
		if setErr := s.setValue(ctx, keyBackupLastError, ""); setErr != nil {
			slog.Warn("backup: failed to clear last error", "error", setErr)
		}
	}
	return err
}

func (s *Service) Status(ctx context.Context) (*Status, error) {
	settings, err := s.loadRuntimeSettings(ctx)
	if err != nil {
		return nil, err
	}

	backups, err := s.ListBackups()
	if err != nil {
		return nil, err
	}

	status := &Status{
		Enabled:        settings.Enabled,
		SendToTelegram: settings.SendToTelegram,
		RestoreEnabled: settings.RestoreEnabled,
		ScheduleTime:   settings.ScheduleTime,
		Timezone:       s.opts.Timezone,
		NextRunAt:      nextRunAt(settings.Location, settings.ScheduleTime, s.nowFn()),
		BackupCount:    len(backups),
		LastError:      strings.TrimSpace(s.mustGetValue(ctx, keyBackupLastError)),
		LastFile:       strings.TrimSpace(s.mustGetValue(ctx, keyBackupLastFile)),
	}

	if sizeStr := strings.TrimSpace(s.mustGetValue(ctx, keyBackupLastSizeBytes)); sizeStr != "" {
		if size, parseErr := strconv.ParseInt(sizeStr, 10, 64); parseErr == nil {
			status.LastSizeBytes = size
		}
	}

	if ts := strings.TrimSpace(s.mustGetValue(ctx, keyBackupLastSuccessAt)); ts != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, ts); parseErr == nil {
			status.LastSuccessAt = &parsed
		}
	}

	s.mu.Lock()
	status.OperationRunning = s.operationLabel != ""
	s.mu.Unlock()

	if status.LastFile == "" && len(backups) > 0 {
		status.LastFile = backups[0].Name
		status.LastSizeBytes = backups[0].Size
		mod := backups[0].ModTime
		status.LastSuccessAt = &mod
	}

	return status, nil
}

func (s *Service) ListBackups() ([]BackupFile, error) {
	if err := os.MkdirAll(s.opts.BackupDir, 0o700); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.opts.BackupDir)
	if err != nil {
		return nil, err
	}

	backups := make([]BackupFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "db_") || !strings.HasSuffix(name, ".sql.gz") {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil, statErr
		}
		backups = append(backups, BackupFile{
			Name:    name,
			Path:    filepath.Join(s.opts.BackupDir, name),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.After(backups[j].ModTime)
	})
	return backups, nil
}

func (s *Service) SetEnabled(ctx context.Context, enabled bool) error {
	return s.setValue(ctx, keyBackupEnabled, strconv.FormatBool(enabled))
}

func (s *Service) SetScheduleTime(ctx context.Context, value string) error {
	if _, _, err := parseScheduleTime(value); err != nil {
		return err
	}
	return s.setValue(ctx, keyBackupScheduleTime, value)
}

func (s *Service) PrepareRestoreLatest(ctx context.Context) (*PendingRestore, error) {
	backups, err := s.ListBackups()
	if err != nil {
		return nil, err
	}
	if len(backups) == 0 {
		return nil, errors.New("no local backups available")
	}
	return s.prepareRestore(ctx, backups[0])
}

func (s *Service) PrepareRestoreFile(ctx context.Context, fileName string) (*PendingRestore, error) {
	backups, err := s.ListBackups()
	if err != nil {
		return nil, err
	}
	for _, backup := range backups {
		if backup.Name == fileName {
			return s.prepareRestore(ctx, backup)
		}
	}
	return nil, fmt.Errorf("backup %q not found", fileName)
}

func (s *Service) CancelPendingRestore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingRestore = nil
}

func (s *Service) ConfirmRestore(ctx context.Context, token string) (*RestoreResult, error) {
	settings, err := s.loadRuntimeSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.RestoreEnabled {
		return nil, errors.New("restore is disabled")
	}

	var pending *PendingRestore
	s.mu.Lock()
	if s.pendingRestore == nil {
		s.mu.Unlock()
		return nil, errors.New("no restore is pending")
	}
	if s.pendingRestore.ExpiresAt.Before(s.nowFn()) {
		s.pendingRestore = nil
		s.mu.Unlock()
		return nil, errors.New("restore confirmation token expired")
	}
	if s.pendingRestore.Token != token {
		s.mu.Unlock()
		return nil, errors.New("invalid restore confirmation token")
	}
	pending = s.pendingRestore
	s.mu.Unlock()

	var result *RestoreResult
	err = s.withOperation("restore", func() error {
		restoreCtx, cancel := context.WithTimeout(ctx, s.opts.RestoreTimeout)
		defer cancel()

		safety, backupErr := s.createBackupLocked(restoreCtx, "pre_restore")
		if backupErr != nil {
			return fmt.Errorf("failed to create pre-restore safety backup: %w", backupErr)
		}

		file, openErr := os.Open(pending.File.Path)
		if openErr != nil {
			return openErr
		}
		defer file.Close()

		reader, gzipErr := gzip.NewReader(file)
		if gzipErr != nil {
			return gzipErr
		}
		defer reader.Close()

		if restoreErr := s.restoreFn(restoreCtx, s.opts.DatabaseURL, reader); restoreErr != nil {
			return restoreErr
		}

		result = &RestoreResult{
			Target:       pending.File,
			SafetyBackup: &safety.File,
			RestoredAt:   s.nowFn(),
		}
		return nil
	})
	if err != nil {
		if setErr := s.setValue(ctx, keyBackupLastError, err.Error()); setErr != nil {
			slog.Warn("backup: failed to persist restore error", "error", setErr)
		}
		return nil, err
	}

	s.mu.Lock()
	s.pendingRestore = nil
	s.mu.Unlock()
	if setErr := s.setValue(ctx, keyBackupLastError, ""); setErr != nil {
		slog.Warn("backup: failed to clear restore error", "error", setErr)
	}
	return result, nil
}

func (s *Service) withOperation(label string, fn func() error) error {
	s.mu.Lock()
	if s.operationLabel != "" {
		s.mu.Unlock()
		return ErrOperationInProgress
	}
	s.operationLabel = label
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.operationLabel = ""
		s.mu.Unlock()
	}()

	return fn()
}

func (s *Service) createBackupLocked(ctx context.Context, reason string) (*BackupResult, error) {
	if err := os.MkdirAll(s.opts.BackupDir, 0o700); err != nil {
		return nil, err
	}

	backupCtx, cancel := context.WithTimeout(ctx, s.opts.JobTimeout)
	defer cancel()

	name := fmt.Sprintf("db_%s.sql.gz", s.nowFn().UTC().Format("20060102_150405"))
	path := filepath.Join(s.opts.BackupDir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}

	gzipWriter := gzip.NewWriter(file)
	dumpErr := s.dumpFn(backupCtx, s.opts.DatabaseURL, gzipWriter)
	closeErr := gzipWriter.Close()
	fileCloseErr := file.Close()
	if dumpErr != nil {
		_ = os.Remove(path)
		_ = s.setValue(ctx, keyBackupLastError, dumpErr.Error())
		return nil, dumpErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		_ = s.setValue(ctx, keyBackupLastError, closeErr.Error())
		return nil, closeErr
	}
	if fileCloseErr != nil {
		_ = os.Remove(path)
		_ = s.setValue(ctx, keyBackupLastError, fileCloseErr.Error())
		return nil, fileCloseErr
	}

	info, err := os.Stat(path)
	if err != nil {
		_ = s.setValue(ctx, keyBackupLastError, err.Error())
		return nil, err
	}

	pruned, err := s.applyRetention()
	if err != nil {
		_ = s.setValue(ctx, keyBackupLastError, err.Error())
		return nil, err
	}

	finishedAt := s.nowFn()
	if err := s.setValue(ctx, keyBackupLastSuccessAt, finishedAt.Format(time.RFC3339)); err != nil {
		slog.Warn("backup: failed to persist success timestamp", "error", err)
	}
	if err := s.setValue(ctx, keyBackupLastFile, name); err != nil {
		slog.Warn("backup: failed to persist last file", "error", err)
	}
	if err := s.setValue(ctx, keyBackupLastSizeBytes, strconv.FormatInt(info.Size(), 10)); err != nil {
		slog.Warn("backup: failed to persist last size", "error", err)
	}
	if err := s.setValue(ctx, keyBackupLastError, ""); err != nil {
		slog.Warn("backup: failed to clear last error", "error", err)
	}

	return &BackupResult{
		File: BackupFile{
			Name:    name,
			Path:    path,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		},
		Reason:     reason,
		Pruned:     pruned,
		FinishedAt: finishedAt,
	}, nil
}

func (s *Service) applyRetention() ([]string, error) {
	backups, err := s.ListBackups()
	if err != nil {
		return nil, err
	}

	pruned := make([]string, 0)
	cutoff := s.nowFn().AddDate(0, 0, -s.opts.RetentionDays)
	kept := make([]BackupFile, 0, len(backups))
	for _, backup := range backups {
		if s.opts.RetentionDays > 0 && backup.ModTime.Before(cutoff) {
			if err := os.Remove(backup.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return pruned, err
			}
			pruned = append(pruned, backup.Name)
			continue
		}
		kept = append(kept, backup)
	}

	if s.opts.MaxLocalFiles > 0 && len(kept) > s.opts.MaxLocalFiles {
		for _, backup := range kept[s.opts.MaxLocalFiles:] {
			if err := os.Remove(backup.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return pruned, err
			}
			pruned = append(pruned, backup.Name)
		}
	}
	return pruned, nil
}

func (s *Service) prepareRestore(ctx context.Context, file BackupFile) (*PendingRestore, error) {
	settings, err := s.loadRuntimeSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.RestoreEnabled {
		return nil, errors.New("restore is disabled")
	}

	pending := &PendingRestore{
		Token:     strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:8],
		File:      file,
		ExpiresAt: s.nowFn().Add(s.opts.ConfirmTTL),
	}

	s.mu.Lock()
	s.pendingRestore = pending
	s.mu.Unlock()
	return pending, nil
}

type runtimeSettings struct {
	Enabled        bool
	SendToTelegram bool
	RestoreEnabled bool
	ScheduleTime   string
	Location       *time.Location
}

type scheduledBackupClaim struct {
	DateKey   string
	Owner     string
	ExpiresAt time.Time
}

func (c scheduledBackupClaim) encode() string {
	return fmt.Sprintf("%s|%s|%s", c.DateKey, c.Owner, c.ExpiresAt.UTC().Format(time.RFC3339Nano))
}

func (s *Service) loadRuntimeSettings(ctx context.Context) (*runtimeSettings, error) {
	location, err := time.LoadLocation(s.opts.Timezone)
	if err != nil {
		return nil, err
	}

	schedule := strings.TrimSpace(s.mustGetValue(ctx, keyBackupScheduleTime))
	if schedule == "" {
		schedule = s.opts.DefaultScheduleTime
	}
	if _, _, err := parseScheduleTime(schedule); err != nil {
		return nil, err
	}

	return &runtimeSettings{
		Enabled:        s.getBoolValue(ctx, keyBackupEnabled, s.opts.Enabled),
		SendToTelegram: s.getBoolValue(ctx, keyBackupSendToTelegram, s.opts.SendToTelegram),
		RestoreEnabled: s.getBoolValue(ctx, keyBackupRestoreEnabled, s.opts.RestoreEnabled),
		ScheduleTime:   schedule,
		Location:       location,
	}, nil
}

func (s *Service) getBoolValue(ctx context.Context, key string, fallback bool) bool {
	value := strings.TrimSpace(s.mustGetValue(ctx, key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (s *Service) mustGetValue(ctx context.Context, key string) string {
	value, _ := s.getValue(ctx, key)
	return value
}

func (s *Service) getValue(ctx context.Context, key string) (string, error) {
	if s.appConfig == nil {
		return "", nil
	}
	return s.appConfig.Get(ctx, key)
}

func (s *Service) setValue(ctx context.Context, key, value string) error {
	if s.appConfig == nil {
		return nil
	}
	return s.appConfig.Set(ctx, key, value)
}

func (s *Service) compareAndSwapValue(ctx context.Context, key string, expected string, next string) (bool, error) {
	if s.appConfig == nil {
		return true, nil
	}
	return s.appConfig.CompareAndSwap(ctx, key, expected, next)
}

func (s *Service) scheduledClaimLeaseTTL() time.Duration {
	ttl := s.opts.JobTimeout + 5*time.Minute
	if ttl < 5*time.Minute {
		return 5 * time.Minute
	}
	return ttl
}

func (s *Service) tryAcquireScheduledBackupClaim(ctx context.Context, todayKey string, now time.Time) (*scheduledBackupClaim, bool, error) {
	claim := &scheduledBackupClaim{
		DateKey:   todayKey,
		Owner:     uuid.NewString(),
		ExpiresAt: now.Add(s.scheduledClaimLeaseTTL()),
	}

	for attempts := 0; attempts < 5; attempts++ {
		current, _ := s.getValue(ctx, keyBackupScheduledClaim)
		current = strings.TrimSpace(current)
		if current != "" {
			existing, err := parseScheduledBackupClaim(current)
			if err == nil && existing.DateKey == todayKey && existing.ExpiresAt.After(now) {
				return nil, false, nil
			}
		}

		swapped, err := s.compareAndSwapValue(ctx, keyBackupScheduledClaim, current, claim.encode())
		if err != nil {
			return nil, false, fmt.Errorf("failed to claim scheduled backup: %w", err)
		}
		if swapped {
			return claim, true, nil
		}
	}

	return nil, false, errors.New("failed to acquire scheduled backup claim after concurrent updates")
}

func (s *Service) releaseScheduledBackupClaim(ctx context.Context, claim *scheduledBackupClaim) {
	if claim == nil {
		return
	}
	released, err := s.compareAndSwapValue(ctx, keyBackupScheduledClaim, claim.encode(), "")
	if err != nil {
		slog.Warn("backup: failed to release scheduled claim", "error", err)
		return
	}
	if !released {
		slog.Warn("backup: scheduled claim changed before release", "date", claim.DateKey, "owner", claim.Owner)
	}
}

func (s *Service) completeScheduledBackup(ctx context.Context, claim *scheduledBackupClaim, todayKey string) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := s.setValue(persistCtx, keyBackupLastScheduledDate, todayKey); err != nil {
		_ = s.setValue(ctx, keyBackupLastError, err.Error())
		return fmt.Errorf("failed to persist scheduled backup completion: %w", err)
	}
	s.releaseScheduledBackupClaim(persistCtx, claim)
	return nil
}

func parseScheduledBackupClaim(value string) (*scheduledBackupClaim, error) {
	parts := strings.Split(value, "|")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid scheduled backup claim %q", value)
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid scheduled backup claim expiry %q: %w", value, err)
	}

	return &scheduledBackupClaim{
		DateKey:   parts[0],
		Owner:     parts[1],
		ExpiresAt: expiresAt,
	}, nil
}

func (s *Service) notifyFailure(ctx context.Context, tg TelegramClient, chatID int64, prefix string, err error) {
	if tg == nil {
		return
	}
	if _, sendErr := tg.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("%s: %v", prefix, err),
	}); sendErr != nil {
		slog.Error("backup: failed to send failure notification", "error", sendErr)
	}
}

func nextRunAt(location *time.Location, schedule string, now time.Time) time.Time {
	hour, minute, err := parseScheduleTime(schedule)
	if err != nil {
		return now
	}
	localNow := now.In(location)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, location)
	if !next.After(localNow) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func scheduledRunTime(now time.Time, schedule string, location *time.Location) (time.Time, error) {
	hour, minute, err := parseScheduleTime(schedule)
	if err != nil {
		return time.Time{}, err
	}

	localNow := now.In(location)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, location), nil
}

func parseScheduleTime(value string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid schedule time %q, expected HH:MM", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("invalid schedule hour in %q", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid schedule minute in %q", value)
	}
	return hour, minute, nil
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func defaultDumpDatabase(ctx context.Context, databaseURL string, output io.Writer) error {
	cmd := exec.CommandContext(ctx, "pg_dump",
		"--dbname", databaseURL,
		"--clean",
		"--if-exists",
		"--no-owner",
		"--no-privileges",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return wrapExecError("pg_dump", err)
	}
	if _, err := io.Copy(output, stdout); err != nil {
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("pg_dump failed: %s", strings.TrimSpace(stderr.String()))
		}
		return wrapExecError("pg_dump", err)
	}
	return nil
}

func defaultRestoreDatabase(ctx context.Context, databaseURL string, input io.Reader) error {
	cmd := exec.CommandContext(ctx, "psql",
		databaseURL,
		"-v", "ON_ERROR_STOP=1",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return wrapExecError("psql", err)
	}

	copyErrCh := make(chan error, 1)
	go func() {
		_, err := io.Copy(stdin, input)
		_ = stdin.Close()
		copyErrCh <- err
	}()

	if err := <-copyErrCh; err != nil {
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("psql restore failed: %s", strings.TrimSpace(stderr.String()))
		}
		return wrapExecError("psql", err)
	}
	return nil
}

func wrapExecError(binary string, err error) error {
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return fmt.Errorf("%s is unavailable in the runtime image: %w", binary, err)
	}
	return err
}

var _ AppConfigStore = (*database.AppConfigRepository)(nil)
