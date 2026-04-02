package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"remnawave-tg-shop-bot/internal/api"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/gemini"
	"remnawave-tg-shop-bot/internal/handler"
	"remnawave-tg-shop-bot/internal/notification"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/service/autorenew"
	"remnawave-tg-shop-bot/internal/service/backup"
	"remnawave-tg-shop-bot/internal/service/invoicechecker"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--health" {
		os.Exit(runHealthProbe())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer cancel()

	config.InitConfig()
	slog.Info("Application starting", "version", Version, "commit", Commit, "buildDate", BuildDate)

	tm := translation.GetInstance()
	err := tm.InitTranslations("./translations", config.DefaultLanguage())
	if err != nil {
		panic(err)
	}

	pool, err := initDatabase(ctx, config.DatabaseUrl())
	if err != nil {
		panic(err)
	}

	err = database.RunMigrations(ctx, &database.MigrationConfig{Direction: "up", MigrationsPath: "./db/migrations", Steps: 0}, pool)
	if err != nil {
		panic(err)
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

	cryptoPayClient := cryptopay.NewCryptoPayClient(config.CryptoPayUrl(), config.CryptoPayToken())
	remnawaveClient := remnawave.NewClient(config.RemnawaveUrl(), config.RemnawaveToken(), config.RemnawaveMode())

	// Mobile banking / screenshot analyzer
	var paymentAnalyzer gemini.Analyzer
	var mobilePaymentRepo *database.MobilePaymentRepository
	if config.IsMobileBankingEnabled() {
		primaryProvider := gemini.NewClient(config.GeminiAPIKey(), config.GeminiModel())
		var fallbackProvider gemini.Provider
		if config.VisionProviderFallback() == "openrouter" {
			if config.OpenRouterAPIKey() != "" {
				fallbackProvider = gemini.NewOpenRouterClient(config.OpenRouterAPIKey(), config.OpenRouterModel())
			} else {
				slog.Warn("Vision fallback requested but OpenRouter API key is not configured", "fallback_provider", config.VisionProviderFallback())
			}
		}
		paymentAnalyzer = gemini.NewAnalyzer(gemini.AnalyzerOptions{
			Primary:       primaryProvider,
			Fallback:      fallbackProvider,
			RetryAttempts: config.VisionRetryAttempts(),
			MaxAttempts:   config.VisionMaxAttempts(),
		})
		mobilePaymentRepo = database.NewMobilePaymentRepository(pool)
		slog.Info("Mobile banking enabled",
			"phone", config.MobileBankingPhone(),
			"vision_primary", primaryProvider.Name(),
			"vision_fallback", config.VisionProviderFallback(),
			"vision_retry_attempts", config.VisionRetryAttempts(),
			"vision_max_attempts", config.VisionMaxAttempts(),
		)
	}

	b, err := bot.New(config.TelegramToken(), bot.WithWorkers(3))
	if err != nil {
		panic(err)
	}

	// Initialize PaymentService first (WalletService depends on it, not the reverse)
	paymentService := payment.NewPaymentService(tm, purchaseRepository, remnawaveClient, customerRepository, b, cryptoPayClient, referralRepository, messageCache, paymentAnalyzer, mobilePaymentRepo, subKeyRepo, promoCodeRepository, walletTxRepo)

	// Initialize WalletService second (depends on PaymentService)
	walletService := wallet.NewWalletService(paymentService, customerRepository, purchaseRepository, remnawaveClient, b, tm, subKeyRepo, walletTxRepo)

	if config.IsCryptoPayEnabled() {
		invoiceJob := invoicechecker.New(purchaseRepository, cryptoPayClient, paymentService)
		cryptoInvoiceCron := cron.New(
			cron.WithSeconds(),
			cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
		)
		_, err = cryptoInvoiceCron.AddFunc("*/5 * * * * *", func() {
			runCronJob(ctx, "invoice_checker", 4*time.Second, func(cronCtx context.Context) {
				invoiceJob.Run(cronCtx)
			})
		})
		if err != nil {
			panic(err)
		}
		cryptoInvoiceCron.Start()
		defer cryptoInvoiceCron.Stop()
	}

	subService := notification.NewSubscriptionService(subKeyRepo, customerRepository, b, tm)

	subscriptionNotificationCronScheduler := subscriptionChecker(ctx, subService)
	subscriptionNotificationCronScheduler.Start()
	defer subscriptionNotificationCronScheduler.Stop()

	autoRenewJob := autorenew.New(subKeyRepo, customerRepository, walletService, tm, b)
	autoRenewCron := cron.New(
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)
	_, err = autoRenewCron.AddFunc("0 9 * * *", func() {
		runCronJob(ctx, "auto_renew", 2*time.Minute, func(cronCtx context.Context) {
			autoRenewJob.Run(cronCtx)
		})
	})
	if err != nil {
		panic(err)
	}
	autoRenewCron.Start()
	defer autoRenewCron.Stop()

	syncService := sync.NewSyncService(remnawaveClient, customerRepository)

	appConfigRepo := database.NewAppConfigRepository(pool)
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
		panic(err)
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
	h := handler.NewHandler(syncService, paymentService, tm, customerRepository, purchaseRepository, cryptoPayClient, subService, subKeyRepo, referralRepository, promoCodeRepository, appConfigRepo, messageCache, mobilePayCache)
	handler.SetBackupService(backupService)

	me, err := b.GetMe(ctx)
	if err != nil {
		panic(err)
	}

	_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		MenuButton: &models.MenuButtonCommands{
			Type: models.MenuButtonTypeCommands,
		},
	})
	if err != nil {
		slog.Warn("Failed to set chat menu button (non-fatal)", "error", err)
	}

	// Set bot commands for Russian
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Начать работу с ботом"},
			{Command: "connect", Description: "Подключиться"},
		},
		LanguageCode: "ru",
	})
	if err != nil {
		slog.Warn("Failed to set Russian bot commands (non-fatal)", "error", err)
	}

	// Set bot commands for English
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Start using the bot"},
			{Command: "connect", Description: "Connect"},
		},
		LanguageCode: "en",
	})
	if err != nil {
		slog.Warn("Failed to set English bot commands (non-fatal)", "error", err)
	}

	config.SetBotURL(fmt.Sprintf("https://t.me/%s", me.Username))

	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, h.StartCommandHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/connect", bot.MatchTypeExact, h.ConnectCommandHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/addpromo", bot.MatchTypePrefix, h.AddPromoCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/listpromos", bot.MatchTypeExact, h.ListPromosCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/deletepromo", bot.MatchTypePrefix, h.DeletePromoCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/transactions", bot.MatchTypePrefix, h.TransactionsCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, h.HelpCommandHandler, isAdminMiddleware)
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
	mux.Handle("/healthcheck", fullHealthHandler(pool, remnawaveClient, paymentAnalyzer))
	api.RegisterHandlers(mux, customerRepository, paymentService, b, tm, subKeyRepo, promoCodeRepository, walletService, referralRepository)

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

	// Daily revenue report cron job — runs at midnight Myanmar time (UTC+6:30)
	mmtZone := time.FixedZone("MMT", 6*3600+30*60)
	c := cron.New(
		cron.WithLocation(mmtZone),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)
	c.AddFunc("0 0 * * *", func() {
		runCronJob(ctx, "daily_revenue_report", 30*time.Second, func(cronCtx context.Context) {
			rows, err := purchaseRepository.GetRevenueSummary(cronCtx, 2)
			if err != nil {
				slog.Error("Daily revenue report failed", "error", err)
				return
			}

			adminID := config.GetAdminTelegramId()
			yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")
			var totalRevenue float64
			var totalTxns int
			var lines []string

			for _, r := range rows {
				if r.Day != yesterday {
					continue
				}
				method := r.PaymentMethod
				if method == "" {
					method = "unknown"
				}
				currency := r.Currency
				if currency == "" {
					currency = "MMK"
				}
				lines = append(lines, fmt.Sprintf("  %s: %.0f %s (%d txns, %d users)",
					method, r.TotalRevenue, currency, r.TotalPurchases, r.UniqueCustomers))
				totalRevenue += r.TotalRevenue
				totalTxns += r.TotalPurchases
			}

			var text string
			if len(lines) == 0 {
				text = fmt.Sprintf("📊 <b>Daily Revenue Report</b>\n\n%s: No sales yesterday.", yesterday)
			} else {
				text = fmt.Sprintf("📊 <b>Daily Revenue Report</b>\n\n<b>%s</b>\n%s\n\n<b>Total: %.0f MMK (%d txns)</b>",
					yesterday, strings.Join(lines, "\n"), totalRevenue, totalTxns)
			}

			b.SendMessage(cronCtx, &bot.SendMessageParams{
				ChatID:    adminID,
				Text:      text,
				ParseMode: models.ParseModeHTML,
			})
			slog.Info("Daily revenue report sent", "total", totalRevenue, "txns", totalTxns)
		})
	})
	c.Start()
	defer c.Stop()

	backupLocation, err := time.LoadLocation(config.BackupTimezone())
	if err != nil {
		panic(err)
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
		panic(err)
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
	return fmt.Sprintf("http://127.0.0.1:%d/healthcheck", port)
}

func fullHealthHandler(pool *pgxpool.Pool, rw *remnawave.Client, analyzer gemini.Analyzer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"status":           "ok",
			"db":               "ok",
			"rw":               "ok",
			"gemini":           "ok",
			"vision_analyzer":  "ok",
			"vision_providers": map[string]string{},
			"time":             time.Now().Format(time.RFC3339),
			"version":          Version,
			"commit":           Commit,
			"buildDate":        BuildDate,
		}

		dbCtx, dbCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer dbCancel()
		if err := pool.Ping(dbCtx); err != nil {
			status["status"] = "fail"
			status["db"] = "error: " + err.Error()
		}

		rwCtx, rwCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer rwCancel()
		if err := rw.Ping(rwCtx); err != nil {
			status["status"] = "fail"
			status["rw"] = "error: " + err.Error()
		}

		// Analyzer health check is non-blocking and does not affect overall status.
		if analyzer != nil {
			gCtx, gCancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer gCancel()
			readiness := analyzer.Readiness(gCtx)
			status["vision_analyzer"] = readiness.Status
			status["vision_providers"] = readiness.Providers
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
			slog.Warn("Unauthorized admin command attempt", "user_id", userID, "admin_id", adminID, "command", update.Message.Text)
			// Optional: Reply to user saying unauthorized? Maybe better to stay silent security-wise, but for debugging prompt:
			// b.SendMessage(ctx, &bot.SendMessageParams{
			// 	ChatID: update.Message.Chat.ID,
			// 	Text:   fmt.Sprintf("⛔ Unauthorized. Your ID: %d. Expected Admin ID: %d", userID, adminID),
			// })
		}
	}
}

func subscriptionChecker(parent context.Context, subService *notification.SubscriptionService) *cron.Cron {
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
		panic(err)
	}
	return c
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
