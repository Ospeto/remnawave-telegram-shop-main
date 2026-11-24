package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// apiError is the machine-readable shape of a user-facing API failure.
//
// U5: the Mini App used to print the raw English text of http.Error straight into
// a Burmese screen ("Unauthorized", "Database error"), so a customer holding the
// phone in Myanmar language got an untranslated dead end. Every customer-facing
// failure now carries a stable `code` that the client maps to localized copy, and
// the `message` stays as a developer-facing fallback.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes shared with web-app/src/lib/apiErrors.ts. Changing a value here is
// a client-visible contract change.
const (
	errorCodeUnauthorized         = "unauthorized"
	errorCodeUserNotFound         = "user_not_found"
	errorCodeDatabase             = "database_error"
	errorCodeMethodNotAllowed     = "method_not_allowed"
	errorCodeNotFound             = "not_found"
	errorCodePurchaseNotFound     = "purchase_not_found"
	errorCodeMissingPurchaseID    = "missing_purchase_id"
	errorCodeInvalidPurchaseID    = "invalid_purchase_id"
	errorCodePurchaseForbidden    = "purchase_not_allowed"
	errorCodeNotCancellable       = "purchase_not_cancellable"
	errorCodeForbidden            = "forbidden"
	errorCodeSessionExpired       = "session_expired"
	errorCodePendingConflict      = "pending_screenshot_payment"
	errorCodeUploadTooLarge       = "upload_too_large"
	errorCodeUploadInvalidFile    = "upload_invalid_file"
	errorCodeUploadReadFailed     = "upload_read_failed"
	errorCodeUploadUnsupported    = "upload_unsupported_type"
	errorCodeVerificationRateLim  = "verification_rate_limited"
	errorCodeVerificationInProg   = "verification_in_progress"
	errorCodeVerificationUnavail  = "verification_unavailable"
	errorCodeTrialAlreadyUsed     = "trial_already_used"
	errorCodeTrialUnavailable     = "trial_unavailable"
	errorCodeTrialPanelUser       = "trial_existing_panel_user"
	errorCodeTrialFailed          = "trial_failed"
	errorCodePromoInvalid         = "promo_invalid"
	errorCodePromoUnavailable     = "promo_unavailable"
	errorCodeInvalidKeyID         = "invalid_key_id"
	errorCodeMissingKeyID         = "missing_key_id"
	errorCodeKeyNotFound          = "key_not_found"
	errorCodeKeyNotHideable       = "key_not_hideable"
	errorCodeRenewalInProgress    = "renewal_in_progress"
	errorCodeAutoRenewUnavailable = "auto_renew_unavailable"
	errorCodeInvalidAmount        = "invalid_amount"
	errorCodeInvalidPayment       = "invalid_payment_method"
	errorCodeInvalidBody          = "invalid_request_body"
	errorCodeServiceUnavailable   = "service_unavailable"
	errorCodeInternal             = "internal_error"
)

// writeAPIError sends a user-facing failure as {code, message} JSON.
//
// Server-side failures (5xx) are logged here so a handler cannot fail silently:
// any 5xx reaching the customer also reaches the terminal.
func writeAPIError(w http.ResponseWriter, status int, code string, message string) {
	writeAPIErrorWithCause(w, status, code, message, nil)
}

// writeAPIErrorWithCause is writeAPIError for a failure carrying a server-side
// cause worth logging.
func writeAPIErrorWithCause(w http.ResponseWriter, status int, code string, message string, cause error) {
	if status >= http.StatusInternalServerError {
		if cause != nil {
			slog.Error("API request failed", "status", status, "code", code, "error", cause)
		} else {
			slog.Error("API request failed", "status", status, "code", code, "message", message)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiError{Code: code, Message: message})
}

// errorCodeForStatus maps an HTTP status to a generic code for validation
// helpers that only report a status and a message.
func errorCodeForStatus(status int, fallback string) string {
	switch status {
	case http.StatusBadRequest:
		return errorCodeInvalidBody
	case http.StatusForbidden:
		return errorCodeForbidden
	case http.StatusNotFound:
		return errorCodePurchaseNotFound
	case http.StatusConflict:
		if fallback != "" {
			return fallback
		}
		return errorCodeNotCancellable
	case http.StatusUnauthorized:
		return errorCodeUnauthorized
	case http.StatusTooManyRequests:
		return errorCodeVerificationRateLim
	case http.StatusServiceUnavailable:
		return errorCodeServiceUnavailable
	}
	if fallback != "" {
		return fallback
	}
	return errorCodeInternal
}

// errorCodeForUploadReject maps a screenshot-upload reject reason to the
// customer-facing code, keeping the reason itself in the server log.
func errorCodeForUploadReject(reason string, status int) string {
	switch reason {
	case uploadRejectPurchaseNotFound, uploadRejectPurchaseOwnerMismatch:
		return errorCodePurchaseNotFound
	case uploadRejectPurchaseNotAwaiting, uploadRejectUnsupportedInvoiceType:
		return errorCodeNotCancellable
	case uploadRejectVerificationRateLimited:
		return errorCodeVerificationRateLim
	case uploadRejectVerificationInProgress:
		return errorCodeVerificationInProg
	case uploadRejectVerificationServiceFailed:
		return errorCodeVerificationUnavail
	case uploadRejectUnsupportedFileType:
		return errorCodeUploadUnsupported
	case uploadRejectFileReadFailed:
		return errorCodeUploadReadFailed
	case uploadRejectMissingFileField, uploadRejectInvalidMultipartForm:
		return errorCodeUploadInvalidFile
	case uploadRejectMissingPurchaseID:
		return errorCodeMissingPurchaseID
	case uploadRejectInvalidPurchaseID:
		return errorCodeInvalidPurchaseID
	case uploadRejectMethodNotAllowed:
		return errorCodeMethodNotAllowed
	case uploadRejectMissingTelegramAuth:
		return errorCodeUnauthorized
	}
	return errorCodeForStatus(status, errorCodeInternal)
}
