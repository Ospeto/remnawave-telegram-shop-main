package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"sync"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type Plan struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	SortOrder      int    `json:"sort_order"`
	Active         bool   `json:"active"`
}

type config struct {
	telegramToken                                             string
	plans                                                     []Plan
	plansMu                                                   sync.RWMutex
	remnawaveUrl, remnawaveToken, remnawaveMode, remnawaveTag string
	defaultLanguage                                           string
	databaseURL                                               string
	cryptoPayURL, cryptoPayToken                              string
	botURL                                                    string
	trialTrafficLimit                                         int
	feedbackURL                                               string
	channelURL                                                string
	serverStatusURL                                           string
	supportURL                                                string
	tosURL                                                    string
	isCryptoEnabled                                           bool
	adminTelegramId                                           int64
	trialDays                                                 int
	trialRemnawaveTag                                         string
	squadUUIDs                                                map[uuid.UUID]uuid.UUID
	referralDays                                              int
	miniApp                                                   string
	enableAutoPayment                                         bool
	healthCheckPort                                           int
	isWebAppLinkEnabled                                       bool
	externalSquadUUID                                         uuid.UUID
	blockedTelegramIds                                        map[int64]bool
	whitelistedTelegramIds                                    map[int64]bool
	trialInternalSquads                                       map[uuid.UUID]uuid.UUID
	trialExternalSquadUUID                                    uuid.UUID
	remnawaveHeaders                                          map[string]string
	trialTrafficLimitResetStrategy                            string
	trafficLimitResetStrategy                                 string
	mobileBankingEnabled                                      bool
	mobileBankingPhone                                        string
	visionProviderPrimary                                     string
	geminiAPIKey                                              string
	geminiModel                                               string
	openRouterAPIKey                                          string
	openRouterModel                                           string
	openRouterFallbackModel                                   string
	visionProviderFallback                                    string
	visionRetryAttempts                                       int
	visionMaxAttempts                                         int
	visionAcceptConfidenceThreshold                           float64
	visionRejectConfidenceThreshold                           float64
	currency                                                  string
	backupEnabled                                             bool
	backupScheduleCron                                        string
	backupTimezone                                            string
	backupDir                                                 string
	backupRetentionDays                                       int
	backupMaxLocalFiles                                       int
	backupSendToTelegram                                      bool
	backupRestoreEnabled                                      bool
	backupConfirmTTLMinutes                                   int
	backupJobTimeoutSeconds                                   int
	backupRestoreTimeoutSeconds                               int
	botURLMu                                                  sync.RWMutex
}

var conf config

func RemnawaveTag() string {
	return conf.remnawaveTag
}

func TrialRemnawaveTag() string {
	if conf.trialRemnawaveTag != "" {
		return conf.trialRemnawaveTag
	}
	return conf.remnawaveTag
}

func DefaultLanguage() string {
	if conf.defaultLanguage == "" {
		return "en"
	}
	return conf.defaultLanguage
}

func GetReferralDays() int {
	return conf.referralDays
}

func GetMiniAppURL() string {
	return conf.miniApp
}

func SquadUUIDs() map[uuid.UUID]uuid.UUID {
	return conf.squadUUIDs
}

func GetBlockedTelegramIds() map[int64]bool {
	return conf.blockedTelegramIds
}

func GetWhitelistedTelegramIds() map[int64]bool {
	return conf.whitelistedTelegramIds
}

func TrialInternalSquads() map[uuid.UUID]uuid.UUID {
	if conf.trialInternalSquads != nil && len(conf.trialInternalSquads) > 0 {
		return conf.trialInternalSquads
	}
	return conf.squadUUIDs
}

func TrialExternalSquadUUID() uuid.UUID {
	if conf.trialExternalSquadUUID != uuid.Nil {
		return conf.trialExternalSquadUUID
	}
	return conf.externalSquadUUID
}

func TrialTrafficLimit() int {
	return conf.trialTrafficLimit * bytesInGigabyte
}

func TrialDays() int {
	return conf.trialDays
}

func SetTrialConfigForTesting(days int, trafficLimitGB int) func() {
	oldDays := conf.trialDays
	oldTrafficLimit := conf.trialTrafficLimit
	conf.trialDays = days
	conf.trialTrafficLimit = trafficLimitGB

	return func() {
		conf.trialDays = oldDays
		conf.trialTrafficLimit = oldTrafficLimit
	}
}

func SetSupportURLForTesting(url string) func() {
	oldURL := conf.supportURL
	conf.supportURL = url

	return func() {
		conf.supportURL = oldURL
	}
}

func FeedbackURL() string {
	return conf.feedbackURL
}

func ChannelURL() string {
	return conf.channelURL
}

func ServerStatusURL() string {
	return conf.serverStatusURL
}

func SupportURL() string {
	return conf.supportURL
}

func TosURL() string {
	return conf.tosURL
}

func Plans() []Plan {
	return ActivePlans()
}

func LowestPlanPrice() int {
	plans := ActivePlans()
	if len(plans) == 0 {
		return 6000
	}
	minPrice := plans[0].Price
	for _, plan := range plans {
		if plan.Price < minPrice {
			minPrice = plan.Price
		}
	}
	return minPrice
}

func ExternalSquadUUID() uuid.UUID {
	return conf.externalSquadUUID
}

func TelegramToken() string {
	return conf.telegramToken
}
func RemnawaveUrl() string {
	return conf.remnawaveUrl
}
func DatabaseUrl() string {
	return conf.databaseURL
}
func RemnawaveToken() string {
	return conf.remnawaveToken
}
func RemnawaveMode() string {
	return conf.remnawaveMode
}
func CryptoPayUrl() string {
	return conf.cryptoPayURL
}
func CryptoPayToken() string {
	return conf.cryptoPayToken
}
func BotURL() string {
	conf.botURLMu.RLock()
	defer conf.botURLMu.RUnlock()
	return conf.botURL
}
func SetBotURL(botURL string) {
	conf.botURLMu.Lock()
	defer conf.botURLMu.Unlock()
	conf.botURL = botURL
}
func TrafficLimitResetStrategy() string {
	return conf.trafficLimitResetStrategy
}

func IsMobileBankingEnabled() bool {
	return conf.mobileBankingEnabled
}

func MobileBankingPhone() string {
	return conf.mobileBankingPhone
}

func GeminiAPIKey() string {
	return conf.geminiAPIKey
}

func GeminiModel() string {
	return conf.geminiModel
}

func OpenRouterAPIKey() string {
	return conf.openRouterAPIKey
}

func OpenRouterModel() string {
	return conf.openRouterModel
}

func OpenRouterFallbackModel() string {
	return conf.openRouterFallbackModel
}

func VisionProviderPrimary() string {
	return conf.visionProviderPrimary
}

func VisionProviderFallback() string {
	return conf.visionProviderFallback
}

func VisionRetryAttempts() int {
	return conf.visionRetryAttempts
}

func VisionMaxAttempts() int {
	return conf.visionMaxAttempts
}

func VisionAcceptConfidenceThreshold() float64 {
	return conf.visionAcceptConfidenceThreshold
}

func VisionRejectConfidenceThreshold() float64 {
	return conf.visionRejectConfidenceThreshold
}

func Currency() string {
	return conf.currency
}

func BackupEnabled() bool {
	return conf.backupEnabled
}

func BackupScheduleCron() string {
	return conf.backupScheduleCron
}

func BackupTimezone() string {
	return conf.backupTimezone
}

func BackupDir() string {
	return conf.backupDir
}

func BackupRetentionDays() int {
	return conf.backupRetentionDays
}

func BackupMaxLocalFiles() int {
	return conf.backupMaxLocalFiles
}

func BackupSendToTelegram() bool {
	return conf.backupSendToTelegram
}

func BackupRestoreEnabled() bool {
	return conf.backupRestoreEnabled
}

func BackupConfirmTTLMinutes() int {
	return conf.backupConfirmTTLMinutes
}

func BackupJobTimeoutSeconds() int {
	return conf.backupJobTimeoutSeconds
}

func BackupRestoreTimeoutSeconds() int {
	return conf.backupRestoreTimeoutSeconds
}

func IsCryptoPayEnabled() bool {
	return conf.isCryptoEnabled
}

func GetAdminTelegramId() int64 {
	return conf.adminTelegramId
}

func GetHealthCheckPort() int {
	return conf.healthCheckPort
}

func IsWebAppLinkEnabled() bool {
	return conf.isWebAppLinkEnabled
}

func RemnawaveHeaders() map[string]string {
	return conf.remnawaveHeaders
}

func TrialTrafficLimitResetStrategy() string {
	return conf.trialTrafficLimitResetStrategy
}

const bytesInGigabyte = 1073741824

func requiredEnv(key string) (string, error) {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func requiredEnvInt(key string) (int, error) {
	v, err := requiredEnv(key)
	if err != nil {
		return 0, err
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return i, nil
}

func envIntDefault(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return i, nil
}

func envStringDefault(key string, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func envBool(key string) bool {
	return os.Getenv(key) == "true"
}

func envOptionalInt(key string) (int, bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return 0, false, nil
	}

	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, false, fmt.Errorf("invalid %s: %w", key, err)
	}
	return i, true, nil
}

func envFloatDefault(key string, def float64) (float64, error) {
	v := os.Getenv(key)
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return f, nil
}

func normalizeVisionProvider(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "gemini":
		return "gemini", nil
	case "openrouter":
		return "openrouter", nil
	default:
		return "", fmt.Errorf("unsupported VISION_PROVIDER_FALLBACK %q", value)
	}
}

func resolveVisionProviders(geminiAPIKey, openRouterAPIKey, openRouterFallbackModel, fallbackRaw string) (string, string, error) {
	hasGemini := strings.TrimSpace(geminiAPIKey) != ""
	hasOpenRouter := strings.TrimSpace(openRouterAPIKey) != ""
	hasOpenRouterFallback := strings.TrimSpace(openRouterFallbackModel) != ""

	if !hasGemini && !hasOpenRouter {
		return "", "", fmt.Errorf("mobile banking requires OPENROUTER_API_KEY or GEMINI_API_KEY")
	}

	primary := "gemini"
	if hasOpenRouter {
		primary = "openrouter"
	}

	fallback := ""
	if strings.TrimSpace(fallbackRaw) == "" {
		return primary, fallback, nil
	}

	normalizedFallback, err := normalizeVisionProvider(fallbackRaw)
	if err != nil {
		return "", "", err
	}
	if normalizedFallback == "openrouter" && primary == "openrouter" && !hasOpenRouterFallback {
		return "", "", fmt.Errorf("VISION_PROVIDER_FALLBACK=openrouter requires OPENROUTER_FALLBACK_MODEL")
	}
	if normalizedFallback == primary && !(normalizedFallback == "openrouter" && hasOpenRouterFallback) {
		return primary, "", nil
	}

	switch normalizedFallback {
	case "gemini":
		if !hasGemini {
			return "", "", fmt.Errorf("VISION_PROVIDER_FALLBACK=gemini requires GEMINI_API_KEY")
		}
	case "openrouter":
		if !hasOpenRouter {
			return "", "", fmt.Errorf("VISION_PROVIDER_FALLBACK=openrouter requires OPENROUTER_API_KEY")
		}
		if !hasOpenRouterFallback {
			return "", "", fmt.Errorf("VISION_PROVIDER_FALLBACK=openrouter requires OPENROUTER_FALLBACK_MODEL")
		}
	}

	return primary, normalizedFallback, nil
}

func InitConfig() (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("load config: %v", recovered)
		}
	}()

	if os.Getenv("DISABLE_ENV_FILE") != "true" {
		if err := godotenv.Load(".env"); err != nil {
			slog.Info("No .env loaded", "error", err)
		}
	}

	adminTelegramID := strings.TrimSpace(os.Getenv("ADMIN_TELEGRAM_ID"))
	if adminTelegramID == "" {
		return fmt.Errorf("ADMIN_TELEGRAM_ID is required")
	}
	conf.adminTelegramId, err = strconv.ParseInt(adminTelegramID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ADMIN_TELEGRAM_ID: %w", err)
	}

	conf.telegramToken, err = requiredEnv("TELEGRAM_TOKEN")
	if err != nil {
		return err
	}

	conf.isWebAppLinkEnabled = func() bool {
		isWebAppLinkEnabled := os.Getenv("IS_WEB_APP_LINK") == "true"
		return isWebAppLinkEnabled
	}()

	conf.miniApp = envStringDefault("MINI_APP_URL", "")

	conf.remnawaveTag = strings.ToUpper(envStringDefault("REMNAWAVE_TAG", ""))

	conf.trialRemnawaveTag = strings.ToUpper(envStringDefault("TRIAL_REMNAWAVE_TAG", ""))

	conf.trialTrafficLimitResetStrategy = envStringDefault("TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")
	conf.trafficLimitResetStrategy = envStringDefault("TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")

	conf.defaultLanguage = envStringDefault("DEFAULT_LANGUAGE", "en")

	externalSquadUUIDStr := os.Getenv("EXTERNAL_SQUAD_UUID")
	if externalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(externalSquadUUIDStr)
		if err != nil {
			return fmt.Errorf("invalid EXTERNAL_SQUAD_UUID: %w", err)
		}
		conf.externalSquadUUID = parsedUUID
	} else {
		conf.externalSquadUUID = uuid.Nil
	}

	conf.trialTrafficLimit, err = requiredEnvInt("TRIAL_TRAFFIC_LIMIT")
	if err != nil {
		return err
	}

	conf.healthCheckPort, err = envIntDefault("HEALTH_CHECK_PORT", 8080)
	if err != nil {
		return err
	}

	conf.trialDays, err = requiredEnvInt("TRIAL_DAYS")
	if err != nil {
		return err
	}

	conf.enableAutoPayment = envBool("ENABLE_AUTO_PAYMENT")

	// Parse PLANS env var: label|days|price|traffic_gb,...
	plansStr := os.Getenv("PLANS")
	if plansStr != "" {
		plans, err := ParsePlansEnv(plansStr)
		if err != nil {
			return fmt.Errorf("parse PLANS: %w", err)
		}
		setPlans(plans)
		slog.Info("Loaded plans from PLANS env", "count", len(plans))
	} else {
		// Backward compat: fall back to PRICE_1/3/6/12
		daysInMonth, err := envIntDefault("DAYS_IN_MONTH", 30)
		if err != nil {
			return err
		}
		trafficLimit, err := envIntDefault("TRAFFIC_LIMIT", 0)
		if err != nil {
			return err
		}
		label := envStringDefault("PLAN_LABEL", "Unlimited")
		var plans []Plan
		for _, m := range []int{1, 3, 6, 12} {
			key := fmt.Sprintf("PRICE_%d", m)
			pStr := os.Getenv(key)
			if pStr == "" {
				continue
			}
			p, err := strconv.Atoi(pStr)
			if err != nil {
				return fmt.Errorf("invalid %s: %w", key, err)
			}
			if p > 0 {
				plans = append(plans, Plan{
					ID:             uuid.NewString(),
					Label:          label,
					Days:           m * daysInMonth,
					Price:          p,
					TrafficLimitGB: trafficLimit,
					SortOrder:      len(plans),
					Active:         true,
				})
			}
		}
		setPlans(plans)
		slog.Info("Loaded plans from PRICE_X env (legacy)", "count", len(plans))
	}

	conf.remnawaveUrl, err = requiredEnv("REMNAWAVE_URL")
	if err != nil {
		return err
	}

	conf.remnawaveMode = envStringDefault("REMNAWAVE_MODE", "remote")
	if conf.remnawaveMode != "remote" && conf.remnawaveMode != "local" {
		return fmt.Errorf("REMNAWAVE_MODE must be either remote or local")
	}

	conf.remnawaveToken, err = requiredEnv("REMNAWAVE_TOKEN")
	if err != nil {
		return err
	}

	conf.databaseURL, err = requiredEnv("DATABASE_URL")
	if err != nil {
		return err
	}

	conf.isCryptoEnabled = envBool("CRYPTO_PAY_ENABLED")
	if conf.isCryptoEnabled {
		conf.cryptoPayURL, err = requiredEnv("CRYPTO_PAY_URL")
		if err != nil {
			return err
		}
		conf.cryptoPayToken, err = requiredEnv("CRYPTO_PAY_TOKEN")
		if err != nil {
			return err
		}
	}

	conf.referralDays, err = requiredEnvInt("REFERRAL_DAYS")
	if err != nil {
		return err
	}

	conf.serverStatusURL = os.Getenv("SERVER_STATUS_URL")
	conf.supportURL = os.Getenv("SUPPORT_URL")
	conf.feedbackURL = os.Getenv("FEEDBACK_URL")
	conf.channelURL = os.Getenv("CHANNEL_URL")
	conf.tosURL = os.Getenv("TOS_URL")

	conf.squadUUIDs = map[uuid.UUID]uuid.UUID{}
	if v := os.Getenv("SQUAD_UUIDS"); v != "" {
		uuids := strings.Split(v, ",")
		for _, value := range uuids {
			parsedUUID, err := uuid.Parse(strings.TrimSpace(value))
			if err != nil {
				return fmt.Errorf("invalid SQUAD_UUIDS: %w", err)
			}
			conf.squadUUIDs[parsedUUID] = parsedUUID
		}
		slog.Info("Loaded squad UUIDs", "uuids", uuids)
	} else {
		slog.Info("No squad UUIDs specified, all will be used")
	}

	conf.blockedTelegramIds = map[int64]bool{}
	if v := os.Getenv("BLOCKED_TELEGRAM_IDS"); v != "" {
		ids := strings.Split(v, ",")
		for _, idStr := range ids {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid BLOCKED_TELEGRAM_IDS: %w", err)
			}
			conf.blockedTelegramIds[id] = true
		}
		slog.Info("Loaded blocked telegram IDs", "count", len(conf.blockedTelegramIds))
	} else {
		slog.Info("No blocked telegram IDs specified")
	}

	conf.whitelistedTelegramIds = map[int64]bool{}
	if v := os.Getenv("WHITELISTED_TELEGRAM_IDS"); v != "" {
		ids := strings.Split(v, ",")
		for _, idStr := range ids {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid WHITELISTED_TELEGRAM_IDS: %w", err)
			}
			conf.whitelistedTelegramIds[id] = true
		}
		slog.Info("Loaded whitelisted telegram IDs", "count", len(conf.whitelistedTelegramIds))
	} else {
		slog.Info("No whitelisted telegram IDs specified")
	}

	conf.trialInternalSquads = map[uuid.UUID]uuid.UUID{}
	if v := os.Getenv("TRIAL_INTERNAL_SQUADS"); v != "" {
		uuids := strings.Split(v, ",")
		for _, value := range uuids {
			parsedUUID, err := uuid.Parse(strings.TrimSpace(value))
			if err != nil {
				return fmt.Errorf("invalid TRIAL_INTERNAL_SQUADS: %w", err)
			}
			conf.trialInternalSquads[parsedUUID] = parsedUUID
		}
		slog.Info("Loaded trial internal squad UUIDs", "uuids", uuids)
	} else {
		slog.Info("No trial internal squads specified, will use regular SQUAD_UUIDS for trial users")
	}

	trialExternalSquadUUIDStr := os.Getenv("TRIAL_EXTERNAL_SQUAD_UUID")
	if trialExternalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(trialExternalSquadUUIDStr)
		if err != nil {
			return fmt.Errorf("invalid TRIAL_EXTERNAL_SQUAD_UUID: %w", err)
		}
		conf.trialExternalSquadUUID = parsedUUID
		slog.Info("Loaded trial external squad UUID", "uuid", trialExternalSquadUUIDStr)
	} else {
		conf.trialExternalSquadUUID = uuid.Nil
		slog.Info("No trial external squad specified, will use regular EXTERNAL_SQUAD_UUID for trial users")
	}

	conf.remnawaveHeaders = func() map[string]string {
		v := os.Getenv("REMNAWAVE_HEADERS")
		if v != "" {
			headers := make(map[string]string)
			pairs := strings.Split(v, ";")
			for _, pair := range pairs {
				parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if key != "" && value != "" {
						headers[key] = value
					}
				}
			}
			if len(headers) > 0 {
				slog.Info("Loaded remnawave headers", "count", len(headers))
				return headers
			}
		}
		return map[string]string{}
	}()

	conf.mobileBankingEnabled = envBool("MOBILE_BANKING_ENABLED")
	if conf.mobileBankingEnabled {
		conf.mobileBankingPhone, err = requiredEnv("MOBILE_BANKING_PHONE")
		if err != nil {
			return err
		}
		conf.geminiAPIKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		conf.geminiModel = envStringDefault("GEMINI_MODEL", "gemini-3.1-flash-lite-preview")
		conf.openRouterAPIKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
		conf.openRouterModel = envStringDefault("OPENROUTER_MODEL", "openai/gpt-4.1-mini")
		conf.openRouterFallbackModel = strings.TrimSpace(os.Getenv("OPENROUTER_FALLBACK_MODEL"))
		primary, fallback, err := resolveVisionProviders(conf.geminiAPIKey, conf.openRouterAPIKey, conf.openRouterFallbackModel, os.Getenv("VISION_PROVIDER_FALLBACK"))
		if err != nil {
			return err
		}
		conf.visionProviderPrimary = primary
		conf.visionProviderFallback = fallback
		conf.visionRetryAttempts, err = envIntDefault("VISION_RETRY_ATTEMPTS", 1)
		if err != nil {
			return err
		}
		if conf.visionRetryAttempts < 0 {
			return fmt.Errorf("VISION_RETRY_ATTEMPTS must be >= 0")
		}
		explicitMaxAttempts, ok, err := envOptionalInt("VISION_RETRY_MAX_ATTEMPTS")
		if err != nil {
			return err
		}
		if !ok {
			explicitMaxAttempts, ok, err = envOptionalInt("VISION_MAX_ATTEMPTS")
			if err != nil {
				return err
			}
		}
		if ok {
			if explicitMaxAttempts < 1 {
				return fmt.Errorf("VISION_RETRY_MAX_ATTEMPTS/VISION_MAX_ATTEMPTS must be >= 1")
			}
			conf.visionMaxAttempts = explicitMaxAttempts
		} else {
			providerCount := 1
			if conf.visionProviderFallback != "" {
				providerCount++
			}
			conf.visionMaxAttempts = providerCount * (conf.visionRetryAttempts + 1)
		}
		conf.visionAcceptConfidenceThreshold, err = envFloatDefault("VISION_ACCEPT_CONFIDENCE_THRESHOLD", 0.55)
		if err != nil {
			return err
		}
		conf.visionRejectConfidenceThreshold, err = envFloatDefault("VISION_REJECT_CONFIDENCE_THRESHOLD", 0.90)
		if err != nil {
			return err
		}
		if conf.visionAcceptConfidenceThreshold <= 0 || conf.visionAcceptConfidenceThreshold > 1 {
			return fmt.Errorf("VISION_ACCEPT_CONFIDENCE_THRESHOLD must be > 0 and <= 1")
		}
		if conf.visionRejectConfidenceThreshold <= 0 || conf.visionRejectConfidenceThreshold > 1 {
			return fmt.Errorf("VISION_REJECT_CONFIDENCE_THRESHOLD must be > 0 and <= 1")
		}
	}

	conf.currency = envStringDefault("CURRENCY", "MMK")
	conf.backupEnabled = envBool("BACKUP_ENABLED")
	conf.backupScheduleCron = envStringDefault("BACKUP_SCHEDULE_CRON", "10 0 * * *")
	conf.backupTimezone = envStringDefault("BACKUP_TIMEZONE", "Asia/Rangoon")
	conf.backupDir = envStringDefault("BACKUP_DIR", "/backups")
	conf.backupRetentionDays, err = envIntDefault("BACKUP_RETENTION_DAYS", 7)
	if err != nil {
		return err
	}
	conf.backupMaxLocalFiles, err = envIntDefault("BACKUP_MAX_LOCAL_FILES", 7)
	if err != nil {
		return err
	}
	conf.backupSendToTelegram = func() bool {
		if os.Getenv("BACKUP_SEND_TO_TELEGRAM") == "" {
			return true
		}
		return envBool("BACKUP_SEND_TO_TELEGRAM")
	}()
	conf.backupRestoreEnabled = envBool("BACKUP_RESTORE_ENABLED")
	conf.backupConfirmTTLMinutes, err = envIntDefault("BACKUP_CONFIRM_TTL_MINUTES", 10)
	if err != nil {
		return err
	}
	conf.backupJobTimeoutSeconds, err = envIntDefault("BACKUP_JOB_TIMEOUT_SECONDS", 900)
	if err != nil {
		return err
	}
	conf.backupRestoreTimeoutSeconds, err = envIntDefault("BACKUP_RESTORE_TIMEOUT_SECONDS", 1800)
	if err != nil {
		return err
	}
	return nil
}
