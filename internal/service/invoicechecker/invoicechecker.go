// Package invoicechecker contains the scheduled job that polls CryptoPay for
// paid invoices and processes the corresponding purchases.
package invoicechecker

import (
	"context"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/payment"
	"strconv"
	"strings"
)

// Job polls CryptoPay for paid invoices and triggers purchase processing.
type Job struct {
	purchaseRepo    *database.PurchaseRepository
	cryptoPayClient *cryptopay.Client
	paymentService  *payment.PaymentService
}

// New creates a new CryptoInvoiceJob.
func New(
	purchaseRepo *database.PurchaseRepository,
	cryptoPayClient *cryptopay.Client,
	paymentService *payment.PaymentService,
) *Job {
	return &Job{
		purchaseRepo:    purchaseRepo,
		cryptoPayClient: cryptoPayClient,
		paymentService:  paymentService,
	}
}

// Run checks all pending crypto invoices and processes any that have been paid.
// It is intended to be called by a cron scheduler (every 5 seconds).
func (j *Job) Run(ctx context.Context) {
	pendingPurchases, err := j.purchaseRepo.FindByInvoiceTypeAndStatus(
		ctx,
		database.InvoiceTypeCrypto,
		database.PurchaseStatusPending,
	)
	if err != nil {
		slog.Error("Invoice checker: error finding pending purchases", "error", err)
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

	invoices, err := j.cryptoPayClient.GetInvoices(ctx, "", "", "", strings.Join(invoiceIDs, ","), 0, 0)
	if err != nil {
		slog.Error("Invoice checker: error fetching invoices from CryptoPay", "error", err)
		return
	}

	for _, invoice := range *invoices {
		if invoice.InvoiceID == nil || !invoice.IsPaid() {
			continue
		}

		purchaseID, username, ok := j.parseInvoicePayload(invoice.Payload, invoice.InvoiceID)
		if !ok {
			continue
		}

		ctxWithUsername := context.WithValue(ctx, payment.UsernameCtxKey, username)
		if err := j.paymentService.ProcessPurchaseById(ctxWithUsername, purchaseID); err != nil {
			slog.Error("Invoice checker: error processing invoice", "invoice_id", *invoice.InvoiceID, "error", err)
		} else {
			slog.Info("Invoice checker: invoice processed", "invoice_id", *invoice.InvoiceID, "purchase_id", purchaseID)
		}
	}
}

// parseInvoicePayload parses "purchaseId=123&username=foo" into its components.
func (j *Job) parseInvoicePayload(payload string, invoiceID interface{}) (purchaseID int64, username string, ok bool) {
	parts := strings.Split(payload, "&")
	if len(parts) < 2 {
		slog.Warn("Invoice checker: malformed payload", "payload", payload, "invoice_id", invoiceID)
		return 0, "", false
	}

	purchaseIDParts := strings.Split(parts[0], "=")
	usernameParts := strings.Split(parts[1], "=")
	if len(purchaseIDParts) < 2 || len(usernameParts) < 2 {
		slog.Warn("Invoice checker: malformed payload fields", "payload", payload, "invoice_id", invoiceID)
		return 0, "", false
	}

	id, err := strconv.ParseInt(purchaseIDParts[1], 10, 64)
	if err != nil {
		slog.Warn("Invoice checker: invalid purchase ID in payload", "payload", payload, "invoice_id", invoiceID, "error", err)
		return 0, "", false
	}

	return id, usernameParts[1], true
}
