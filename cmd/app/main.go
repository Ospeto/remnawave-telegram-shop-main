package main

import (
	"context"
	"fmt"
	"log"
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
	"remnawave-tg-shop-bot/internal/service/wallet"
	"remnawave-tg-shop-bot/internal/sync"
	"remnawave-tg-shop-bot/internal/translation"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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

	// Mobile banking / Gemini
	var geminiClient *gemini.Client
	var mobilePaymentRepo *database.MobilePaymentRepository
	if config.IsMobileBankingEnabled() {
		geminiClient = gemini.NewClient(config.GeminiAPIKey(), config.GeminiModel())
		mobilePaymentRepo = database.NewMobilePaymentRepository(pool)
		slog.Info("Mobile banking enabled", "phone", config.MobileBankingPhone())
	}

	b, err := bot.New(config.TelegramToken(), bot.WithWorkers(3))
	if err != nil {
		panic(err)
	}

	// Actually, walletService calls paymentService.CreatePurchase.
	// PaymentService does NOT need walletService strictly for basic ops, but maybe for some checks?
	// Let's check PaymentService struct. It has `walletService` field added in previous thought?
	// Wait, I haven't added walletService field to PaymentService struct yet in code?
	// Let's check if I added it. I don't recall adding it to PaymentService struct in `payment.go`.
	// I added `CreateWalletPayment` to `payment.go`.
	// `WalletService` calls `PaymentService`. CONSTANT -> `NewWalletService` takes `PaymentService`.
	// So `PaymentService` must be created FIRST.

	// Create PaymentService first (without walletService dependency if possible, or refactor).
	// Checking `payment.go` again... it does NOT import `wallet`.
	// So PaymentService does NOT depend on WalletService.
	// WalletService depends on PaymentService.

	// BUT! `main.go` line 90 in original file tried to pass `walletService` to `NewPaymentService`.
	// I must have hallucinated that requirement or added it in a way I didn't verify.
	// Let's check `payment.NewPaymentService` signature.
	// From previous `view_file` of `payment.go` (step 572), `NewPaymentService` does NOT take `WalletService`.
	// Params: translation, purchaseRepo, remnawaveClient, customerRepo, telegramBot, cryptoPayClient, referralRepo, cache, geminiClient, mobilePaymentRepo, subKeyRepo, promoCodeRepo.
	// So I should NOT pass `walletService` to `NewPaymentService`.

	// Initialize PaymentService FIRST (no circular dependency now)
	paymentService := payment.NewPaymentService(tm, purchaseRepository, remnawaveClient, customerRepository, b, cryptoPayClient, referralRepository, messageCache, geminiClient, mobilePaymentRepo, subKeyRepo, promoCodeRepository, walletTxRepo)

	// Initialize WalletService SECOND (depends on PaymentService)
	// Also needs walletTxRepo
	walletService := wallet.NewWalletService(paymentService, customerRepository, purchaseRepository, remnawaveClient, b, tm, subKeyRepo, walletTxRepo)

	cronScheduler := setupInvoiceChecker(purchaseRepository, cryptoPayClient, paymentService)
	if cronScheduler != nil {
		cronScheduler.Start()
		defer cronScheduler.Stop()
	}

	subService := notification.NewSubscriptionService(customerRepository, b, tm)

	subscriptionNotificationCronScheduler := subscriptionChecker(subService)
	subscriptionNotificationCronScheduler.Start()
	defer subscriptionNotificationCronScheduler.Stop()

	autoRenewCronScheduler := autoRenewChecker(customerRepository, walletService, paymentService, tm, b)
	autoRenewCronScheduler.Start()
	defer autoRenewCronScheduler.Stop()

	syncService := sync.NewSyncService(remnawaveClient, customerRepository)

	mobilePayCache := cache.NewCache(1 * time.Hour)
	h := handler.NewHandler(syncService, paymentService, tm, customerRepository, purchaseRepository, cryptoPayClient, referralRepository, promoCodeRepository, messageCache, mobilePayCache)

	me, err := b.GetMe(ctx)
	if err != nil {
		panic(err)
	}

	_, err = b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		MenuButton: &models.MenuButtonCommands{
			Type: models.MenuButtonTypeCommands,
		},
	})

	// Set bot commands for Russian
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Начать работу с ботом"},
			{Command: "connect", Description: "Подключиться"},
		},
		LanguageCode: "ru",
	})

	// Set bot commands for English
	_, err = b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "start", Description: "Start using the bot"},
			{Command: "connect", Description: "Connect"},
		},
		LanguageCode: "en",
	})

	config.SetBotURL(fmt.Sprintf("https://t.me/%s", me.Username))

	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, h.StartCommandHandler, h.SuspiciousUserFilterMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/connect", bot.MatchTypeExact, h.ConnectCommandHandler, h.SuspiciousUserFilterMiddleware, h.CreateCustomerIfNotExistMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/addpromo", bot.MatchTypePrefix, h.AddPromoCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/listpromos", bot.MatchTypeExact, h.ListPromosCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/deletepromo", bot.MatchTypePrefix, h.DeletePromoCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/transactions", bot.MatchTypePrefix, h.TransactionsCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, h.HelpCommandHandler, isAdminMiddleware)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/sync", bot.MatchTypeExact, h.SyncUsersCommandHandler, isAdminMiddleware)

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
	api.RegisterHandlers(mux, customerRepository, paymentService, b, tm, subKeyRepo, promoCodeRepository, walletService)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.GetHealthCheckPort()),
		Handler: mux,
	}
	go func() {
		log.Printf("Server listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	slog.Info("Bot is starting...")
	b.Start(ctx)

	log.Println("Shutting down health server…")
	shutdownCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Health server shutdown error: %v", err)
	}
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
			w.WriteHeader(http.StatusServiceUnavailable)
			status["status"] = "fail"
			status["db"] = "error: " + err.Error()
		}

		rwCtx, rwCancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer rwCancel()
		if err := rw.Ping(rwCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			status["status"] = "fail"
			status["rw"] = "error: " + err.Error()
		}

		if status["status"] == "ok" {
			w.WriteHeader(http.StatusOK)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"%s","db":"%s","remnawave":"%s","time":"%s","version":"%s","commit":"%s","buildDate":"%s"}`,
			status["status"], status["db"], status["rw"], status["time"], Version, Commit, BuildDate)
	})
}

func isAdminMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message != nil && update.Message.From.ID == config.GetAdminTelegramId() {
			next(ctx, b, update)
		} else {
			return
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

func setupInvoiceChecker(
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	paymentService *payment.PaymentService) *cron.Cron {
	if !config.IsCryptoPayEnabled() {
		return nil
	}
	c := cron.New(cron.WithSeconds())

	_, err := c.AddFunc("*/5 * * * * *", func() {
		ctx := context.Background()
		checkCryptoPayInvoice(ctx, purchaseRepository, cryptoPayClient, paymentService)
	})

	if err != nil {
		panic(err)
	}

	return c
}

func checkCryptoPayInvoice(
	ctx context.Context,
	purchaseRepository *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	paymentService *payment.PaymentService,
) {
	pendingPurchases, err := purchaseRepository.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeCrypto,
		database.PurchaseStatusPending,
	)
	if err != nil {
		log.Printf("Error finding pending purchases: %v", err)
		return
	}
	if len(*pendingPurchases) == 0 {
		return
	}

	var invoiceIDs []string

	for _, purchase := range *pendingPurchases {
		if purchase.CryptoInvoiceID != nil {
			invoiceIDs = append(invoiceIDs, fmt.Sprintf("%d", *purchase.CryptoInvoiceID))
		}
	}

	if len(invoiceIDs) == 0 {
		return
	}

	stringInvoiceIDs := strings.Join(invoiceIDs, ",")
	invoices, err := cryptoPayClient.GetInvoices("", "", "", stringInvoiceIDs, 0, 0)
	if err != nil {
		log.Printf("Error getting invoices: %v", err)
		return
	}

	for _, invoice := range *invoices {
		if invoice.InvoiceID != nil && invoice.IsPaid() {
			payload := strings.Split(invoice.Payload, "&")
			if len(payload) < 2 {
				slog.Warn("Malformed invoice payload", "payload", invoice.Payload, "invoiceId", invoice.InvoiceID)
				continue
			}
			purchaseIDStr := strings.Split(payload[0], "=")
			if len(purchaseIDStr) < 2 {
				slog.Warn("Malformed purchase ID in payload", "payload", invoice.Payload, "invoiceId", invoice.InvoiceID)
				continue
			}
			usernameStr := strings.Split(payload[1], "=")
			if len(usernameStr) < 2 {
				slog.Warn("Malformed username in payload", "payload", invoice.Payload, "invoiceId", invoice.InvoiceID)
				continue
			}
			purchaseID, err := strconv.Atoi(purchaseIDStr[1])
			if err != nil {
				slog.Warn("Invalid purchase ID in payload", "payload", invoice.Payload, "invoiceId", invoice.InvoiceID, "error", err)
				continue
			}
			username := usernameStr[1]
			ctxWithUsername := context.WithValue(ctx, "username", username)
			err = paymentService.ProcessPurchaseById(ctxWithUsername, int64(purchaseID))
			if err != nil {
				slog.Error("Error processing invoice", "invoiceId", invoice.InvoiceID, "error", err)
			} else {
				slog.Info("Invoice processed", "invoiceId", invoice.InvoiceID, "purchaseId", purchaseID)
			}

		}
	}

}

func autoRenewChecker(
	customerRepository *database.CustomerRepository,
	walletService *wallet.WalletService,
	paymentService *payment.PaymentService,
	tm *translation.Manager,
	b *bot.Bot,
) *cron.Cron {
	c := cron.New()

	// Run daily at 9:00 AM to check for subscriptions expiring in 3 days
	_, err := c.AddFunc("0 9 * * *", func() {
		ctx := context.Background()
		processAutoRenewals(ctx, customerRepository, walletService, paymentService, tm, b)
	})

	if err != nil {
		panic(err)
	}

	return c
}

func processAutoRenewals(
	ctx context.Context,
	customerRepository *database.CustomerRepository,
	walletService *wallet.WalletService,
	paymentService *payment.PaymentService,
	tm *translation.Manager,
	b *bot.Bot,
) {
	// Find customers with auto_renew=true and expire_at within 3 days
	threeDaysFromNow := time.Now().Add(3 * 24 * time.Hour)
	customers, err := customerRepository.FindByAutoRenewExpiring(ctx, threeDaysFromNow)
	if err != nil {
		slog.Error("Error finding customers for auto-renewal", "error", err)
		return
	}

	for _, customer := range customers {
		// Find matching plan for auto_renew_duration
		plan := findPlanByDuration(customer.AutoRenewDuration)
		if plan == nil {
			slog.Warn("No matching plan for auto-renew duration", "customer_id", customer.ID, "duration", customer.AutoRenewDuration)
			continue
		}

		// Check if customer has sufficient balance
		hasBalance, err := walletService.HasSufficientBalance(ctx, customer.ID, float64(plan.Price))
		if err != nil {
			slog.Error("Error checking balance for auto-renewal", "customer_id", customer.ID, "error", err)
			continue
		}

		if !hasBalance {
			// Send notification about insufficient balance
			_, err = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: customer.TelegramID,
				Text:   tm.GetText(customer.Language, "auto_renew_insufficient_balance"),
			})
			if err != nil {
				slog.Error("Error sending auto-renew notification", "customer_id", customer.ID, "error", err)
			}
			continue
		}

		// Attempt auto-renewal
		_, purchaseID, err := paymentService.CreatePurchase(ctx, float64(plan.Price), plan.Days, plan.TrafficLimitGB, &customer, database.InvoiceTypeWalletPayment, "")
		if err != nil {
			slog.Error("Auto-renewal failed", "customer_id", customer.ID, "error", err)
			// Send failure notification
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: customer.TelegramID,
				Text:   tm.GetText(customer.Language, "auto_renew_failed"),
			})
			continue
		}

		slog.Info("Auto-renewal successful", "customer_id", customer.ID, "purchase_id", purchaseID)

		// Send success notification
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: customer.TelegramID,
			Text:   tm.GetText(customer.Language, "auto_renew_success"),
		})
		if err != nil {
			slog.Error("Error sending auto-renew success notification", "customer_id", customer.ID, "error", err)
		}
	}
}

func findPlanByDuration(days int) *config.Plan {
	plans := config.Plans()
	for _, plan := range plans {
		if plan.Days == days {
			return &plan
		}
	}
	return nil
}
