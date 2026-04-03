package api

import (
	"testing"

	"remnawave-tg-shop-bot/internal/payment"
)

func TestUploadScreenshotFailureResponsePreservesVerificationReason(t *testing.T) {
	result := &payment.VerificationResult{
		Success:   false,
		Reason:    "Screenshot verification is temporarily unavailable right now. Please try again later or contact support.",
		ReasonKey: "mobile_pay_failed_provider_auth",
	}

	resp := uploadScreenshotFailureResponse(result)

	if resp.Status != "failed" {
		t.Fatalf("uploadScreenshotFailureResponse() status = %q, want failed", resp.Status)
	}
	if resp.Message != result.Reason {
		t.Fatalf("uploadScreenshotFailureResponse() message = %q, want %q", resp.Message, result.Reason)
	}
	if resp.Reason != result.ReasonKey {
		t.Fatalf("uploadScreenshotFailureResponse() reason = %q, want %q", resp.Reason, result.ReasonKey)
	}
}
