package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type fakeConfigStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (f *fakeConfigStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value, ok := f.values[key]; ok {
		return value, nil
	}
	return "", errors.New("missing")
}

func (f *fakeConfigStore) Set(_ context.Context, key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = value
	return nil
}

func (f *fakeConfigStore) CompareAndSwap(_ context.Context, key string, expected string, value string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	current := f.values[key]
	if current != expected {
		return false, nil
	}
	f.values[key] = value
	return true, nil
}

func TestCreateBackupAndRetention(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{}}
	svc := NewService(store, Options{
		DatabaseURL:         "postgres://example",
		BackupDir:           dir,
		Timezone:            "Asia/Rangoon",
		DefaultScheduleTime: "00:10",
		RetentionDays:       30,
		MaxLocalFiles:       2,
	})

	times := []time.Time{
		time.Date(2026, 4, 1, 0, 10, 0, 0, time.UTC),
		time.Date(2026, 4, 1, 0, 10, 1, 0, time.UTC),
		time.Date(2026, 4, 2, 0, 10, 0, 0, time.UTC),
		time.Date(2026, 4, 2, 0, 10, 1, 0, time.UTC),
		time.Date(2026, 4, 3, 0, 10, 0, 0, time.UTC),
		time.Date(2026, 4, 3, 0, 10, 1, 0, time.UTC),
	}
	index := 0
	svc.nowFn = func() time.Time {
		current := times[index]
		if index < len(times)-1 {
			index++
		}
		return current
	}
	svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
		_, err := output.Write([]byte("SELECT 1;"))
		return err
	}

	for i := 0; i < 3; i++ {
		if _, err := svc.CreateBackup(context.Background(), "test"); err != nil {
			t.Fatalf("CreateBackup() error = %v", err)
		}
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups after retention, got %d", len(backups))
	}
	if strings.Contains(backups[0].Name, "20260401") || strings.Contains(backups[1].Name, "20260401") {
		t.Fatalf("oldest backup should have been pruned: %+v", backups)
	}
}

func TestPrepareAndConfirmRestore(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{
		keyBackupRestoreEnabled: "true",
	}}
	svc := NewService(store, Options{
		DatabaseURL:         "postgres://example",
		BackupDir:           dir,
		Timezone:            "Asia/Rangoon",
		DefaultScheduleTime: "00:10",
		RestoreEnabled:      true,
	})

	times := []time.Time{
		time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 1, 1, 0, 1, 0, time.UTC),
		time.Date(2026, 4, 1, 1, 5, 0, 0, time.UTC),
		time.Date(2026, 4, 1, 1, 5, 1, 0, time.UTC),
	}
	index := 0
	svc.nowFn = func() time.Time {
		current := times[index]
		if index < len(times)-1 {
			index++
		}
		return current
	}
	svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
		_, err := output.Write([]byte("SELECT 42;"))
		return err
	}

	if _, err := svc.CreateBackup(context.Background(), "seed"); err != nil {
		t.Fatalf("CreateBackup() seed error = %v", err)
	}

	var restored bytes.Buffer
	svc.restoreFn = func(_ context.Context, _ string, input io.Reader) error {
		_, err := io.Copy(&restored, input)
		return err
	}

	pending, err := svc.PrepareRestoreLatest(context.Background())
	if err != nil {
		t.Fatalf("PrepareRestoreLatest() error = %v", err)
	}
	if pending.Token == "" {
		t.Fatal("PrepareRestoreLatest() returned empty token")
	}

	result, err := svc.ConfirmRestore(context.Background(), pending.Token)
	if err != nil {
		t.Fatalf("ConfirmRestore() error = %v", err)
	}
	if result.SafetyBackup == nil {
		t.Fatal("ConfirmRestore() should create safety backup")
	}
	if got := strings.TrimSpace(restored.String()); got != "SELECT 42;" {
		t.Fatalf("ConfirmRestore() restored %q", got)
	}
}

func TestRunScheduledBackupIfDue(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{
		keyBackupEnabled:        "true",
		keyBackupScheduleTime:   "00:10",
		keyBackupSendToTelegram: "false",
	}}
	svc := NewService(store, Options{
		DatabaseURL:         "postgres://example",
		BackupDir:           dir,
		Timezone:            "Asia/Rangoon",
		DefaultScheduleTime: "00:10",
		Enabled:             true,
	})
	loc, _ := time.LoadLocation("Asia/Rangoon")
	currentTime := time.Date(2026, 4, 1, 0, 10, 0, 0, loc)
	svc.nowFn = func() time.Time {
		return currentTime
	}
	svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
		_, err := output.Write([]byte("SELECT now();"))
		return err
	}

	if err := svc.RunScheduledBackupIfDue(context.Background(), &fakeTelegramClient{}, 1); err != nil {
		t.Fatalf("RunScheduledBackupIfDue() error = %v", err)
	}
	if got := store.values[keyBackupLastScheduledDate]; got != "2026-04-01" {
		t.Fatalf("expected scheduled date to be persisted, got %q", got)
	}
}

func TestParseScheduleTime(t *testing.T) {
	if hour, minute, err := parseScheduleTime("09:45"); err != nil || hour != 9 || minute != 45 {
		t.Fatalf("parseScheduleTime() = (%d, %d, %v)", hour, minute, err)
	}
	if _, _, err := parseScheduleTime("99:99"); err == nil {
		t.Fatal("parseScheduleTime() should reject invalid time")
	}
}

func TestConfirmRestoreInvalidToken(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{
		keyBackupRestoreEnabled: "true",
	}}
	svc := NewService(store, Options{
		DatabaseURL:    "postgres://example",
		BackupDir:      dir,
		Timezone:       "Asia/Rangoon",
		RestoreEnabled: true,
	})
	svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
		_, err := output.Write([]byte("SELECT 9;"))
		return err
	}

	if _, err := svc.CreateBackup(context.Background(), "seed"); err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}
	if _, err := svc.PrepareRestoreLatest(context.Background()); err != nil {
		t.Fatalf("PrepareRestoreLatest() error = %v", err)
	}

	if _, err := svc.ConfirmRestore(context.Background(), "WRONGTOK"); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("ConfirmRestore() error = %v, expected invalid token error", err)
	}
}

func TestCreateBackupOperationLock(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{}}
	svc := NewService(store, Options{
		DatabaseURL: "postgres://example",
		BackupDir:   dir,
		Timezone:    "Asia/Rangoon",
	})

	started := make(chan struct{})
	release := make(chan struct{})
	svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
		close(started)
		<-release
		_, err := output.Write([]byte("SELECT 11;"))
		return err
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.CreateBackup(context.Background(), "first")
		done <- err
	}()

	<-started
	if _, err := svc.CreateBackup(context.Background(), "second"); !errors.Is(err, ErrOperationInProgress) {
		t.Fatalf("CreateBackup() concurrent error = %v, expected ErrOperationInProgress", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first CreateBackup() error = %v", err)
	}
}

func TestRunScheduledBackupIfDueFailureNotification(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{
		keyBackupEnabled:        "true",
		keyBackupScheduleTime:   "00:10",
		keyBackupSendToTelegram: "true",
	}}
	svc := NewService(store, Options{
		DatabaseURL:         "postgres://example",
		BackupDir:           dir,
		Timezone:            "Asia/Rangoon",
		DefaultScheduleTime: "00:10",
		Enabled:             true,
		SendToTelegram:      true,
	})
	loc, _ := time.LoadLocation("Asia/Rangoon")
	currentTime := time.Date(2026, 4, 1, 0, 10, 0, 0, loc)
	svc.nowFn = func() time.Time {
		return currentTime
	}
	svc.dumpFn = func(_ context.Context, _ string, _ io.Writer) error {
		return errors.New("dump failed")
	}
	tg := &fakeTelegramClient{}

	if err := svc.RunScheduledBackupIfDue(context.Background(), tg, 777); err == nil {
		t.Fatal("RunScheduledBackupIfDue() expected error")
	}
	if tg.MessageCount() == 0 {
		t.Fatal("expected failure notification message to be sent")
	}
	if !strings.Contains(strings.ToLower(tg.LastMessage()), "failed") {
		t.Fatalf("unexpected failure notification text: %q", tg.LastMessage())
	}
	if _, ok := store.values[keyBackupLastScheduledDate]; ok {
		t.Fatal("scheduled date should not be persisted when backup creation fails")
	}
}

func TestRunScheduledBackupIfDueRetriesSameDayAfterUploadFailure(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{
		keyBackupEnabled:        "true",
		keyBackupScheduleTime:   "00:10",
		keyBackupSendToTelegram: "true",
	}}
	svc := NewService(store, Options{
		DatabaseURL:         "postgres://example",
		BackupDir:           dir,
		Timezone:            "Asia/Rangoon",
		DefaultScheduleTime: "00:10",
		Enabled:             true,
		SendToTelegram:      true,
	})
	loc, _ := time.LoadLocation("Asia/Rangoon")
	currentTime := time.Date(2026, 4, 1, 0, 10, 0, 0, loc)
	svc.nowFn = func() time.Time {
		return currentTime
	}
	svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
		_, err := output.Write([]byte("SELECT retry();"))
		return err
	}

	failingTG := &fakeTelegramClient{sendDocumentErr: errors.New("telegram unavailable")}
	if err := svc.RunScheduledBackupIfDue(context.Background(), failingTG, 777); err == nil {
		t.Fatal("first RunScheduledBackupIfDue() expected upload error")
	}
	if got := store.values[keyBackupLastScheduledDate]; got != "" {
		t.Fatalf("scheduled date should stay unset after upload failure, got %q", got)
	}

	currentTime = time.Date(2026, 4, 1, 0, 11, 1, 0, loc)
	successTG := &fakeTelegramClient{}
	if err := svc.RunScheduledBackupIfDue(context.Background(), successTG, 777); err != nil {
		t.Fatalf("second RunScheduledBackupIfDue() error = %v", err)
	}
	if successTG.documentCount != 1 {
		t.Fatalf("expected retry to upload one backup document, got %d", successTG.documentCount)
	}
	if got := store.values[keyBackupLastScheduledDate]; got != "2026-04-01" {
		t.Fatalf("expected scheduled date after successful retry, got %q", got)
	}
}

func TestRunScheduledBackupIfDueSingleClaimAcrossServices(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{
		keyBackupEnabled:        "true",
		keyBackupScheduleTime:   "00:10",
		keyBackupSendToTelegram: "false",
	}}
	opts := Options{
		DatabaseURL:         "postgres://example",
		BackupDir:           dir,
		Timezone:            "Asia/Rangoon",
		DefaultScheduleTime: "00:10",
		Enabled:             true,
		JobTimeout:          time.Minute,
	}

	loc, _ := time.LoadLocation("Asia/Rangoon")
	release := make(chan struct{})
	var dumpCalls int
	var dumpMu sync.Mutex

	makeSvc := func(now time.Time) *Service {
		svc := NewService(store, opts)
		svc.nowFn = func() time.Time {
			return now
		}
		svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
			dumpMu.Lock()
			dumpCalls++
			dumpMu.Unlock()
			<-release
			_, err := output.Write([]byte("SELECT once();"))
			return err
		}
		return svc
	}

	svcA := makeSvc(time.Date(2026, 4, 1, 0, 10, 0, 0, loc))
	svcB := makeSvc(time.Date(2026, 4, 1, 0, 10, 1, 0, loc))

	errs := make(chan error, 2)
	go func() {
		errs <- svcA.RunScheduledBackupIfDue(context.Background(), &fakeTelegramClient{}, 1)
	}()
	go func() {
		errs <- svcB.RunScheduledBackupIfDue(context.Background(), &fakeTelegramClient{}, 1)
	}()

	time.Sleep(150 * time.Millisecond)
	close(release)

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("RunScheduledBackupIfDue() concurrent error = %v", err)
		}
	}

	dumpMu.Lock()
	gotCalls := dumpCalls
	dumpMu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("expected exactly one scheduled dump across services, got %d", gotCalls)
	}
}

func TestBackupFileIsGzipped(t *testing.T) {
	dir := t.TempDir()
	store := &fakeConfigStore{values: map[string]string{}}
	svc := NewService(store, Options{
		DatabaseURL: "postgres://example",
		BackupDir:   dir,
		Timezone:    "Asia/Rangoon",
	})
	svc.dumpFn = func(_ context.Context, _ string, output io.Writer) error {
		_, err := output.Write([]byte("SELECT 7;"))
		return err
	}

	result, err := svc.CreateBackup(context.Background(), "gzip")
	if err != nil {
		t.Fatalf("CreateBackup() error = %v", err)
	}

	file, err := os.Open(filepath.Clean(result.File.Path))
	if err != nil {
		t.Fatalf("os.Open() error = %v", err)
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "SELECT 7;" {
		t.Fatalf("gzipped payload = %q", got)
	}
}

type fakeTelegramClient struct {
	mu              sync.Mutex
	messages        []string
	messageCount    int
	documentCount   int
	sendMessageErr  error
	sendDocumentErr error
}

func (f *fakeTelegramClient) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messageCount++
	f.messages = append(f.messages, params.Text)
	if f.sendMessageErr != nil {
		return nil, f.sendMessageErr
	}
	return &models.Message{}, nil
}

func (f *fakeTelegramClient) SendDocument(_ context.Context, _ *bot.SendDocumentParams) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.documentCount++
	if f.sendDocumentErr != nil {
		return nil, f.sendDocumentErr
	}
	return &models.Message{}, nil
}

func (f *fakeTelegramClient) MessageCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.messageCount
}

func (f *fakeTelegramClient) LastMessage() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.messages) == 0 {
		return ""
	}
	return f.messages[len(f.messages)-1]
}
