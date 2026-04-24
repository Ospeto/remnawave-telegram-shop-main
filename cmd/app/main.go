package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"remnawave-tg-shop-bot/internal/api"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/gemini"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/notification"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/reporting"
	"remnawave-tg-shop-bot/internal/service/autorenew"
	"remnawave-tg-shop-bot/internal/service/backup"
	"remnawave-tg-shop-bot/internal/service/healthcheck"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/utils"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/robfig/cron/v3"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isMeaningfulBuildValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized != "" && normalized != "none" && normalized != "unknown" && normalized != "dev"
}

func versionedMiniAppURL(rawURL string) string {
	base := strings.TrimSpace(rawURL)
	if base == "" {
		return ""
	}

	u, err := url.Parse(base)
	if err != nil {
		return base
	}

	version := ""
	for _, candidate := range []string{Commit, BuildDate, Version} {
		if isMeaningfulBuildValue(candidate) {
			version = strings.TrimSpace(candidate)
			break
		}
	}
	if version == "" {
		return base
	}

	query := u.Query()
	query.Set("v", version)
	u.RawQuery = query.Encode()
	return u.String()
}

func sendRevenueReport(ctx context.Context, b *bot.Bot, purchaseRepository *database.PurchaseRepository, jobName, title string, period database.RevenueSummaryPeriod, start, end time.Time) {
	rows, err := purchaseRepository.GetRevenueSummaryRange(ctx, start, end, period)
	if err != nil {
		slog.Error("Revenue report failed", "job", jobName, "error", err)
		return
	}

	text := reporting.FormatTelegramPeriodRevenueReport(title, reporting.FormatDateRange(start, end), rows)
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    config.GetAdminTelegramId(),
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}); err != nil {
		slog.Error("Revenue report send failed", "job", jobName, "error", err)
		return
	}

	totals, _ := reporting.SummarizeRevenuePeriod(rows)
	slog.Info("Revenue report sent", "job", jobName, "service_revenue", totals.ServiceRevenue, "cash_collected", totals.CashCollected, "txns", totals.TotalPurchases)
}

func newVisionProvider(providerName, geminiAPIKey, geminiModel, openRouterAPIKey, openRouterModel, openRouterFallbackModel string) (gemini.Provider, error) {
	switch strings.TrimSpace(providerName) {
	case "":
		return nil, nil
	case "gemini":
		if strings.TrimSpace(geminiAPIKey) == "" {
			return nil, fmt.Errorf("vision provider gemini requires GEMINI_API_KEY")
		}
		return gemini.NewClient(geminiAPIKey, geminiModel), nil
	case "openrouter":
		if strings.TrimSpace(openRouterAPIKey) == "" {
			return nil, fmt.Errorf("vision provider openrouter requires OPENROUTER_API_KEY")
		}
		if strings.TrimSpace(openRouterFallbackModel) != "" {
			return gemini.NewNamedOpenRouterClient("openrouter-fallback", openRouterAPIKey, openRouterFallbackModel), nil
		}
		return gemini.NewOpenRouterClient(openRouterAPIKey, openRouterModel), nil
	default:
		return nil, fmt.Errorf("unsupported vision provider %q", providerName)
	}
}

func visionProviderName(provider gemini.Provider) string {
	if provider == nil {
		return ""
	}
	return provider.Name()
}

func fatalStartup(component string, err error) {
	if err == nil {
		return
	}
	slog.Error("Application startup failed", "component", component, "error", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--health" {
		os.Exit(runHealthProbe())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer cancel()

	if err := config.InitConfig(); err != nil {
		fatalStartup("config", err)
	}
	slog.Info("Application starting", "version", Version, "commit", Commit, "buildDate", BuildDate)

	tm := translation.GetInstance()
	err := tm.InitTranslations("./translations", config.DefaultLanguage())
	if err != nil {
		fatalStartup("translations", err)
	}

	pool, err := initDatabase(ctx, config.DatabaseUrl())
	if err != nil {
		fatalStartup("database", err)
	}
	if err := api.ConfigureStateStores(pool, config.TelegramToken()); err != nil {
		fatalStartup("api state stores", err)
	}

	err = database.RunMigrations(ctx, &database.MigrationConfig{Direction: "up", MigrationsPath: "./db/migrations", Steps: 0}, pool)
	if err != nil {
		fatalStartup("database migrations", err)
	}
	messageCache := cache.NewCache(30 * time.Minute)
	customerRepository := database.NewCustomerRepository(pool)
	purchaseRepository := database.NewPurchaseRepository(pool)
	referralRepository := database.NewReferralRepository(pool)
	promoCodeRepository := database.NewPromoCodeRepository(pool)
	subKeyRepo := database.NewSubscriptionKeyRepository(pool)
	walletTxRepo := database.NewWalletTransactionRepository(pool)

	// Initialize repositories first
	// walletTxRepo is already Init at line 68

	remnawaveClient, err := remnawave.NewClient(config.RemnawaveUrl(), config.RemnawaveToken(), config.RemnawaveMode())
	if err != nil {
		fatalStartup("remnawave client", err)
	}

	// Mobile banking / screenshot analyzer
	var paymentAnalyzer gemini.Analyzer
	var mobilePaymentRepo *database.MobilePaymentRepository
	if config.IsMobileBankingEnabled() {
		primaryProvider, err := newVisionProvider(
			config.VisionProviderPrimary(),
			config.GeminiAPIKey(),
			config.GeminiModel(),
			config.OpenRouterAPIKey(),
			config.OpenRouterModel(),
			"",
		)
		if err != nil {
			fatalStartup("vision primary provider", err)
		}

		fallbackOpenRouterModel := config.OpenRouterFallbackModel()
		fallbackProvider, err := newVisionProvider(
			config.VisionProviderFallback(),
			config.GeminiAPIKey(),
			config.GeminiModel(),
			config.OpenRouterAPIKey(),
			config.OpenRouterModel(),
			fallbackOpenRouterModel,
		)
		if err != nil {
			fatalStartup("vision fallback provider", err)
		}

		paymentAnalyzer = gemini.NewAnalyzer(gemini.AnalyzerOptions{
			Primary:                   primaryProvider,
			Fallback:                  fallbackProvider,
			RetryAttempts:             config.VisionRetryAttempts(),
			MaxAttempts:               config.VisionMaxAttempts(),
			AcceptConfidenceThreshold: config.VisionAcceptConfidenceThreshold(),
			RejectConfidenceThreshold: config.VisionRejectConfidenceThreshold(),
		})
		mobilePaymentRepo = database.NewMobilePaymentRepository(pool)
		slog.Info("Mobile banking enabled",
			"phone", config.MobileBankingPhone(),
			"vision_primary", visionProviderName(primaryProvider),
			"vision_fallback", visionProviderName(fallbackProvider),
			"vision_retry_attempts", config.VisionRetryAttempts(),
			"vision_max_attempts", config.VisionMaxAttempts(),
			"vision_accept_confidence_threshold", config.VisionAcceptConfidenceThreshold(),
			"vision_reject_confidence_threshold", config.VisionRejectConfidenceThreshold(),
		)
	}

	b, err := bot.New(config.TelegramToken(), bot.WithWorkers(3))
	if err != nil {
		fatalStartup("telegram bot", err)
	}

	// Initialize PaymentService first (WalletService depends on it, not the reverse)
	paymentService := payment.NewPaymentService(tm, purchaseRepository, remnawaveClient, customerRepository, b, nil, referralRepository, messageCache, paymentAnalyzer, mobilePaymentRepo, subKeyRepo, promoCodeRepository, walletTxRepo)

	// Initialize WalletService second (depends on PaymentService)
	walletService := wallet.NewWalletService(paymentService, customerRepository, purchaseRepository, remnawaveClient, b, tm, subKeyRepo, walletTxRepo)

	subService := notification.NewSubscriptionService(subKeyRepo, customerRepository, b, tm)

	subscriptionNotificationCronScheduler, err := subscriptionChecker(ctx, subService)
	if err != nil {
		fatalStartup("subscription notification cron", err)
	}
	subscriptionNotificationCronScheduler.Start()
	defer subscriptionNotificationCronScheduler.Stop()

	autoRenewJob := autorenew.New(subKeyRepo, customerRepository, walletService, tm, b)
	autoRenewCron := cron.New(
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)
	_, err = autoRenewCron.AddFunc("0 * * * *", func() {
		runCronJob(ctx, "auto_renew", 2*time.Minute, func(cronCtx context.Context) {
			autoRenewJob.Run(cronCtx)
		})
	})
	if err != nil {
		fatalStartup("auto-renew cron", err)
	}
	autoRenewCron.Start()
	defer autoRenewCron.Stop()

	syncService := sync.NewSyncService(remnawaveClient, customerRepository)

	appConfigRepo := database.NewAppConfigRepository(pool)
	if err := config.LoadPlansCatalog(ctx, appConfigRepo); err != nil {
		fatalStartup("plans catalog", err)
	}
	bonusStr, err := appConfigRepo.Get(ctx, "referral_bonus_amount")
	if err == nil {
		amount, errParse := strconv.ParseFloat(bonusStr, 64)
		if errParse == nil {
			payment.ReferralBonusAmount = amount
		}
	} else {
		// Just in case it wasn't pre-seeded
		appConfigRepo.Set(ctx, "referral_bonus_amount", "1000")
	}

	// Load per-provider payment receivers from DB.
	// Legacy MOBILE_BANKING_PHONE still seeds KPay/WavePay for backward compatibility,
	// but AyaPay must now be configured explicitly.
	legacyFallbackPhone := ""
	if config.IsMobileBankingEnabled() {
		legacyFallbackPhone = config.MobileBankingPhone()
	}
	for _, entry := range []struct {
		phoneKey      string
		nameKey       string
		phonePtr      *string
		namePtr       *string
		phoneFallback string
		nameFallback  string
	}{
		{
			phoneKey:      "phone_kpay",
			nameKey:       "name_kpay",
			phonePtr:      &payment.PhoneKPay,
			namePtr:       &payment.AccountNameKPay,
			phoneFallback: firstNonEmpty(os.Getenv("MOBILE_BANKING_PHONE_KPAY"), legacyFallbackPhone),
			nameFallback:  os.Getenv("MOBILE_BANKING_NAME_KPAY"),
		},
		{
			phoneKey:      "phone_wavepay",
			nameKey:       "name_wavepay",
			phonePtr:      &payment.PhoneWavePay,
			namePtr:       &payment.AccountNameWave,
			phoneFallback: firstNonEmpty(os.Getenv("MOBILE_BANKING_PHONE_WAVEPAY"), legacyFallbackPhone),
			nameFallback:  os.Getenv("MOBILE_BANKING_NAME_WAVEPAY"),
		},
		{
			phoneKey:      "phone_ayapay",
			nameKey:       "name_ayapay",
			phonePtr:      &payment.PhoneAyaPay,
			namePtr:       &payment.AccountNameAya,
			phoneFallback: os.Getenv("MOBILE_BANKING_PHONE_AYAPAY"),
			nameFallback:  os.Getenv("MOBILE_BANKING_NAME_AYAPAY"),
		},
	} {
		v, loadErr := appConfigRepo.Get(ctx, entry.phoneKey)
		if loadErr == nil {
			// Key exists in DB — use its value (empty = disabled)
			*entry.phonePtr = v
		} else if entry.phoneFallback != "" {
			// Key not in DB yet — seed with fallback
			*entry.phonePtr = entry.phoneFallback
			appConfigRepo.Set(ctx, entry.phoneKey, entry.phoneFallback)
		}

		name, nameErr := appConfigRepo.Get(ctx, entry.nameKey)
		if nameErr == nil {
			*entry.namePtr = name
		} else if entry.nameFallback != "" {
			*entry.namePtr = entry.nameFallback
			appConfigRepo.Set(ctx, entry.nameKey, entry.nameFallback)
		}
	}
	slog.Info("Payment receivers loaded", "providers", payment.GetAcceptedProviderText(", "))

	backupScheduleTime, err := parseDailyScheduleTime(config.BackupScheduleCron())
	if err != nil {
		fatalStartup("backup schedule", err)
	}
	backupService := backup.NewService(appConfigRepo, backup.Options{
		DatabaseURL:         config.DatabaseUrl(),
		BackupDir:           config.BackupDir(),
		Timezone:            config.BackupTimezone(),
		DefaultScheduleTime: backupScheduleTime,
		Enabled:             config.BackupEnabled(),
		SendToTelegram:      config.BackupSendToTelegram(),
		RestoreEnabled:      config.BackupRestoreEnabled(),
		RetentionDays:       config.BackupRetentionDays(),
		MaxLocalFiles:       config.BackupMaxLocalFiles(),
		ConfirmTTL:          time.Duration(config.BackupConfirmTTLMinutes()) * time.Minute,
		JobTimeout:          time.Duration(config.BackupJobTimeoutSeconds()) * time.Second,
		RestoreTimeout:      time.Duration(config.BackupRestoreTimeoutSeconds()) * time.Second,
	})

	mobilePayCache := cache.NewCache(1 * time.Hour)
	botHealthcheck := healthcheck.NewService(healthcheck.ServiceOptions{
		Analyzer:            paymentAnalyzer,
		Customers:           customerRepository,
		Payments:            paymentService,
		SubscriptionKeys:    subKeyRepo,
		RemnawaveUsers:      remnawaveClient,
		SyntheticTelegramID: healthcheck.DefaultSyntheticTelegramID(config.GetAdminTelegramId()),
		CanaryDays:          1,
		CanaryTrafficGB:     1,
	})
	h := handler.NewHandler(syncService, paymentService, tm, customerRepository, purchaseRepository, nil, subService, subKeyRepo, referralRepository, promoCodeRepository, appConfigRepo, botHealthcheck, messageCache, mobilePayCache)
	handler.SetBackupService(backupService)

	me, err := b.GetMe(ctx)
	if err != nil {
		fatalStartup("telegram getMe", err)
	}

	miniAppURL := strings.TrimSpace(config.GetMiniAppURL())
	if miniAppURL != "" {
		menuButtonURL := versionedMiniAppURL(miniAppURL)
		_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
			MenuButton: &models.MenuButtonWebApp{
				Type: models.MenuButtonTypeWebApp,
				Text: "ဒီမှာဝယ်ပါ",
				WebApp: models.WebAppInfo{
					URL: menuButtonURL,
				},
			},
		})
		slog.Info("Configured Mini App menu button", "url", menuButtonURL)
	} else {
		_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
			MenuButton: &models.MenuButtonCommands{
				Type: models.MenuButtonTypeCommands,
			},
		})
	}
	if err != nil {
		slog.Warn("Failed to set chat menu button (non-fatal)", "error", err)
	}

	// Set default bot commands (English fallback for all other locales).
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: handler.PublicBotCommands("en"),
	})
	if err != nil {
		slog.Warn("Failed to set default bot commands (non-fatal)", "error", err)
	}

	// Set public bot commands for Russian.
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands:     handler.PublicBotCommands("ru"),
		LanguageCode: "ru",
	})
	if err != nil {
		slog.Warn("Failed to set Russian bot commands (non-fatal)", "error", err)
	}

	adminID := config.GetAdminTelegramId()
	if adminID != 0 {
		adminScope := &models.BotCommandScopeChat{
			ChatID: adminID,
		}

		_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands: handler.AdminBotCommands("en"),
			Scope:    adminScope,
		})
		if err != nil {
			slog.Warn("Failed to set default admin bot commands (non-fatal)", "error", err)
		}

		_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands:     handler.AdminBotCommands("ru"),
			Scope:        adminScope,
			LanguageCode: "ru",
		})
		if err != nil {
			slog.Warn("Failed to set Russian admin bot commands (non-fatal)", "error", err)
		}
	}

	config.SetBotURL(fmt.Sprintf("https://t.me/%s", me.Username))

	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, h.StartCommandHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/connect", bot.MatchTypeExact, h.ConnectCommandHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/admin", bot.MatchTypeExact, h.AdminCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/admin@", bot.MatchTypePrefix, h.AdminCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/addpromo", bot.MatchTypePrefix, h.AddPromoCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/listpromos", bot.MatchTypeExact, h.ListPromosCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/deletepromo", bot.MatchTypePrefix, h.DeletePromoCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/transactions", bot.MatchTypePrefix, h.TransactionsCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, h.HelpCommandHandler)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/setreferralbonus", bot.MatchTypePrefix, h.SetReferralBonusCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/setphone", bot.MatchTypePrefix, h.SetPhoneCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/setname", bot.MatchTypePrefix, h.SetNameCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/disablephone", bot.MatchTypePrefix, h.DisablePhoneCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/disablename", bot.MatchTypePrefix, h.DisableNameCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/phones", bot.MatchTypeExact, h.PhonesCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/revenue", bot.MatchTypeExact, h.RevenueCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/backup", bot.MatchTypePrefix, h.BackupCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/restore", bot.MatchTypePrefix, h.RestoreCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sync", bot.MatchTypeExact, h.SyncUsersCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/test", bot.MatchTypePrefix, h.TestCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/healthbot", bot.MatchTypePrefix, h.HealthcheckCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/noti", bot.MatchTypePrefix, h.NotiCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/notify", bot.MatchTypePrefix, h.NotiCommandHandler, isAdminMiddleware)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackReferral, bot.MatchTypeExact, h.ReferralCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBuy, bot.MatchTypeExact, h.BuyCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackTrial, bot.MatchTypeExact, h.TrialCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackActivateTrial, bot.MatchTypeExact, h.ActivateTrialCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackStart, bot.MatchTypeExact, h.StartCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackSell, bot.MatchTypePrefix, h.SellCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackConnect, bot.MatchTypeExact, h.ConnectCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPayment, bot.MatchTypePrefix, h.PaymentCallbackHandler, h.AcknowledgeCallbackQueryMiddleware, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackAdmin+":", bot.MatchTypePrefix, h.AdminCallbackHandler, h.AcknowledgeCallbackQueryMiddleware)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.From != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			!h.HasPendingAdminFlow(update.Message.From.ID) &&
			h.IsAdminQuickAction(update.Message.Text)
	}, h.AdminQuickActionHandler, isAdminMiddleware)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil &&
			update.Message.From != nil &&
			update.Message.Text != "" &&
			!strings.HasPrefix(update.Message.Text, "/") &&
			h.HasPendingAdminFlow(update.Message.From.ID)
	}, h.AdminFlowInputHandler, isAdminMiddleware)

	// Register photo handler for mobile banking screenshot uploads
	if config.IsMobileBankingEnabled() {
		b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
			return update.Message != nil && len(update.Message.Photo) > 0
		}, h.MobilePayScreenshotHandler)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/readyz", fullHealthHandler(pool, remnawaveClient, paymentAnalyzer))
	mux.Handle("/healthcheck", fullHealthHandler(pool, remnawaveClient, paymentAnalyzer))
	api.RegisterHandlers(mux, customerRepository, paymentService, b, tm, subKeyRepo, promoCodeRepository, walletService, referralRepository, appConfigRepo)

	// Rate Limiter: 10 req/s, burst 20
	// This prevents abuse while allowing normal usage patterns.
	limiter := api.NewRateLimiter(10, 20)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", config.GetHealthCheckPort()),
		Handler:           limiter.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		slog.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			cancel()
		}
	}()

	// Revenue report cron jobs run on Myanmar calendar boundaries.
	mmtZone := reporting.YangonLocation()
	revenueCron := cron.New(
		cron.WithLocation(mmtZone),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)
	if _, err := revenueCron.AddFunc("0 0 * * *", func() {
		runCronJob(ctx, "daily_revenue_report", 30*time.Second, func(cronCtx context.Context) {
			start, end := reporting.PreviousDayRange(time.Now().In(mmtZone))
			sendRevenueReport(cronCtx, b, purchaseRepository, "daily_revenue_report", "Daily Revenue Report", database.RevenuePeriodDay, start, end)
		})
	}); err != nil {
		fatalStartup("daily revenue cron", err)
	}
	if _, err := revenueCron.AddFunc("5 0 * * 1", func() {
		runCronJob(ctx, "weekly_revenue_report", 30*time.Second, func(cronCtx context.Context) {
			start, end := reporting.PreviousWeekRange(time.Now().In(mmtZone))
			sendRevenueReport(cronCtx, b, purchaseRepository, "weekly_revenue_report", "Weekly Revenue Report", database.RevenuePeriodWeek, start, end)
		})
	}); err != nil {
		fatalStartup("weekly revenue cron", err)
	}
	if _, err := revenueCron.AddFunc("10 0 1 * *", func() {
		runCronJob(ctx, "monthly_revenue_report", 30*time.Second, func(cronCtx context.Context) {
			start, end := reporting.PreviousMonthRange(time.Now().In(mmtZone))
			sendRevenueReport(cronCtx, b, purchaseRepository, "monthly_revenue_report", "Monthly Revenue Report", database.RevenuePeriodMonth, start, end)
		})
	}); err != nil {
		fatalStartup("monthly revenue cron", err)
	}
	revenueCron.Start()
	defer revenueCron.Stop()

	backupLocation, err := time.LoadLocation(config.BackupTimezone())
	if err != nil {
		fatalStartup("backup timezone", err)
	}
	backupCron := cron.New(
		cron.WithLocation(backupLocation),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)
	_, err = backupCron.AddFunc("* * * * *", func() {
		runCronJob(ctx, "backup_scheduler", time.Duration(config.BackupJobTimeoutSeconds())*time.Second, func(cronCtx context.Context) {
			if err := backupService.RunScheduledBackupIfDue(cronCtx, b, config.GetAdminTelegramId()); err != nil {
				slog.Error("Scheduled backup job failed", "error", err)
			}
		})
	})
	if err != nil {
		fatalStartup("backup cron", err)
	}
	backupCron.Start()
	defer backupCron.Stop()

	slog.Info("Bot is starting...")
	b.Start(ctx)

	slog.Info("Shutting down health server")
	shutdownCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Health server shutdown error", "error", err)
	}
}

func shutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

// newCronContext creates a derived context with timeout and a unique request ID
// for structured log correlation across cron job runs.
func newCronContext(parent context.Context, jobName string, timeout time.Duration) (context.Context, context.CancelFunc) {
	type cronJobKey struct{}
	type requestIDKey struct{}
	ctx := context.WithValue(parent, cronJobKey{}, jobName)
	ctx = context.WithValue(ctx, requestIDKey{}, uuid.New().String())
	return context.WithTimeout(ctx, timeout)
}

func runCronJob(parent context.Context, jobName string, timeout time.Duration, fn func(context.Context)) {
	cronCtx, cronCancel := newCronContext(parent, jobName, timeout)
	defer cronCancel()

	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("Cron job panicked", "job", jobName, "panic", recovered)
		}
	}()

	fn(cronCtx)
}

func runHealthProbe() int {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(healthProbeURL())
	if err != nil {
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		return 0
	}
	return 1
}

func healthProbeURL() string {
	port := 8080
	if raw := strings.TrimSpace(os.Getenv("HEALTH_CHECK_PORT")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			port = parsed
		}
	}
	return fmt.Sprintf("http://127.0.0.1:%d/readyz", port)
}

func fullHealthHandler(pool *pgxpool.Pool, rw *remnawave.Client, analyzer gemini.Analyzer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"status":           "ok",
			"db":               "disabled",
			"rw":               "disabled",
			"gemini":           "disabled",
			"vision_analyzer":  "disabled",
			"vision_providers": map[string]string{},
			"time":             time.Now().Format(time.RFC3339),
			"version":          Version,
			"commit":           Commit,
			"buildDate":        BuildDate,
		}

		if pool != nil {
			status["db"] = "ok"
			dbCtx, dbCancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer dbCancel()
			if err := pool.Ping(dbCtx); err != nil {
				status["status"] = "fail"
				status["db"] = "error: " + err.Error()
			}
		}

		if rw != nil {
			status["rw"] = "ok"
			rwCtx, rwCancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer rwCancel()
			if err := rw.Ping(rwCtx); err != nil {
				status["status"] = "fail"
				status["rw"] = "error: " + err.Error()
			}
		}

		if analyzer != nil {
			gCtx, gCancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer gCancel()
			readiness := analyzer.Readiness(gCtx)
			status["vision_analyzer"] = readiness.Status
			status["vision_providers"] = readiness.Providers
			if readiness.Status != "ok" {
				status["status"] = "fail"
			}
			if geminiStatus, ok := readiness.Providers["gemini"]; ok {
				status["gemini"] = geminiStatus
			} else {
				status["gemini"] = "disabled"
			}
			if readiness.Primary != "" {
				status["vision_primary"] = readiness.Primary
			}
			if readiness.Fallback != "" {
				status["vision_fallback"] = readiness.Fallback
			}
		} else {
			status["gemini"] = "disabled"
			status["vision_analyzer"] = "disabled"
		}

		// Set Content-Type before WriteHeader so it is actually sent.
		w.Header().Set("Content-Type", "application/json")
		if status["status"] == "ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		status["remnawave"] = status["rw"]
		delete(status, "rw")
		_ = json.NewEncoder(w).Encode(status)
	})
}

func isAdminMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}
		adminID := config.GetAdminTelegramId()
		userID := update.Message.From.ID

		if userID == adminID {
			next(ctx, b, update)
		} else {
			slog.Warn(
				"Unauthorized admin command attempt",
				"user_id", utils.MaskHalfInt64(userID),
				"command", utils.FirstToken(update.Message.Text),
			)
			// Optional: Reply to user saying unauthorized? Maybe better to stay silent security-wise, but for debugging prompt:
			// b.SendMessage(ctx, &bot.SendMessageParams{
			// 	ChatID: update.Message.Chat.ID,
			// 	Text:   fmt.Sprintf("⛔ Unauthorized. Your ID: %d. Expected Admin ID: %d", userID, adminID),
			// })
		}
	}
}

func subscriptionChecker(parent context.Context, subService *notification.SubscriptionService) (*cron.Cron, error) {
	c := cron.New(
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)

	_, err := c.AddFunc("0 16 * * *", func() {
		runCronJob(parent, "subscription_expiration", 2*time.Minute, func(cronCtx context.Context) {
			err := subService.ProcessSubscriptionExpirationWithContext(cronCtx)
			if err != nil {
				slog.Error("Error sending subscription notifications", "error", err)
			}
		})
	})

	if err != nil {
		return nil, err
	}
	return c, nil
}

func initDatabase(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	config.MaxConns = 20
	config.MinConns = 5

	return pgxpool.ConnectConfig(ctx, config)
}

func parseDailyScheduleTime(expr string) (string, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return "", fmt.Errorf("invalid BACKUP_SCHEDULE_CRON %q, expected 5 fields", expr)
	}

	minute, err := strconv.Atoi(parts[0])
	if err != nil || minute < 0 || minute > 59 {
		return "", fmt.Errorf("invalid backup cron minute in %q", expr)
	}
	hour, err := strconv.Atoi(parts[1])
	if err != nil || hour < 0 || hour > 23 {
		return "", fmt.Errorf("invalid backup cron hour in %q", expr)
	}
	return fmt.Sprintf("%02d:%02d", hour, minute), nil
}
