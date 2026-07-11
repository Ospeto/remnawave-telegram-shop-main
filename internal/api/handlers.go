package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/reporting"
	walletsvc "remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	appPromo "remnawave-tg-shop-bot/internal/promo"
)

// --- Response types ---

type KeyResponse struct {
	ID              int64      `json:"id"`
	Label           string     `json:"label"`
	Username        string     `json:"username"`
	SubscriptionURL string     `json:"subscription_url"`
	HappLink        string     `json:"happ_link"`
	RedirectURL     string     `json:"redirect_url,omitempty"`
	ExpireAt        *time.Time `json:"expire_at"`
	DaysRemaining   int        `json:"days_remaining"`
	Status          string     `json:"status"`
	TrafficUsedGB   float64    `json:"traffic_used_gb"`
	TrafficLimitGB  float64    `json:"traffic_limit_gb"`
	AutoRenew       bool       `json:"auto_renew"`
}

type UserResponse struct {
	ID         int64 `json:"id"`
	TelegramID int64 `json:"telegram_id"`
}

type ValidationResponse struct {
	User                     *UserResponse `json:"user"`
	Keys                     []KeyResponse `json:"keys"`
	IsActive                 bool          `json:"is_active"`
	IsAdmin                  bool          `json:"is_admin"`
	ExpireAt                 *time.Time    `json:"expire_at"`
	DaysRemaining            int           `json:"days_remaining"`
	TrialEligible            bool          `json:"trial_eligible"`
	TrialDays                int           `json:"trial_days"`
	ReferralCount            *int          `json:"referral_count,omitempty"`
	ReferralEarned           *float64      `json:"referral_earned,omitempty"`
	ReferralStatsUnavailable bool          `json:"referral_stats_unavailable,omitempty"`
	ReferralBonusAmount      float64       `json:"referral_bonus_amount"`
	BotURL                   string        `json:"bot_url"`
	SupportURL               string        `json:"support_url,omitempty"`
}

func userResponse(customer *database.Customer) *UserResponse {
	if customer == nil {
		return nil
	}
	return &UserResponse{
		ID:         customer.ID,
		TelegramID: customer.TelegramID,
	}
}

type PlanResponse struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	SortOrder      int    `json:"sort_order"`
	Currency       string `json:"currency"`
}

type CreatePurchaseRequest struct {
	PlanIndex     int    `json:"plan_index"`
	PlanID        string `json:"plan_id,omitempty"`
	ExtendKeyID   *int64 `json:"extend_key_id,omitempty"`
	PromoCode     string `json:"promo_code,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
	Amount        int    `json:"amount,omitempty"` // Explicit amount for wallet top-up
}

type CreatePurchaseResponse struct {
	PurchaseID       int64                     `json:"purchase_id"`
	PaymentPhone     string                    `json:"payment_phone,omitempty"`
	PaymentPhones    map[string]string         `json:"payment_phones,omitempty"`
	PaymentProviders []payment.PaymentProvider `json:"payment_providers,omitempty"`
	Amount           int                       `json:"amount"`
	Currency         string                    `json:"currency"`
	Instructions     string                    `json:"instructions,omitempty"`
	InvoiceType      string                    `json:"invoice_type"`
	ExtendKeyID      *int64                    `json:"extend_key_id,omitempty"`
	BotURL           string                    `json:"bot_url"`
	HappLink         string                    `json:"happ_link,omitempty"`
	RedirectURL      string                    `json:"redirect_url,omitempty"`
}

type PendingPurchaseConflictResponse struct {
	Code            string                 `json:"code"`
	Message         string                 `json:"message"`
	PendingPurchase CreatePurchaseResponse `json:"pending_purchase"`
}

type CancelPurchaseResponse struct {
	PurchaseID int64  `json:"purchase_id"`
	Status     string `json:"status"`
}

type SessionExchangeResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type AdminPromoResponse struct {
	Code            string    `json:"code"`
	DiscountPercent int       `json:"discount_percent"`
	MaxUses         int       `json:"max_uses"`
	UsedCount       int       `json:"used_count"`
	ValidUntil      time.Time `json:"valid_until"`
	CreatedAt       time.Time `json:"created_at"`
	Status          string    `json:"status"`
}

type AdminPlanResponse struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	SortOrder      int    `json:"sort_order"`
	Active         bool   `json:"active"`
	Currency       string `json:"currency"`
}

type AdminPlanRequest struct {
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	SortOrder      int    `json:"sort_order"`
}

type syncKeyStats = payment.KeyStats

type UploadScreenshotResponse struct {
	Status       string `json:"status"`
	Message      string `json:"message"`
	Reason       string `json:"reason,omitempty"`
	HappLink     string `json:"happ_link,omitempty"`
	RedirectURL  string `json:"redirect_url,omitempty"`
	TestMode     bool   `json:"test_mode,omitempty"`
	ShadowPassed *bool  `json:"shadow_passed,omitempty"`
}

func uploadScreenshotFailureResponse(result *payment.VerificationResult) UploadScreenshotResponse {
	return UploadScreenshotResponse{
		Status:  "failed",
		Message: result.Reason,
		Reason:  result.ReasonKey,
	}
}

func referralActivityAt(ref database.Referral) time.Time {
	return database.ReferralActivityAt(ref)
}

func (h *APIHandler) referralSummary(ctx context.Context, customer *database.Customer) (int, float64, bool) {
	if h.referralRepo == nil || customer == nil {
		return 0, 0, false
	}

	referrals, err := h.referralRepo.FindByReferrerAny(ctx, database.ReferralIdentityValues(customer.ID, customer.TelegramID)...)
	if err != nil {
		slog.Warn("Failed to load referrals", "customer_id", customer.ID, "error", err)
		return 0, 0, true
	}
	referrals, err = database.NormalizeReferralsByReferee(ctx, referrals, h.customerRepo)
	if err != nil {
		slog.Warn("Failed to normalize referrals", "customer_id", customer.ID, "error", err)
		return 0, 0, true
	}

	count := len(referrals)
	total := 0.0
	if h.paymentService != nil {
		var err error
		total, err = h.paymentService.ReferralEarnedTotal(ctx, customer.ID)
		if err != nil {
			slog.Warn("Failed to sum referral earnings", "customer_id", customer.ID, "error", err)
			return 0, 0, true
		}
	}

	return count, total, false
}

func (h *APIHandler) referralMaskedTelegramID(ctx context.Context, customerID int64) string {
	customer, err := database.ResolveReferralCustomer(ctx, h.customerRepo, customerID)
	if err != nil || customer == nil {
		return maskHalfInt64(customerID)
	}
	return maskHalfInt64(customer.TelegramID)
}

func writeSanitizedError(w http.ResponseWriter, status int, publicMessage string, err error) {
	if err != nil {
		slog.Error(publicMessage, "status", status, "error", err)
	}
	http.Error(w, publicMessage, status)
}

// mapCreatePurchaseIdempotencyError maps payment-layer idempotency isolation errors
// to HTTP status/message. Cross-user key reuse must not disclose purchase details.
func mapCreatePurchaseIdempotencyError(err error) (status int, message string, ok bool) {
	switch {
	case errors.Is(err, payment.ErrIdempotencyKeyConflict):
		return http.StatusForbidden, "Idempotency key conflict", true
	case errors.Is(err, payment.ErrIdempotencyRequestMismatch):
		return http.StatusConflict, "Idempotency key already used with a different request", true
	default:
		return 0, "", false
	}
}

// shouldUpdatePurchaseFieldsAfterCreate gates plan_label/payment_phone writes.
// Only stamp when the purchase has not been labeled yet so idempotent resumes
// (and body-matched retries) do not rewrite the original row.
func shouldUpdatePurchaseFieldsAfterCreate(purchase *database.Purchase) bool {
	if purchase == nil {
		return false
	}
	return purchase.PlanLabel == ""
}

func adminPromoResponse(code database.PromoCode, now time.Time) AdminPromoResponse {
	return AdminPromoResponse{
		Code:            code.Code,
		DiscountPercent: code.DiscountPercent,
		MaxUses:         code.MaxUses,
		UsedCount:       code.UsedCount,
		ValidUntil:      code.ValidUntil,
		CreatedAt:       code.CreatedAt,
		Status:          appPromo.StatusAt(code, now),
	}
}

func adminPlanResponse(plan config.Plan) AdminPlanResponse {
	return AdminPlanResponse{
		ID:             plan.ID,
		Label:          plan.Label,
		Days:           plan.Days,
		Price:          plan.Price,
		TrafficLimitGB: plan.TrafficLimitGB,
		SortOrder:      plan.SortOrder,
		Active:         plan.Active,
		Currency:       config.Currency(),
	}
}

func validateAdminPlanRequest(req AdminPlanRequest) error {
	if strings.TrimSpace(req.Label) == "" {
		return errors.New("Plan label is required")
	}
	if req.Days <= 0 {
		return errors.New("Plan days must be greater than zero")
	}
	if req.Price <= 0 {
		return errors.New("Plan price must be greater than zero")
	}
	if req.TrafficLimitGB < 0 {
		return errors.New("Plan traffic limit cannot be negative")
	}
	if req.SortOrder < 0 {
		return errors.New("Plan sort order cannot be negative")
	}
	return nil
}

func writePlanCatalogError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	switch {
	case strings.Contains(message, "must be unique"):
		http.Error(w, message, http.StatusConflict)
		return true
	case strings.Contains(message, "required"),
		strings.Contains(message, "positive"),
		strings.Contains(message, "cannot be negative"):
		http.Error(w, message, http.StatusBadRequest)
		return true
	default:
		return false
	}
}

func resolvePurchasePlan(req CreatePurchaseRequest) (*config.Plan, error) {
	if strings.TrimSpace(req.PlanID) != "" {
		plan := config.PlanByID(req.PlanID)
		if plan == nil || !plan.Active {
			return nil, errors.New("Invalid plan id")
		}
		return plan, nil
	}

	plan := config.PlanByIndex(req.PlanIndex)
	if plan == nil {
		return nil, errors.New("Invalid plan index")
	}
	return plan, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isPromoNotFoundError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (h *APIHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "twa ") {
		http.Error(w, "Telegram session expired. Please reopen the app and try again.", http.StatusUnauthorized)
		return
	}

	initData := strings.TrimSpace(strings.TrimPrefix(authHeader, "twa "))
	if initData == "" {
		http.Error(w, "Telegram session expired. Please reopen the app and try again.", http.StatusUnauthorized)
		return
	}

	telegramID, username, bindingKey, expiresAt, err := validateInitData(initData, config.TelegramToken(), telegramSessionExchangeMaxAge)
	if err != nil {
		slog.Warn("Rejected Telegram session exchange", "reason", err.Error())
		http.Error(w, "Telegram session expired. Please reopen the app and try again.", http.StatusUnauthorized)
		return
	}
	if err := initDataExchanges.consume(r.Context(), bindingKey, expiresAt); err != nil {
		slog.Warn("Rejected reused Telegram initData", "telegram_id", telegramID)
		http.Error(w, "Telegram session expired. Please reopen the app and try again.", http.StatusUnauthorized)
		return
	}

	token, sessionExpiresAt, err := authSessions.create(r.Context(), telegramID, username, requestFingerprint(r))
	if err != nil {
		slog.Error("Failed to create auth session", "telegram_id", telegramID, "error", err)
		http.Error(w, "Unable to start session right now. Please try again.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SessionExchangeResponse{
		Token:     token,
		ExpiresAt: sessionExpiresAt,
	})
}

type PurchaseStatusResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

func parsePaymentMethod(method string) (database.InvoiceType, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", "mobile_banking":
		return database.InvoiceTypeMobileBanking, nil
	case "wallet":
		return database.InvoiceTypeWalletPayment, nil
	case "wallet_topup":
		return database.InvoiceTypeWalletTopUp, nil
	default:
		return "", fmt.Errorf("unsupported payment_method %q", method)
	}
}

// --- Handler ---

type financialAdjustmentCreator interface {
	Create(ctx context.Context, in database.CreateFinancialAdjustmentInput) (*database.FinancialAdjustment, bool, error)
}

// financeReporter is the sole reviewed finance report path used by revenue JSON/CSV handlers.
type financeReporter interface {
	GetReport(ctx context.Context, q reporting.ReportQuery) (reporting.FinanceReport, error)
}

type APIHandler struct {
	customerRepo               *database.CustomerRepository
	paymentService             *payment.PaymentService
	telegramBot                *bot.Bot
	translation                *translation.Manager
	subKeyRepo                 *database.SubscriptionKeyRepository
	promoCodeRepository        *database.PromoCodeRepository
	appConfigRepo              *database.AppConfigRepository
	walletService              *walletsvc.WalletService
	referralRepo               *database.ReferralRepository
	financeService             financeReporter
	financialAdjustmentRepo    financialAdjustmentCreator
	screenshotMu               sync.Mutex
	screenshotAttempts         map[int64]time.Time
	customerScreenshotAttempts map[int64][]time.Time
	screenshotInFlight         map[int64]struct{}
	now                        func() time.Time
	findCustomerByTelegramID   func(context.Context, int64) (*database.Customer, error)
	getPurchaseByID            func(context.Context, int64) (*database.Purchase, error)
	listPromoCodes             func(context.Context) ([]database.PromoCode, error)
	createPromoCode            func(context.Context, string, int, int, time.Time) error
	deletePromoCode            func(context.Context, string) error
	retirePromoCode            func(context.Context, string, time.Time) error
	savePlansCatalog           func(context.Context, []config.Plan) error
	findSubscriptionKeys       func(context.Context, int64) ([]database.SubscriptionKey, error)
	activateCustomerTrial      func(context.Context, int64) (string, error)
	getCachedSyncKeys          func(int64) ([]syncKeyStats, bool)
	triggerSyncKeys            func(context.Context, int64, int64)
	isAdminTelegramID          func(int64) bool
}

func NewAPIHandler(
	customerRepo *database.CustomerRepository,
	paymentService *payment.PaymentService,
	telegramBot *bot.Bot,
	tm *translation.Manager,
	subKeyRepo *database.SubscriptionKeyRepository,
	promoCodeRepository *database.PromoCodeRepository,
	walletService *walletsvc.WalletService,
	referralRepo *database.ReferralRepository,
	appConfigRepo *database.AppConfigRepository,
	financeService *reporting.FinanceService,
	financialAdjustmentRepo financialAdjustmentCreator,
) *APIHandler {
	h := &APIHandler{
		customerRepo:               customerRepo,
		paymentService:             paymentService,
		telegramBot:                telegramBot,
		translation:                tm,
		subKeyRepo:                 subKeyRepo,
		promoCodeRepository:        promoCodeRepository,
		appConfigRepo:              appConfigRepo,
		walletService:              walletService,
		referralRepo:               referralRepo,
		financialAdjustmentRepo:    financialAdjustmentRepo,
		screenshotAttempts:         make(map[int64]time.Time),
		customerScreenshotAttempts: make(map[int64][]time.Time),
		screenshotInFlight:         make(map[int64]struct{}),
		now:                        time.Now,
	}
	// Assign only a non-nil concrete pointer so a nil *FinanceService stays a nil interface
	// (typed-nil interface would bypass nil checks and panic on GetReport).
	if financeService != nil {
		h.financeService = financeService
	}
	return h
}

func (h *APIHandler) currentTime() time.Time {
	if h != nil && h.now != nil {
		return h.now()
	}
	return time.Now()
}

func (h *APIHandler) customerByTelegramID(ctx context.Context, telegramID int64) (*database.Customer, error) {
	if h.findCustomerByTelegramID != nil {
		return h.findCustomerByTelegramID(ctx, telegramID)
	}
	if h.customerRepo == nil {
		return nil, errors.New("customer repository is unavailable")
	}
	return h.customerRepo.FindByTelegramId(ctx, telegramID)
}

func (h *APIHandler) purchaseByID(ctx context.Context, purchaseID int64) (*database.Purchase, error) {
	if h.getPurchaseByID != nil {
		return h.getPurchaseByID(ctx, purchaseID)
	}
	if h.paymentService == nil {
		return nil, errors.New("payment service is unavailable")
	}
	return h.paymentService.GetPurchaseByID(ctx, purchaseID)
}

func (h *APIHandler) subscriptionKeysByCustomerID(ctx context.Context, customerID int64) ([]database.SubscriptionKey, error) {
	if h.findSubscriptionKeys != nil {
		return h.findSubscriptionKeys(ctx, customerID)
	}
	if h.subKeyRepo == nil {
		return nil, nil
	}
	return h.subKeyRepo.FindByCustomerID(ctx, customerID)
}

func (h *APIHandler) activateTrialForCustomer(ctx context.Context, telegramID int64) (string, error) {
	if h.activateCustomerTrial != nil {
		return h.activateCustomerTrial(ctx, telegramID)
	}
	if h.paymentService == nil {
		return "", errors.New("payment service is unavailable")
	}
	return h.paymentService.ActivateTrial(ctx, telegramID)
}

func (h *APIHandler) allPromoCodes(ctx context.Context) ([]database.PromoCode, error) {
	if h.listPromoCodes != nil {
		return h.listPromoCodes(ctx)
	}
	if h.promoCodeRepository == nil {
		return nil, errors.New("promo repository is unavailable")
	}
	return h.promoCodeRepository.ListAll(ctx)
}

func (h *APIHandler) persistPlans(ctx context.Context, plans []config.Plan) error {
	if h.savePlansCatalog != nil {
		return h.savePlansCatalog(ctx, plans)
	}
	return config.SavePlansCatalog(ctx, h.appConfigRepo, plans)
}

func (h *APIHandler) savePromoCode(ctx context.Context, code string, discount, maxUses int, validUntil time.Time) error {
	if h.createPromoCode != nil {
		return h.createPromoCode(ctx, code, discount, maxUses, validUntil)
	}
	if h.promoCodeRepository == nil {
		return errors.New("promo repository is unavailable")
	}
	return h.promoCodeRepository.Create(ctx, code, discount, maxUses, validUntil)
}

func (h *APIHandler) removePromoCode(ctx context.Context, code string) error {
	if h.deletePromoCode != nil {
		err := h.deletePromoCode(ctx, code)
		if err == nil || !isForeignKeyViolation(err) {
			return err
		}
		if h.retirePromoCode == nil {
			return err
		}
		return h.retirePromoCode(ctx, code, h.currentTime().Add(-time.Second))
	}
	if h.promoCodeRepository == nil {
		return errors.New("promo repository is unavailable")
	}
	err := h.promoCodeRepository.Delete(ctx, code)
	if err == nil || !isForeignKeyViolation(err) {
		return err
	}
	return h.promoCodeRepository.Retire(ctx, code, h.currentTime().Add(-time.Second))
}

func (h *APIHandler) cachedSyncStats(customerID int64) ([]syncKeyStats, bool) {
	if h.getCachedSyncKeys != nil {
		return h.getCachedSyncKeys(customerID)
	}
	if h.paymentService == nil {
		return nil, false
	}
	return h.paymentService.GetCachedSyncKeys(customerID)
}

func (h *APIHandler) requestSyncKeys(ctx context.Context, customerID, telegramID int64) {
	if h.triggerSyncKeys != nil {
		h.triggerSyncKeys(ctx, customerID, telegramID)
		return
	}
	if h.paymentService != nil {
		h.paymentService.TriggerSyncKeysAsync(ctx, customerID, telegramID)
	}
}

func (h *APIHandler) isAdminUser(telegramID int64) bool {
	if h.isAdminTelegramID != nil {
		return h.isAdminTelegramID(telegramID)
	}
	return telegramID == config.GetAdminTelegramId()
}

const (
	screenshotVerificationCooldown        = 15 * time.Second
	screenshotVerificationWindow          = 10 * time.Minute
	maxScreenshotVerificationsPerCustomer = 6
)

var errScreenshotVerificationRateLimited = errors.New("too many verification attempts")

const (
	uploadRejectMethodNotAllowed          = "method_not_allowed"
	uploadRejectMissingTelegramAuth       = "missing_telegram_auth"
	uploadRejectMissingPurchaseID         = "missing_purchase_id"
	uploadRejectInvalidPurchaseID         = "invalid_purchase_id"
	uploadRejectPurchaseLookupFailed      = "purchase_lookup_failed"
	uploadRejectPurchaseNotFound          = "purchase_not_found"
	uploadRejectCustomerLookupFailed      = "customer_lookup_failed"
	uploadRejectPurchaseOwnerMismatch     = "purchase_owner_mismatch"
	uploadRejectPurchaseNotAwaiting       = "purchase_not_awaiting_verification"
	uploadRejectUnsupportedInvoiceType    = "unsupported_invoice_type"
	uploadRejectVerificationRateLimited   = "verification_rate_limited"
	uploadRejectVerificationInProgress    = "verification_already_in_progress"
	uploadRejectInvalidMultipartForm      = "invalid_multipart_form"
	uploadRejectMissingFileField          = "missing_file_field"
	uploadRejectFileReadFailed            = "file_read_failed"
	uploadRejectUnsupportedFileType       = "unsupported_file_type"
	uploadRejectVerificationServiceFailed = "verification_service_failed"
)

func logUploadScreenshotReject(level slog.Level, r *http.Request, reason string, statusCode int, err error, attrs ...any) {
	logAttrs := []any{
		"event", "upload_screenshot_rejected",
		"reason", reason,
		"status_code", statusCode,
	}
	if r != nil {
		logAttrs = append(logAttrs,
			"method", r.Method,
			"path", r.URL.Path,
			"content_length", r.ContentLength,
		)
		if contentType := strings.TrimSpace(r.Header.Get("Content-Type")); contentType != "" {
			logAttrs = append(logAttrs, "content_type", contentType)
		}
	}
	if err != nil {
		logAttrs = append(logAttrs, "error", err)
	}
	logAttrs = append(logAttrs, attrs...)

	if level >= slog.LevelError {
		slog.Error("Upload screenshot rejected", logAttrs...)
		return
	}
	slog.Warn("Upload screenshot rejected", logAttrs...)
}

func trustedPaymentScreenshotMIME(fileBytes []byte) (string, bool) {
	detected := http.DetectContentType(fileBytes)
	switch detected {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return detected, true
	default:
		return detected, false
	}
}

func (h *APIHandler) beginScreenshotVerification(purchaseID, customerID int64) error {
	h.screenshotMu.Lock()
	defer h.screenshotMu.Unlock()

	if _, exists := h.screenshotInFlight[purchaseID]; exists {
		return fmt.Errorf("verification already in progress")
	}
	if lastAttempt, exists := h.screenshotAttempts[purchaseID]; exists && time.Since(lastAttempt) < screenshotVerificationCooldown {
		return fmt.Errorf("verification throttled")
	}

	now := time.Now()
	recentAttempts := h.customerScreenshotAttempts[customerID][:0]
	for _, attemptAt := range h.customerScreenshotAttempts[customerID] {
		if now.Sub(attemptAt) < screenshotVerificationWindow {
			recentAttempts = append(recentAttempts, attemptAt)
		}
	}
	if len(recentAttempts) >= maxScreenshotVerificationsPerCustomer {
		h.customerScreenshotAttempts[customerID] = recentAttempts
		return errScreenshotVerificationRateLimited
	}
	h.customerScreenshotAttempts[customerID] = append(recentAttempts, now)

	h.screenshotAttempts[purchaseID] = now
	h.screenshotInFlight[purchaseID] = struct{}{}
	return nil
}

func (h *APIHandler) finishScreenshotVerification(purchaseID int64) {
	h.screenshotMu.Lock()
	defer h.screenshotMu.Unlock()
	delete(h.screenshotInFlight, purchaseID)
}

func (h *APIHandler) screenshotVerificationInFlight(purchaseID int64) bool {
	h.screenshotMu.Lock()
	defer h.screenshotMu.Unlock()
	_, exists := h.screenshotInFlight[purchaseID]
	return exists
}

func validateScreenshotUploadAccess(purchase *database.Purchase, customer *database.Customer, telegramID int64) (int, string, string, bool) {
	if purchase == nil {
		return http.StatusNotFound, "Purchase not found", uploadRejectPurchaseNotFound, false
	}
	if customer == nil || customer.TelegramID != telegramID || purchase.CustomerID != customer.ID {
		return http.StatusNotFound, "Purchase not found", uploadRejectPurchaseOwnerMismatch, false
	}
	if purchase.Status != database.PurchaseStatusPending && purchase.Status != database.PurchaseStatusNew {
		return http.StatusConflict, "Purchase is not awaiting verification", uploadRejectPurchaseNotAwaiting, false
	}
	if !payment.SupportsScreenshotVerification(purchase.InvoiceType) {
		return http.StatusConflict, "This purchase does not accept screenshot verification", uploadRejectUnsupportedInvoiceType, false
	}
	return 0, "", "", true
}

func validatePendingPurchaseCancellationAccess(purchase *database.Purchase, customer *database.Customer, telegramID int64) (int, string, bool) {
	if purchase == nil {
		return http.StatusNotFound, "Purchase not found", false
	}
	if customer == nil || customer.TelegramID != telegramID || purchase.CustomerID != customer.ID {
		return http.StatusNotFound, "Purchase not found", false
	}
	if !payment.SupportsScreenshotVerification(purchase.InvoiceType) {
		return http.StatusConflict, "This purchase cannot be cancelled here", false
	}
	if purchase.Status != database.PurchaseStatusPending && purchase.Status != database.PurchaseStatusNew {
		return http.StatusConflict, "Purchase is not awaiting verification", false
	}
	return 0, "", true
}

func compactSubscriptionKeysForDisplay(keys []database.SubscriptionKey) []database.SubscriptionKey {
	if len(keys) < 2 {
		return keys
	}

	compacted := make([]database.SubscriptionKey, 0, len(keys))
	identityToIndex := make(map[string]int)

	isNewer := func(candidate, current database.SubscriptionKey) bool {
		statusRank := func(status string) int {
			switch status {
			case "active":
				return 3
			case "expired":
				return 2
			case "deleted":
				return 0
			default:
				return 1
			}
		}

		if candidateRank, currentRank := statusRank(candidate.Status), statusRank(current.Status); candidateRank != currentRank {
			return candidateRank > currentRank
		}

		if candidate.ExpireAt != nil && current.ExpireAt == nil {
			return true
		}
		if candidate.ExpireAt == nil && current.ExpireAt != nil {
			return false
		}
		if candidate.ExpireAt != nil && current.ExpireAt != nil {
			if candidate.ExpireAt.After(*current.ExpireAt) {
				return true
			}
			if candidate.ExpireAt.Before(*current.ExpireAt) {
				return false
			}
		}

		if candidate.CreatedAt.After(current.CreatedAt) {
			return true
		}
		if candidate.CreatedAt.Before(current.CreatedAt) {
			return false
		}

		return candidate.ID > current.ID
	}

	identity := func(key database.SubscriptionKey) (string, bool) {
		if url := strings.TrimSpace(key.SubscriptionURL); url != "" {
			return "url|" + url, true
		}
		if key.RemnawaveUUID != uuid.Nil {
			return "uuid|" + key.RemnawaveUUID.String(), true
		}
		return "", false
	}

	for _, key := range keys {
		id, ok := identity(key)
		if !ok {
			compacted = append(compacted, key)
			continue
		}

		if idx, exists := identityToIndex[id]; exists {
			if isNewer(key, compacted[idx]) {
				compacted[idx] = key
			}
			continue
		}

		identityToIndex[id] = len(compacted)
		compacted = append(compacted, key)
	}

	sort.SliceStable(compacted, func(i, j int) bool {
		if compacted[i].CreatedAt.Equal(compacted[j].CreatedAt) {
			return compacted[i].ID > compacted[j].ID
		}
		return compacted[i].CreatedAt.After(compacted[j].CreatedAt)
	})

	return compacted
}

// --- Handlers ---

func promoValidationStatus(promo *database.PromoCode, lookupErr error, now time.Time) int {
	if lookupErr != nil {
		return http.StatusServiceUnavailable
	}
	if promo == nil {
		return http.StatusNotFound
	}
	if promo.UsedCount >= promo.MaxUses {
		return http.StatusNotFound
	}
	if !promo.ValidUntil.After(now) {
		return http.StatusNotFound
	}
	return http.StatusOK
}

func (h *APIHandler) pendingPurchaseConflictResponse(customer *database.Customer, purchase *database.Purchase) PendingPurchaseConflictResponse {
	currency := purchase.Currency
	if strings.TrimSpace(currency) == "" {
		currency = config.Currency()
	}

	instructions := ""
	mobileNumber := ""
	var paymentPhones map[string]string
	var paymentProviders []payment.PaymentProvider
	if payment.SupportsScreenshotVerification(purchase.InvoiceType) {
		instructions = fmt.Sprintf(
			h.translation.GetText(customer.Language, "mobile_pay_instructions"),
			int(purchase.Amount),
			payment.BuildPaymentReceiversHTML(),
		)
		mobileNumber = payment.GetFirstPaymentPhone()
		paymentPhones = payment.GetAllPaymentPhones()
		paymentProviders = payment.GetEnabledPaymentProviders()
	}

	return PendingPurchaseConflictResponse{
		Code:    "pending_screenshot_payment",
		Message: "You already have a pending screenshot payment. Upload its screenshot or cancel it to choose another plan.",
		PendingPurchase: CreatePurchaseResponse{
			PurchaseID:       purchase.ID,
			PaymentPhone:     mobileNumber,
			PaymentPhones:    paymentPhones,
			PaymentProviders: paymentProviders,
			Amount:           int(purchase.Amount),
			Currency:         currency,
			Instructions:     instructions,
			InvoiceType:      string(purchase.InvoiceType),
			ExtendKeyID:      purchase.ExtendKeyID,
			BotURL:           config.BotURL(),
		},
	}
}

func (h *APIHandler) ValidatePromo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code", http.StatusBadRequest)
		return
	}

	promo, err := h.promoCodeRepository.FindByCode(r.Context(), code)
	switch promoValidationStatus(promo, err, time.Now()) {
	case http.StatusServiceUnavailable:
		http.Error(w, "Promo validation is temporarily unavailable", http.StatusServiceUnavailable)
		return
	case http.StatusNotFound:
		http.Error(w, "Invalid or expired code", http.StatusNotFound)
		return
	}

	resp := map[string]interface{}{
		"valid":            true,
		"code":             promo.Code,
		"discount_percent": promo.DiscountPercent,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) CreatePurchase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	if rawKey := strings.TrimSpace(r.Header.Get("Idempotency-Key")); rawKey != "" {
		key, err := uuid.Parse(rawKey)
		if err != nil {
			http.Error(w, "Invalid Idempotency-Key header", http.StatusBadRequest)
			return
		}
		ctx = payment.WithIdempotencyKey(ctx, key)
	}

	telegramID, ok := ctx.Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req CreatePurchaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	invoiceType, err := parsePaymentMethod(req.PaymentMethod)
	if err != nil {
		http.Error(w, "Invalid payment method", http.StatusBadRequest)
		return
	}

	var price float64
	var days int
	var trafficLimit int
	var label string

	if invoiceType == database.InvoiceTypeWalletTopUp {
		if req.Amount <= 0 {
			http.Error(w, "Invalid amount for top-up", http.StatusBadRequest)
			return
		}
		if req.PromoCode != "" {
			http.Error(w, "promo_code is not valid for wallet top-up", http.StatusBadRequest)
			return
		}
		// For top-up, we use the explicit amount
		price = float64(req.Amount)
		days = 0
		trafficLimit = 0
		label = fmt.Sprintf("Wallet Top-up: %d %s", req.Amount, config.Currency())
	} else {
		plan, err := resolvePurchasePlan(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		price = float64(plan.Price)
		days = plan.Days
		trafficLimit = plan.TrafficLimitGB
		label = plan.Label
	}

	customer, err := h.customerRepo.FindByTelegramId(ctx, telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if req.ExtendKeyID != nil {
		if invoiceType == database.InvoiceTypeWalletTopUp {
			http.Error(w, "extend_key_id is not valid for wallet top-up", http.StatusBadRequest)
			return
		}
		if h.subKeyRepo == nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		extendKey, err := h.subKeyRepo.FindByID(ctx, *req.ExtendKeyID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if extendKey == nil {
			http.Error(w, "Subscription key not found", http.StatusNotFound)
			return
		}
		if extendKey.CustomerID != customer.ID {
			http.Error(w, "Purchase not allowed", http.StatusForbidden)
			return
		}
	}

	var purchaseID int64
	if req.ExtendKeyID != nil {
		switch invoiceType {
		case database.InvoiceTypeWalletPayment:
			_, purchaseID, err = h.paymentService.CreatePurchaseWithExtend(ctx, price, days, trafficLimit, customer, *req.ExtendKeyID, req.PromoCode)
		case database.InvoiceTypeWalletTopUp:
			http.Error(w, "extend_key_id is not valid for wallet top-up", http.StatusBadRequest)
			return
		default:
			_, purchaseID, err = h.paymentService.CreatePurchaseWithExtendForInvoice(ctx, price, days, trafficLimit, customer, invoiceType, req.PromoCode, *req.ExtendKeyID)
		}
	} else {
		_, purchaseID, err = h.paymentService.CreatePurchase(ctx, price, days, trafficLimit, customer, invoiceType, req.PromoCode)
	}
	if err != nil {
		if status, message, ok := mapCreatePurchaseIdempotencyError(err); ok {
			http.Error(w, message, status)
			return
		}
		if errors.Is(err, payment.ErrInvalidPromoCode) {
			http.Error(w, "Invalid or expired promo code", http.StatusBadRequest)
			return
		}
		var pendingErr *payment.AwaitingReceiptVerificationError
		if errors.As(err, &pendingErr) && pendingErr.Purchase != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(h.pendingPurchaseConflictResponse(customer, pendingErr.Purchase))
			return
		}
		if errors.Is(err, payment.ErrAwaitingReceiptVerification) {
			http.Error(w, "You already have a pending screenshot payment. Upload its screenshot or cancel it to choose another plan.", http.StatusConflict)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to create purchase", err)
		return
	}

	// Fetch the actual purchase to get the final amount (which might be discounted)
	purchase, err := h.paymentService.GetPurchaseByID(ctx, purchaseID)
	if err != nil || purchase == nil {
		slog.Error("Failed to retrieve purchase after creation", "purchase_id", purchaseID, "error", err)
		http.Error(w, "Failed to retrieve purchase details", http.StatusInternalServerError)
		return
	}

	// Only stamp plan_label/payment_phone when unset. Idempotent resumes must not
	// rewrite the original purchase (especially after a body-mismatched attempt is
	// rejected at the payment layer; same-body retries keep original fields).
	if shouldUpdatePurchaseFieldsAfterCreate(purchase) {
		updateFields := map[string]interface{}{
			"plan_label":    label,
			"payment_phone": payment.GetFirstPaymentPhone(),
		}
		if err := h.paymentService.UpdatePurchaseFields(ctx, purchaseID, updateFields); err != nil {
			slog.Warn("Failed to update purchase fields", "purchase_id", purchaseID, "error", err)
		}
	}

	var instructions string
	var mobileNumber string
	var paymentPhones map[string]string
	var paymentProviders []payment.PaymentProvider
	if purchase.InvoiceType == database.InvoiceTypeMobileBanking || purchase.InvoiceType == database.InvoiceTypeWalletTopUp {
		instructions = fmt.Sprintf(
			h.translation.GetText(customer.Language, "mobile_pay_instructions"),
			int(purchase.Amount),
			payment.BuildPaymentReceiversHTML(),
		)
		mobileNumber = payment.GetFirstPaymentPhone()
		paymentPhones = payment.GetAllPaymentPhones()
		paymentProviders = payment.GetEnabledPaymentProviders()
	}

	happLink := ""
	if purchase.InvoiceType == database.InvoiceTypeWalletPayment {
		if h.subKeyRepo != nil {
			if req.ExtendKeyID != nil {
				if extendKey, kErr := h.subKeyRepo.FindByID(ctx, *req.ExtendKeyID); kErr == nil && extendKey != nil && extendKey.SubscriptionURL != "" {
					happLink = "happ://add/" + extendKey.SubscriptionURL
				}
			} else {
				keys, kErr := h.subKeyRepo.FindByCustomerID(ctx, customer.ID)
				if kErr == nil && len(keys) > 0 {
					latestKey := keys[0]
					happLink = "happ://add/" + latestKey.SubscriptionURL
				}
			}
		}
	}

	resp := CreatePurchaseResponse{
		PurchaseID:       purchase.ID,
		PaymentPhone:     mobileNumber,
		PaymentPhones:    paymentPhones,
		PaymentProviders: paymentProviders,
		Amount:           int(purchase.Amount),
		Currency:         config.Currency(),
		Instructions:     instructions,
		InvoiceType:      string(purchase.InvoiceType),
		ExtendKeyID:      purchase.ExtendKeyID,
		BotURL:           config.BotURL(),
		HappLink:         happLink,
		RedirectURL:      signedRedirectURLForTarget(happLink),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerByTelegramID(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	var keys []KeyResponse
	const bytesInGB = 1073741824.0

	// Track if user has any keys in the new system (even deleted ones).
	hasMigratedKeys := false

	cachedStats, hasCachedStats := h.cachedSyncStats(customer.ID)
	if !hasCachedStats {
		h.requestSyncKeys(r.Context(), customer.ID, customer.TelegramID)
	}
	statsMap := make(map[int64]payment.KeyStats, len(cachedStats))
	for _, sk := range cachedStats {
		statsMap[sk.ID] = sk
	}

	if h.subKeyRepo != nil || h.findSubscriptionKeys != nil {
		localKeys, _ := h.subscriptionKeysByCustomerID(r.Context(), customer.ID)
		localKeys = compactSubscriptionKeysForDisplay(localKeys)
		if len(localKeys) > 0 {
			hasMigratedKeys = true
		}
		if hasMigratedKeys {
			primaryKey := database.PrimarySubscriptionKey(localKeys)
			if primaryKey == nil {
				customer.SubscriptionLink = nil
				customer.ExpireAt = nil
			} else {
				if strings.TrimSpace(primaryKey.SubscriptionURL) == "" {
					customer.SubscriptionLink = nil
				} else {
					link := primaryKey.SubscriptionURL
					customer.SubscriptionLink = &link
				}
				customer.ExpireAt = primaryKey.ExpireAt
			}
		}
		for _, k := range localKeys {
			if k.Status == "deleted" {
				continue
			}

			kDays := 0
			if k.ExpireAt != nil && k.ExpireAt.After(time.Now()) {
				kDays = int(time.Until(*k.ExpireAt).Hours() / 24)
			}

			usedGB, limitGB := 0.0, 0.0
			if stat, ok := statsMap[k.ID]; ok {
				usedGB = stat.TrafficUsedBytes / bytesInGB
				limitGB = float64(stat.TrafficLimitBytes) / bytesInGB
			} else if k.TrafficLimitGB > 0 {
				limitGB = float64(k.TrafficLimitGB)
			}

			keys = append(keys, KeyResponse{
				ID:              k.ID,
				Label:           k.Label,
				Username:        k.Username,
				SubscriptionURL: k.SubscriptionURL,
				HappLink:        "happ://add/" + k.SubscriptionURL,
				RedirectURL:     signedRedirectURLForTarget("happ://add/" + k.SubscriptionURL),
				ExpireAt:        k.ExpireAt,
				DaysRemaining:   kDays,
				Status:          k.Status,
				TrafficUsedGB:   usedGB,
				TrafficLimitGB:  limitGB,
				AutoRenew:       k.AutoRenew,
			})
		}
	}

	isActive := false
	daysRemaining := 0
	if customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()) {
		isActive = true
		daysRemaining = int(time.Until(*customer.ExpireAt).Hours() / 24)
	}

	// Legacy fallback: customer has subscription_link but no subscription_key rows (not migrated yet)
	if len(keys) == 0 && !hasMigratedKeys && customer.SubscriptionLink != nil && *customer.SubscriptionLink != "" {
		status := "expired"
		if isActive {
			status = "active"
		}
		keys = append(keys, KeyResponse{
			ID:              0,
			Label:           "Key 1",
			SubscriptionURL: *customer.SubscriptionLink,
			HappLink:        "happ://add/" + *customer.SubscriptionLink,
			RedirectURL:     signedRedirectURLForTarget("happ://add/" + *customer.SubscriptionLink),
			ExpireAt:        customer.ExpireAt,
			DaysRemaining:   daysRemaining,
			Status:          status,
		})
	}

	// Determine trial eligibility: trial enabled + no subscription history.
	trialEligible := config.TrialDays() > 0 && customer.TrialUsedAt == nil && customer.SubscriptionLink == nil && len(keys) == 0 && !hasMigratedKeys

	referralCount, referralEarned, referralStatsUnavailable := h.referralSummary(r.Context(), customer)

	resp := ValidationResponse{
		User:                     userResponse(customer),
		Keys:                     keys,
		IsActive:                 isActive,
		IsAdmin:                  h.isAdminUser(telegramID),
		ExpireAt:                 customer.ExpireAt,
		DaysRemaining:            daysRemaining,
		TrialEligible:            trialEligible,
		TrialDays:                config.TrialDays(),
		ReferralStatsUnavailable: referralStatsUnavailable,
		ReferralBonusAmount:      payment.ReferralBonusAmount,
		BotURL:                   config.BotURL(),
		SupportURL:               config.SupportURL(),
	}
	if !referralStatsUnavailable {
		resp.ReferralCount = &referralCount
		resp.ReferralEarned = &referralEarned
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) AdminPromos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListAdminPromos(w, r)
	case http.MethodPost:
		h.CreateAdminPromo(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) ListAdminPromos(w http.ResponseWriter, r *http.Request) {
	if h.promoCodeRepository == nil && h.listPromoCodes == nil {
		http.Error(w, "Promo management is unavailable", http.StatusInternalServerError)
		return
	}

	codes, err := h.allPromoCodes(r.Context())
	if err != nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to list promo codes", err)
		return
	}

	now := h.currentTime()
	resp := make([]AdminPromoResponse, 0, len(codes))
	for _, code := range codes {
		resp = append(resp, adminPromoResponse(code, now))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) CreateAdminPromo(w http.ResponseWriter, r *http.Request) {
	if h.promoCodeRepository == nil && h.createPromoCode == nil {
		http.Error(w, "Promo management is unavailable", http.StatusInternalServerError)
		return
	}

	var req appPromo.CreateParams
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	params, err := appPromo.ValidateCreateParams(req.Code, req.DiscountPercent, req.DurationDays, req.MaxUses)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	validUntil := params.ValidUntilAt(h.currentTime())
	if err := h.savePromoCode(r.Context(), params.Code, params.DiscountPercent, params.MaxUses, validUntil); err != nil {
		if isUniqueViolation(err) {
			http.Error(w, "Promo code already exists", http.StatusConflict)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to create promo code", err)
		return
	}

	created := &database.PromoCode{
		Code:            params.Code,
		DiscountPercent: params.DiscountPercent,
		MaxUses:         params.MaxUses,
		UsedCount:       0,
		ValidUntil:      validUntil,
		CreatedAt:       h.currentTime(),
	}
	if h.promoCodeRepository != nil {
		loaded, err := h.promoCodeRepository.FindByCode(r.Context(), params.Code)
		if err != nil {
			writeSanitizedError(w, http.StatusInternalServerError, "Failed to load created promo code", err)
			return
		}
		if loaded != nil {
			created = loaded
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(adminPromoResponse(*created, h.currentTime()))
}

func (h *APIHandler) DeleteAdminPromo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.promoCodeRepository == nil && h.deletePromoCode == nil {
		http.Error(w, "Promo management is unavailable", http.StatusInternalServerError)
		return
	}

	code := strings.TrimPrefix(r.URL.Path, "/api/admin/promos/")
	code = strings.TrimSpace(code)
	if code == "" {
		http.Error(w, "Missing promo code", http.StatusBadRequest)
		return
	}

	if err := h.removePromoCode(r.Context(), code); err != nil {
		if isPromoNotFoundError(err) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to delete promo code", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) AdminPlans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.ListAdminPlans(w, r)
	case http.MethodPost:
		h.CreateAdminPlan(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) AdminPlanByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPatch:
		h.UpdateAdminPlan(w, r)
	case http.MethodDelete:
		h.DeleteAdminPlan(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) ListAdminPlans(w http.ResponseWriter, r *http.Request) {
	plans := config.AllPlans()
	resp := make([]AdminPlanResponse, 0, len(plans))
	for _, plan := range plans {
		resp = append(resp, adminPlanResponse(plan))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) CreateAdminPlan(w http.ResponseWriter, r *http.Request) {
	if h.appConfigRepo == nil && h.savePlansCatalog == nil {
		http.Error(w, "Plan management is unavailable", http.StatusInternalServerError)
		return
	}

	var req AdminPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateAdminPlanRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	plans := config.AllPlans()
	nextSortOrder := req.SortOrder
	if req.SortOrder == 0 && len(plans) > 0 {
		maxSortOrder := plans[0].SortOrder
		for _, plan := range plans[1:] {
			if plan.SortOrder > maxSortOrder {
				maxSortOrder = plan.SortOrder
			}
		}
		nextSortOrder = maxSortOrder + 1
	}

	plan := config.Plan{
		ID:             uuid.NewString(),
		Label:          strings.TrimSpace(req.Label),
		Days:           req.Days,
		Price:          req.Price,
		TrafficLimitGB: req.TrafficLimitGB,
		SortOrder:      nextSortOrder,
		Active:         true,
	}
	plans = append(plans, plan)
	if err := h.persistPlans(r.Context(), plans); err != nil {
		if writePlanCatalogError(w, err) {
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to create plan", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(adminPlanResponse(plan))
}

func (h *APIHandler) UpdateAdminPlan(w http.ResponseWriter, r *http.Request) {
	if h.appConfigRepo == nil && h.savePlansCatalog == nil {
		http.Error(w, "Plan management is unavailable", http.StatusInternalServerError)
		return
	}

	planID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/admin/plans/"))
	if planID == "" {
		http.Error(w, "Missing plan id", http.StatusBadRequest)
		return
	}

	var req AdminPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := validateAdminPlanRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	plans := config.AllPlans()
	updated := false
	var updatedPlan config.Plan
	for i := range plans {
		if plans[i].ID != planID {
			continue
		}
		plans[i].Label = strings.TrimSpace(req.Label)
		plans[i].Days = req.Days
		plans[i].Price = req.Price
		plans[i].TrafficLimitGB = req.TrafficLimitGB
		plans[i].SortOrder = req.SortOrder
		updatedPlan = plans[i]
		updated = true
		break
	}
	if !updated {
		http.Error(w, "Plan not found", http.StatusNotFound)
		return
	}
	if err := h.persistPlans(r.Context(), plans); err != nil {
		if writePlanCatalogError(w, err) {
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to update plan", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(adminPlanResponse(updatedPlan))
}

func (h *APIHandler) DeleteAdminPlan(w http.ResponseWriter, r *http.Request) {
	if h.appConfigRepo == nil && h.savePlansCatalog == nil {
		http.Error(w, "Plan management is unavailable", http.StatusInternalServerError)
		return
	}

	planID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/admin/plans/"))
	if planID == "" {
		http.Error(w, "Missing plan id", http.StatusBadRequest)
		return
	}

	plans := config.AllPlans()
	activeCount := 0
	targetIndex := -1
	for i := range plans {
		if plans[i].Active {
			activeCount++
		}
		if plans[i].ID == planID {
			targetIndex = i
		}
	}
	if targetIndex == -1 {
		http.Error(w, "Plan not found", http.StatusNotFound)
		return
	}
	if plans[targetIndex].Active && activeCount <= 1 {
		http.Error(w, "At least one active plan is required", http.StatusConflict)
		return
	}
	plans[targetIndex].Active = false
	if err := h.persistPlans(r.Context(), plans); err != nil {
		if writePlanCatalogError(w, err) {
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to archive plan", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) ActivateTrial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Use actual username if available, otherwise stringified ID. Ideally this logic belongs in service layer.
	username, _ := r.Context().Value(payment.UsernameCtxKey).(string)
	if username == "" {
		username = fmt.Sprintf("%d", telegramID)
	}
	ctxWithUsername := context.WithValue(r.Context(), payment.UsernameCtxKey, username)
	subURL, err := h.activateTrialForCustomer(ctxWithUsername, telegramID)
	if err != nil {
		switch {
		case errors.Is(err, payment.ErrTrialUnavailable):
			http.Error(w, "Trial is not available", http.StatusBadRequest)
		case errors.Is(err, payment.ErrCustomerNotFound):
			http.Error(w, "User not found", http.StatusNotFound)
		case errors.Is(err, payment.ErrTrialAlreadyUsed):
			http.Error(w, "Trial already used", http.StatusConflict)
		default:
			http.Error(w, "Failed to activate trial", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "activated",
		"subscription_url": subURL,
		"trial_days":       config.TrialDays(),
	})
}

func (h *APIHandler) GetPlans(w http.ResponseWriter, r *http.Request) {
	plans := config.ActivePlans()
	var response []PlanResponse
	currency := config.Currency()

	for _, p := range plans {
		response = append(response, PlanResponse{
			ID:             p.ID,
			Label:          p.Label,
			Days:           p.Days,
			Price:          p.Price,
			TrafficLimitGB: p.TrafficLimitGB,
			SortOrder:      p.SortOrder,
			Currency:       currency,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *APIHandler) UploadScreenshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectMethodNotAllowed, http.StatusMethodNotAllowed, nil)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectMissingTelegramAuth, http.StatusUnauthorized, nil)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	purchaseIDStr := r.URL.Query().Get("id")
	if purchaseIDStr == "" {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectMissingPurchaseID, http.StatusBadRequest, nil,
			"telegram_id", utils.MaskHalfInt64(telegramID),
		)
		http.Error(w, "Missing purchase id", http.StatusBadRequest)
		return
	}
	purchaseID, err := strconv.Atoi(purchaseIDStr)
	if err != nil {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectInvalidPurchaseID, http.StatusBadRequest, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
		)
		http.Error(w, "Invalid purchase id", http.StatusBadRequest)
		return
	}
	purchaseID64 := int64(purchaseID)

	// === OWNERSHIP CHECK BEFORE READING FILE BODY ===
	// Do this first to avoid loading up to 10MB into RAM for unauthorized requests.
	purchase, err := h.purchaseByID(r.Context(), purchaseID64)
	if err != nil {
		logUploadScreenshotReject(slog.LevelError, r, uploadRejectPurchaseLookupFailed, http.StatusNotFound, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"purchase_id", utils.MaskHalfInt64(purchaseID64),
		)
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	if purchase == nil {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectPurchaseNotFound, http.StatusNotFound, nil,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"purchase_id", utils.MaskHalfInt64(purchaseID64),
		)
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	customer, err := h.customerByTelegramID(r.Context(), telegramID)
	if err != nil {
		logUploadScreenshotReject(slog.LevelError, r, uploadRejectCustomerLookupFailed, http.StatusNotFound, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_customer_id", utils.MaskHalfInt64(purchase.CustomerID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
		)
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	if status, message, reason, ok := validateScreenshotUploadAccess(purchase, customer, telegramID); !ok {
		attrs := []any{
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_customer_id", utils.MaskHalfInt64(purchase.CustomerID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
		}
		if customer != nil {
			attrs = append(attrs, "customer_id", utils.MaskHalfInt64(customer.ID))
		}
		logUploadScreenshotReject(slog.LevelWarn, r, reason, status, nil, attrs...)
		http.Error(w, message, status)
		return
	}
	if err := h.beginScreenshotVerification(purchase.ID, customer.ID); err != nil {
		if errors.Is(err, errScreenshotVerificationRateLimited) {
			logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectVerificationRateLimited, http.StatusTooManyRequests, err,
				"telegram_id", utils.MaskHalfInt64(telegramID),
				"customer_id", utils.MaskHalfInt64(customer.ID),
				"purchase_id", utils.MaskHalfInt64(purchase.ID),
				"purchase_status", purchase.Status,
				"invoice_type", purchase.InvoiceType,
			)
			http.Error(w, "Too many verification attempts. Please wait a few minutes and try again.", http.StatusTooManyRequests)
			return
		}
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectVerificationInProgress, http.StatusTooManyRequests, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
		)
		http.Error(w, "Verification is temporarily unavailable for this purchase", http.StatusTooManyRequests)
		return
	}
	defer h.finishScreenshotVerification(purchase.ID)

	// Now safe to read the file. Limit body to 10 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectInvalidMultipartForm, http.StatusBadRequest, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
		)
		http.Error(w, "File too big or invalid form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectMissingFileField, http.StatusBadRequest, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
		)
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectFileReadFailed, http.StatusBadRequest, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
		)
		http.Error(w, "Error reading file", http.StatusBadRequest)
		return
	}

	mimeType, ok := trustedPaymentScreenshotMIME(fileBytes)
	if !ok {
		logUploadScreenshotReject(slog.LevelWarn, r, uploadRejectUnsupportedFileType, http.StatusBadRequest, nil,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
			"mime_type", mimeType,
			"file_size", len(fileBytes),
		)
		http.Error(w, "Unsupported file type", http.StatusBadRequest)
		return
	}

	result, err := h.paymentService.VerifyMobilePayment(r.Context(), int64(purchaseID), fileBytes, mimeType)
	if err != nil {
		logUploadScreenshotReject(slog.LevelError, r, uploadRejectVerificationServiceFailed, http.StatusInternalServerError, err,
			"telegram_id", utils.MaskHalfInt64(telegramID),
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"purchase_id", utils.MaskHalfInt64(purchase.ID),
			"purchase_status", purchase.Status,
			"invoice_type", purchase.InvoiceType,
			"mime_type", mimeType,
			"file_size", len(fileBytes),
		)
		writeSanitizedError(w, http.StatusInternalServerError, "Verification is temporarily unavailable right now. Please try again.", err)
		return
	}

	resp := UploadScreenshotResponse{Status: "failed"}
	if result.Success {
		resp.Status = "success"
		resp.Message = "Payment verified successfully!"
		if strings.TrimSpace(result.Reason) != "" {
			resp.Message = result.Reason
		}
		if result.TestModeBypass {
			resp.TestMode = true
			resp.ShadowPassed = result.ShadowPassed
		}
		// Look up the latest subscription key for this customer to build Happ deep link
		if h.subKeyRepo != nil {
			if purchase.ExtendKeyID != nil {
				if extendKey, kErr := h.subKeyRepo.FindByID(r.Context(), *purchase.ExtendKeyID); kErr == nil && extendKey != nil && extendKey.SubscriptionURL != "" {
					resp.HappLink = "happ://add/" + extendKey.SubscriptionURL
					resp.RedirectURL = signedRedirectURLForTarget(resp.HappLink)
				}
			} else {
				keys, kErr := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
				if kErr == nil && len(keys) > 0 {
					latestKey := keys[0]
					resp.HappLink = "happ://add/" + latestKey.SubscriptionURL
					resp.RedirectURL = signedRedirectURLForTarget(resp.HappLink)
				}
			}
		}
	} else {
		resp = uploadScreenshotFailureResponse(result)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) CancelPurchase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	purchaseIDStr := r.URL.Query().Get("id")
	if purchaseIDStr == "" {
		http.Error(w, "Missing purchase id", http.StatusBadRequest)
		return
	}
	purchaseID, err := strconv.Atoi(purchaseIDStr)
	if err != nil {
		http.Error(w, "Invalid purchase id", http.StatusBadRequest)
		return
	}

	purchase, err := h.paymentService.GetPurchaseByID(r.Context(), int64(purchaseID))
	if err != nil || purchase == nil {
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	if status, message, ok := validatePendingPurchaseCancellationAccess(purchase, customer, telegramID); !ok {
		http.Error(w, message, status)
		return
	}
	if h.screenshotVerificationInFlight(purchase.ID) {
		http.Error(w, "Verification is already in progress for this purchase", http.StatusConflict)
		return
	}

	cancelled, err := h.paymentService.CancelAwaitingVerificationPurchase(r.Context(), purchase.ID, customer.ID)
	if err != nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Unable to cancel purchase right now. Please try again.", err)
		return
	}
	if !cancelled {
		http.Error(w, "Purchase is not awaiting verification", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CancelPurchaseResponse{
		PurchaseID: purchase.ID,
		Status:     string(database.PurchaseStatusCancel),
	})
}

func (h *APIHandler) GetPurchaseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	purchaseIDStr := r.URL.Query().Get("id")
	if purchaseIDStr == "" {
		http.Error(w, "Missing purchase id", http.StatusBadRequest)
		return
	}
	purchaseID, err := strconv.Atoi(purchaseIDStr)
	if err != nil {
		http.Error(w, "Invalid purchase id", http.StatusBadRequest)
		return
	}

	purchase, err := h.paymentService.GetPurchaseByID(r.Context(), int64(purchaseID))
	if err != nil || purchase == nil {
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}
	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil || customer == nil || purchase.CustomerID != customer.ID {
		http.Error(w, "Purchase not found", http.StatusNotFound)
		return
	}

	resp := PurchaseStatusResponse{
		ID:     purchase.ID,
		Status: string(purchase.Status),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) parseReportQuery(r *http.Request) (reporting.ReportQuery, error) {
	period, err := database.NormalizeRevenueSummaryPeriod(r.URL.Query().Get("period"))
	if err != nil {
		return reporting.ReportQuery{}, fmt.Errorf("%w: period must be day, week, month, year, or custom", reporting.ErrInvalidReportQuery)
	}
	q := reporting.ReportQuery{Period: period, Now: h.currentTime()}
	if period == database.RevenuePeriodCustom {
		fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
		toStr := strings.TrimSpace(r.URL.Query().Get("to"))
		if fromStr == "" || toStr == "" {
			return reporting.ReportQuery{}, fmt.Errorf("%w: custom period requires from and to (YYYY-MM-DD)", reporting.ErrInvalidReportQuery)
		}
		loc := reporting.YangonLocation()
		fromDay, err := time.ParseInLocation("2006-01-02", fromStr, loc)
		if err != nil {
			return reporting.ReportQuery{}, fmt.Errorf("%w: from must be YYYY-MM-DD", reporting.ErrInvalidReportQuery)
		}
		toDay, err := time.ParseInLocation("2006-01-02", toStr, loc)
		if err != nil {
			return reporting.ReportQuery{}, fmt.Errorf("%w: to must be YYYY-MM-DD", reporting.ErrInvalidReportQuery)
		}
		q.CustomFrom = &fromDay
		q.CustomTo = &toDay
		return q, nil
	}
	// periods takes precedence; days maps to periods for day period only (compat)
	if p := r.URL.Query().Get("periods"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return reporting.ReportQuery{}, fmt.Errorf("%w: periods must be a positive integer", reporting.ErrInvalidReportQuery)
		}
		q.HistoryPeriods = n
	} else if d := r.URL.Query().Get("days"); d != "" && period == database.RevenuePeriodDay {
		n, err := strconv.Atoi(d)
		if err != nil || n < 1 {
			return reporting.ReportQuery{}, fmt.Errorf("%w: days must be a positive integer", reporting.ErrInvalidReportQuery)
		}
		q.HistoryPeriods = n
	}
	return q, nil
}

func (h *APIHandler) GetRevenueSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Validate query before service availability so parse failures stay 400.
	q, err := h.parseReportQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.financeService == nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to fetch revenue", fmt.Errorf("finance service nil"))
		return
	}
	report, err := h.financeService.GetReport(r.Context(), q)
	if err != nil {
		if errors.Is(err, reporting.ErrInvalidReportQuery) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to fetch revenue", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func (h *APIHandler) ExportRevenue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Validate query before service availability so parse failures stay 400.
	q, err := h.parseReportQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.financeService == nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to export revenue", fmt.Errorf("finance service nil"))
		return
	}
	report, err := h.financeService.GetReport(r.Context(), q)
	if err != nil {
		if errors.Is(err, reporting.ErrInvalidReportQuery) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to export revenue", err)
		return
	}
	csvBytes, err := reporting.FormatFinanceReportCSV(report)
	if err != nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to export revenue", err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="finance-report.csv"`)
	_, _ = w.Write(csvBytes)
}

// --- Wallet Handlers ---

func maskHalfInt64(id int64) string {
	s := strconv.FormatInt(id, 10)
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	maskLen := len(s) / 2
	visibleLen := len(s) - maskLen
	return strings.Repeat("*", maskLen) + s[visibleLen:]
}

func (h *APIHandler) GetReferrals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	type ReferralItem struct {
		ID        int64     `json:"id"`
		MaskedID  string    `json:"masked_id"`
		CreatedAt time.Time `json:"created_at"`
		Status    string    `json:"status"` // "bonus_received" or "pending"
	}

	var items []ReferralItem
	if h.referralRepo != nil {
		refs, err := h.referralRepo.FindByReferrerAny(r.Context(), database.ReferralIdentityValues(customer.ID, customer.TelegramID)...)
		if err != nil {
			writeSanitizedError(w, http.StatusInternalServerError, "Failed to load referrals", err)
			return
		}
		refs, err = database.NormalizeReferralsByReferee(r.Context(), refs, h.customerRepo)
		if err != nil {
			writeSanitizedError(w, http.StatusInternalServerError, "Failed to load referrals", err)
			return
		}
		for _, ref := range refs {
			status := "pending"
			if ref.BonusGranted {
				status = "bonus_received"
			}
			items = append(items, ReferralItem{
				ID:        ref.ID,
				MaskedID:  h.referralMaskedTelegramID(r.Context(), ref.RefereeID),
				CreatedAt: referralActivityAt(ref),
				Status:    status,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func (h *APIHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	balance, err := h.walletService.GetBalance(r.Context(), customer.ID)
	if err != nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to get balance", err)
		return
	}

	autoRenew := false
	if h.subKeyRepo != nil {
		keys, err := h.subKeyRepo.FindByCustomerID(r.Context(), customer.ID)
		if err != nil {
			writeSanitizedError(w, http.StatusInternalServerError, "Failed to get auto-renew status", err)
			return
		}
		for _, key := range keys {
			if key.AutoRenew {
				autoRenew = true
				break
			}
		}
	}

	referralCount, referralEarned, referralStatsUnavailable := h.referralSummary(r.Context(), customer)

	resp := map[string]interface{}{
		"balance":                    balance,
		"currency":                   config.Currency(),
		"auto_renew":                 autoRenew,
		"auto_renew_duration":        nil,
		"bot_url":                    config.BotURL(),
		"referral_bonus_amount":      payment.ReferralBonusAmount,
		"referral_stats_unavailable": referralStatsUnavailable,
	}
	if !referralStatsUnavailable {
		resp["referral_count"] = referralCount
		resp["referral_earned"] = referralEarned
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *APIHandler) GetWalletHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	history, err := h.walletService.GetTransactionHistory(r.Context(), customer.ID, limit)
	if err != nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to get transaction history", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func (h *APIHandler) UpdateAutoRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Error(w, "Customer-wide auto-renew has been removed. Please enable auto-renew on a specific key from the home screen.", http.StatusGone)
}

// UpdateKeyAutoRenew toggles the auto_renew flag on a specific subscription key.
// POST /api/keys/autorenew
// Body: { "key_id": 42, "enabled": true }
func (h *APIHandler) UpdateKeyAutoRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		KeyID   int64 `json:"key_id"`
		Enabled bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.KeyID == 0 {
		http.Error(w, "Missing key_id", http.StatusBadRequest)
		return
	}

	customer, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if customer == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// SetKeyAutoRenew validates that key.customer_id == customer.ID internally.
	if err := h.walletService.SetKeyAutoRenew(r.Context(), req.KeyID, customer.ID, req.Enabled); err != nil {
		if errors.Is(err, walletsvc.ErrAutoRenewPlanUnknown) {
			http.Error(w, "This key needs one manual renewal before auto-renew can be enabled.", http.StatusConflict)
			return
		}
		writeSanitizedError(w, http.StatusBadRequest, "Failed to update key auto-renew", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

type createFinancialAdjustmentRequest struct {
	PurchaseID     *int64  `json:"purchase_id"`
	AdjustmentType string  `json:"adjustment_type"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	EffectiveAt    *string `json:"effective_at"`
	Reason         string  `json:"reason"`
	ExternalRef    string  `json:"external_ref"`
	IdempotencyKey string  `json:"idempotency_key"`
}

func (h *APIHandler) CreateFinancialAdjustment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.financialAdjustmentRepo == nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Financial adjustments unavailable", fmt.Errorf("repo nil"))
		return
	}
	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var req createFinancialAdjustmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		http.Error(w, "idempotency_key is required", http.StatusBadRequest)
		return
	}
	if req.AdjustmentType != string(database.FinancialAdjustmentTypeRefund) {
		http.Error(w, "adjustment_type must be refund", http.StatusBadRequest)
		return
	}
	if req.Amount <= 0 {
		http.Error(w, "amount must be positive", http.StatusBadRequest)
		return
	}
	effectiveAt := h.currentTime()
	if req.EffectiveAt != nil && strings.TrimSpace(*req.EffectiveAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.EffectiveAt))
		if err != nil {
			http.Error(w, "effective_at must be RFC3339", http.StatusBadRequest)
			return
		}
		effectiveAt = parsed
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "MMK"
	}
	row, created, err := h.financialAdjustmentRepo.Create(r.Context(), database.CreateFinancialAdjustmentInput{
		PurchaseID:     req.PurchaseID,
		AdjustmentType: database.FinancialAdjustmentTypeRefund,
		Amount:         req.Amount,
		Currency:       currency,
		EffectiveAt:    effectiveAt,
		Reason:         req.Reason,
		ExternalRef:    req.ExternalRef,
		CreatedBy:      fmt.Sprintf("admin:%d", telegramID),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	})
	if err != nil {
		switch {
		case errors.Is(err, database.ErrFinancialAdjustmentForeignKey),
			errors.Is(err, database.ErrFinancialAdjustmentCheck):
			writeSanitizedError(w, http.StatusBadRequest, "Invalid financial adjustment", err)
			return
		case errors.Is(err, database.ErrFinancialAdjustmentUnique),
			errors.Is(err, database.ErrFinancialAdjustmentIdempotencyConflict):
			writeSanitizedError(w, http.StatusConflict, "Financial adjustment conflict", err)
			return
		default:
			writeSanitizedError(w, http.StatusInternalServerError, "Failed to create financial adjustment", err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(row)
}
