package config

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type Plan struct {
	Label          string
	Days           int
	Price          int
	TrafficLimitGB int
}

type config struct {
	telegramToken                                             string
	plans                                                     []Plan
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
	geminiAPIKey                                              string
	geminiModel                                               string
	openRouterAPIKey                                          string
	openRouterModel                                           string
	currency                                                  string
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
	return conf.plans
}

func PlanByIndex(idx int) *Plan {
	if idx < 0 || idx >= len(conf.plans) {
		return nil
	}
	return &conf.plans[idx]
}

func LowestPlanPrice() int {
	if len(conf.plans) == 0 {
		return 6000
	}
	minPrice := conf.plans[0].Price
	for _, plan := range conf.plans {
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
	return conf.botURL
}
func SetBotURL(botURL string) {
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

func Currency() string {
	return conf.currency
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

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Panicf("env %q not set", key)
	}
	return v
}

func mustEnvInt(key string) int {
	v := mustEnv(key)
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Panicf("invalid int in %q: %v", key, err)
	}
	return i
}

func envIntDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		log.Panicf("invalid int in %q: %v", key, err)
	}
	return i
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

func InitConfig() {
	if os.Getenv("DISABLE_ENV_FILE") != "true" {
		if err := godotenv.Load(".env"); err != nil {
			log.Println("No .env loaded:", err)
		}
	}
	var err error
	conf.adminTelegramId, err = strconv.ParseInt(os.Getenv("ADMIN_TELEGRAM_ID"), 10, 64)
	if err != nil {
		panic("ADMIN_TELEGRAM_ID .env variable not set")
	}

	conf.telegramToken = mustEnv("TELEGRAM_TOKEN")

	conf.isWebAppLinkEnabled = func() bool {
		isWebAppLinkEnabled := os.Getenv("IS_WEB_APP_LINK") == "true"
		return isWebAppLinkEnabled
	}()

	conf.miniApp = envStringDefault("MINI_APP_URL", "")

	conf.remnawaveTag = strings.ToUpper(envStringDefault("REMNAWAVE_TAG", ""))

	conf.trialRemnawaveTag = strings.ToUpper(envStringDefault("TRIAL_REMNAWAVE_TAG", ""))

	conf.trialTrafficLimitResetStrategy = envStringDefault("TRIAL_TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")
	conf.trafficLimitResetStrategy = envStringDefault("TRAFFIC_LIMIT_RESET_STRATEGY", "MONTH")

	conf.defaultLanguage = envStringDefault("DEFAULT_LANGUAGE", "ru")

	externalSquadUUIDStr := os.Getenv("EXTERNAL_SQUAD_UUID")
	if externalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(externalSquadUUIDStr)
		if err != nil {
			panic(fmt.Sprintf("invalid EXTERNAL_SQUAD_UUID format: %v", err))
		}
		conf.externalSquadUUID = parsedUUID
	} else {
		conf.externalSquadUUID = uuid.Nil
	}

	conf.trialTrafficLimit = mustEnvInt("TRIAL_TRAFFIC_LIMIT")

	conf.healthCheckPort = envIntDefault("HEALTH_CHECK_PORT", 8080)

	conf.trialDays = mustEnvInt("TRIAL_DAYS")

	conf.enableAutoPayment = envBool("ENABLE_AUTO_PAYMENT")

	// Parse PLANS env var: label|days|price|traffic_gb,...
	plansStr := os.Getenv("PLANS")
	if plansStr != "" {
		for _, entry := range strings.Split(plansStr, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			parts := strings.Split(entry, "|")
			if len(parts) != 4 {
				panic(fmt.Sprintf("invalid PLANS entry %q — expected label|days|price|traffic_gb", entry))
			}
			days, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				panic(fmt.Sprintf("invalid days in PLANS entry %q: %v", entry, err))
			}
			price, err := strconv.Atoi(strings.TrimSpace(parts[2]))
			if err != nil {
				panic(fmt.Sprintf("invalid price in PLANS entry %q: %v", entry, err))
			}
			trafficGB, err := strconv.Atoi(strings.TrimSpace(parts[3]))
			if err != nil {
				panic(fmt.Sprintf("invalid traffic_gb in PLANS entry %q: %v", entry, err))
			}
			conf.plans = append(conf.plans, Plan{
				Label:          strings.TrimSpace(parts[0]),
				Days:           days,
				Price:          price,
				TrafficLimitGB: trafficGB,
			})
		}
		slog.Info("Loaded plans from PLANS env", "count", len(conf.plans))
	} else {
		// Backward compat: fall back to PRICE_1/3/6/12
		daysInMonth := envIntDefault("DAYS_IN_MONTH", 30)
		trafficLimit := envIntDefault("TRAFFIC_LIMIT", 0)
		label := envStringDefault("PLAN_LABEL", "Unlimited")
		for _, m := range []int{1, 3, 6, 12} {
			key := fmt.Sprintf("PRICE_%d", m)
			pStr := os.Getenv(key)
			if pStr == "" {
				continue
			}
			p, err := strconv.Atoi(pStr)
			if err != nil {
				panic(fmt.Sprintf("invalid %s: %v", key, err))
			}
			if p > 0 {
				conf.plans = append(conf.plans, Plan{
					Label:          label,
					Days:           m * daysInMonth,
					Price:          p,
					TrafficLimitGB: trafficLimit,
				})
			}
		}
		slog.Info("Loaded plans from PRICE_X env (legacy)", "count", len(conf.plans))
	}

	conf.remnawaveUrl = mustEnv("REMNAWAVE_URL")

	conf.remnawaveMode = func() string {
		v := os.Getenv("REMNAWAVE_MODE")
		if v != "" {
			if v != "remote" && v != "local" {
				panic("REMNAWAVE_MODE .env variable must be either 'remote' or 'local'")
			} else {
				return v
			}
		} else {
			return "remote"
		}
	}()

	conf.remnawaveToken = mustEnv("REMNAWAVE_TOKEN")

	conf.databaseURL = mustEnv("DATABASE_URL")

	conf.isCryptoEnabled = envBool("CRYPTO_PAY_ENABLED")
	if conf.isCryptoEnabled {
		conf.cryptoPayURL = mustEnv("CRYPTO_PAY_URL")
		conf.cryptoPayToken = mustEnv("CRYPTO_PAY_TOKEN")
	}

	conf.referralDays = mustEnvInt("REFERRAL_DAYS")

	conf.serverStatusURL = os.Getenv("SERVER_STATUS_URL")
	conf.supportURL = os.Getenv("SUPPORT_URL")
	conf.feedbackURL = os.Getenv("FEEDBACK_URL")
	conf.channelURL = os.Getenv("CHANNEL_URL")
	conf.tosURL = os.Getenv("TOS_URL")

	conf.squadUUIDs = func() map[uuid.UUID]uuid.UUID {
		v := os.Getenv("SQUAD_UUIDS")
		if v != "" {
			uuids := strings.Split(v, ",")
			var inboundsMap = make(map[uuid.UUID]uuid.UUID)
			for _, value := range uuids {
				uuid, err := uuid.Parse(value)
				if err != nil {
					panic(err)
				}
				inboundsMap[uuid] = uuid
			}
			slog.Info("Loaded squad UUIDs", "uuids", uuids)
			return inboundsMap
		} else {
			slog.Info("No squad UUIDs specified, all will be used")
			return map[uuid.UUID]uuid.UUID{}
		}
	}()

	conf.blockedTelegramIds = func() map[int64]bool {
		v := os.Getenv("BLOCKED_TELEGRAM_IDS")
		if v != "" {
			ids := strings.Split(v, ",")
			var blockedMap = make(map[int64]bool)
			for _, idStr := range ids {
				id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
				if err != nil {
					panic(fmt.Sprintf("invalid telegram ID in BLOCKED_TELEGRAM_IDS: %v", err))
				}
				blockedMap[id] = true
			}
			slog.Info("Loaded blocked telegram IDs", "count", len(blockedMap))
			return blockedMap
		} else {
			slog.Info("No blocked telegram IDs specified")
			return map[int64]bool{}
		}
	}()

	conf.whitelistedTelegramIds = func() map[int64]bool {
		v := os.Getenv("WHITELISTED_TELEGRAM_IDS")
		if v != "" {
			ids := strings.Split(v, ",")
			var whitelistedMap = make(map[int64]bool)
			for _, idStr := range ids {
				id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
				if err != nil {
					panic(fmt.Sprintf("invalid telegram ID in WHITELISTED_TELEGRAM_IDS: %v", err))
				}
				whitelistedMap[id] = true
			}
			slog.Info("Loaded whitelisted telegram IDs", "count", len(whitelistedMap))
			return whitelistedMap
		} else {
			slog.Info("No whitelisted telegram IDs specified")
			return map[int64]bool{}
		}
	}()

	conf.trialInternalSquads = func() map[uuid.UUID]uuid.UUID {
		v := os.Getenv("TRIAL_INTERNAL_SQUADS")
		if v != "" {
			uuids := strings.Split(v, ",")
			var trialSquadsMap = make(map[uuid.UUID]uuid.UUID)
			for _, value := range uuids {
				parsedUUID, err := uuid.Parse(strings.TrimSpace(value))
				if err != nil {
					panic(fmt.Sprintf("invalid UUID in TRIAL_INTERNAL_SQUADS: %v", err))
				}
				trialSquadsMap[parsedUUID] = parsedUUID
			}
			slog.Info("Loaded trial internal squad UUIDs", "uuids", uuids)
			return trialSquadsMap
		} else {
			slog.Info("No trial internal squads specified, will use regular SQUAD_UUIDS for trial users")
			return map[uuid.UUID]uuid.UUID{}
		}
	}()

	trialExternalSquadUUIDStr := os.Getenv("TRIAL_EXTERNAL_SQUAD_UUID")
	if trialExternalSquadUUIDStr != "" {
		parsedUUID, err := uuid.Parse(trialExternalSquadUUIDStr)
		if err != nil {
			panic(fmt.Sprintf("invalid TRIAL_EXTERNAL_SQUAD_UUID format: %v", err))
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
		conf.mobileBankingPhone = mustEnv("MOBILE_BANKING_PHONE")
		conf.geminiAPIKey = mustEnv("GEMINI_API_KEY")
		conf.geminiModel = envStringDefault("GEMINI_MODEL", "gemini-2.5-flash")
		conf.openRouterAPIKey = os.Getenv("OPENROUTER_API_KEY")
		conf.openRouterModel = envStringDefault("OPENROUTER_MODEL", "google/gemini-2.5-flash")
	}

	conf.currency = envStringDefault("CURRENCY", "MMK")
}
