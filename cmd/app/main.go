package main

import (
	"context"
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
	"remnawave-tg-shop-bot/internal/openrouter"
	"remnawave-tg-shop-bot/internal/payment"
	"remnawave-tg-shop-bot/internal/receiptai"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/service/autorenew"
	"remnawave-tg-shop-bot/internal/service/invoicechecker"
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
	"strconv"
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

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
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

	// Mobile banking / receipt AI
	var receiptAnalyzer receiptai.Analyzer
	var mobilePaymentRepo *database.MobilePaymentRepository
	if config.IsMobileBankingEnabled() {
		primaryAnalyzer := receiptai.Analyzer(gemini.NewClient(config.GeminiAPIKey(), config.GeminiModel()))
		var fallbackAnalyzer receiptai.Analyzer
		if config.OpenRouterAPIKey() != "" {
			fallbackAnalyzer = openrouter.NewClient(config.OpenRouterAPIKey(), config.OpenRouterModel())
		}
		receiptAnalyzer = receiptai.NewFailoverAnalyzer(primaryAnalyzer, fallbackAnalyzer)
		mobilePaymentRepo = database.NewMobilePaymentRepository(pool)
		slog.Info("Mobile banking enabled", "phone", config.MobileBankingPhone(), "primary_ai", primaryAnalyzer.ProviderName(), "fallback_ai_enabled", fallbackAnalyzer != nil)
	}

	b, err := bot.New(
		config.TelegramToken(),
		bot.WithWorkers(3),
		bot.WithSkipGetMe(),
	)
	if err != nil {
		panic(err)
	}

	// Initialize PaymentService first (WalletService depends on it, not the reverse)
	paymentService := payment.NewPaymentService(tm, purchaseRepository, remnawaveClient, customerRepository, b, cryptoPayClient, referralRepository, messageCache, receiptAnalyzer, mobilePaymentRepo, subKeyRepo, promoCodeRepository, walletTxRepo)

	// Initialize WalletService second (depends on PaymentService)
	walletService := wallet.NewWalletService(paymentService, customerRepository, purchaseRepository, remnawaveClient, b, tm, subKeyRepo, walletTxRepo)

	if config.IsCryptoPayEnabled() {
		invoiceJob := invoicechecker.New(purchaseRepository, cryptoPayClient, paymentService)
		cryptoInvoiceCron := cron.New(cron.WithSeconds())
		_, err = cryptoInvoiceCron.AddFunc("*/5 * * * * *", func() {
			cronCtx := newCronContext("invoice_checker")
			invoiceJob.Run(cronCtx)
		})
		if err != nil {
			panic(err)
		}
		cryptoInvoiceCron.Start()
		defer cryptoInvoiceCron.Stop()
	}

	subService := notification.NewSubscriptionService(customerRepository, b, tm)

	subscriptionNotificationCronScheduler := subscriptionChecker(subService)
	subscriptionNotificationCronScheduler.Start()
	defer subscriptionNotificationCronScheduler.Stop()

	autoRenewJob := autorenew.New(subKeyRepo, customerRepository, walletService, tm, b)
	autoRenewCron := cron.New()
	_, err = autoRenewCron.AddFunc("0 9 * * *", func() {
		cronCtx := newCronContext("auto_renew")
		autoRenewJob.Run(cronCtx)
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

	mobilePayCache := cache.NewCache(1 * time.Hour)
	h := handler.NewHandler(syncService, paymentService, tm, customerRepository, purchaseRepository, cryptoPayClient, subService, referralRepository, promoCodeRepository, appConfigRepo, messageCache, mobilePayCache)

	me, err := getBotIdentity(ctx, b)
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
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sync", bot.MatchTypeExact, h.SyncUsersCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/apicheck", bot.MatchTypeExact, h.APICheckCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/test", bot.MatchTypePrefix, h.TestCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/noti", bot.MatchTypePrefix, h.NotiCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/notify", bot.MatchTypePrefix, h.NotiCommandHandler, isAdminMiddleware)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackReferral, bot.MatchTypeExact, h.ReferralCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackBuy, bot.MatchTypeExact, h.BuyCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackTrial, bot.MatchTypeExact, h.TrialCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackActivateTrial, bot.MatchTypeExact, h.ActivateTrialCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackStart, bot.MatchTypeExact, h.StartCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackSell, bot.MatchTypePrefix, h.SellCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackConnect, bot.MatchTypeExact, h.ConnectCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, handler.CallbackPayment, bot.MatchTypePrefix, h.PaymentCallbackHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)

	// Register photo handler for mobile banking screenshot uploads
	if config.IsMobileBankingEnabled() {
		b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
			return update.Message != nil && len(update.Message.Photo) > 0
		}, h.MobilePayScreenshotHandler)
	}

	mux := http.NewServeMux()
	mux.Handle("/healthcheck", fullHealthHandler(pool, remnawaveClient))
	api.RegisterHandlers(mux, customerRepository, paymentService, b, tm, subKeyRepo, promoCodeRepository, walletService, referralRepository)

	// Rate Limiter: 10 req/s, burst 20
	// This prevents abuse while allowing normal usage patterns.
	limiter := api.NewRateLimiter(10, 20)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.GetHealthCheckPort()),
		Handler: limiter.Middleware(mux),
	}
	go func() {
		slog.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("Bot is starting...")
	b.Start(ctx)

	slog.Info("Shutting down health server")
	shutdownCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Health server shutdown error", "error", err)
	}
}

// newCronContext creates a background context with a unique request ID for
// structured log correlation across cron job runs.
func newCronContext(jobName string) context.Context {
	type cronJobKey struct{}
	type requestIDKey struct{}
	ctx := context.WithValue(context.Background(), cronJobKey{}, jobName)
	return context.WithValue(ctx, requestIDKey{}, uuid.New().String())
}

func getBotIdentity(ctx context.Context, b *bot.Bot) (*models.User, error) {
	var lastErr error

	for attempt := 1; attempt <= 5; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		me, err := b.GetMe(requestCtx)
		cancel()
		if err == nil {
			return me, nil
		}

		lastErr = err
		slog.Warn("Telegram getMe failed during startup", "attempt", attempt, "error", err)
		if attempt == 5 {
			break
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 3 * time.Second):
		}
	}

	return nil, fmt.Errorf("telegram getMe failed after retries: %w", lastErr)
}

func fullHealthHandler(pool *pgxpool.Pool, rw *remnawave.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{
			"status":    "ok",
			"db":        "ok",
			"rw":        "ok",
			"time":      time.Now().Format(time.RFC3339),
			"version":   Version,
			"commit":    Commit,
			"buildDate": BuildDate,
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

		// Set Content-Type before WriteHeader so it is actually sent.
		w.Header().Set("Content-Type", "application/json")
		if status["status"] == "ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		fmt.Fprintf(w, `{"status":"%s","db":"%s","remnawave":"%s","time":"%s","version":"%s","commit":"%s","buildDate":"%s"}`,
			status["status"], status["db"], status["rw"], status["time"], Version, Commit, BuildDate)
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

func subscriptionChecker(subService *notification.SubscriptionService) *cron.Cron {
	c := cron.New()

	_, err := c.AddFunc("0 16 * * *", func() {
		err := subService.ProcessSubscriptionExpiration()
		if err != nil {
			slog.Error("Error sending subscription notifications", "error", err)
		}
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
