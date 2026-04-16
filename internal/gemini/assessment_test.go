package gemini

import "testing"

func TestAssessPaymentInfoRequestsClearerScreenshotForLowConfidenceValidResult(t *testing.T) {
	assessment := AssessPaymentInfo(&PaymentInfo{
		Provider:          "kpay",
		TransactionID:     "TX-1",
		PhoneNumber:       "09123456789",
		Amount:            12000,
		IsValid:           true,
		Confidence:        0.52,
		NeedsClearerImage: true,
	}, 0.80, 0.70)

	if assessment.Action != OutcomeAskClearer {
		t.Fatalf("AssessPaymentInfo().Action = %q, want %q", assessment.Action, OutcomeAskClearer)
	}
	if assessment.Reason != "clearer_image_required" {
		t.Fatalf("AssessPaymentInfo().Reason = %q, want %q", assessment.Reason, "clearer_image_required")
	}
}

func TestAssessPaymentInfoRejectsHighConfidenceInvalidReceipt(t *testing.T) {
	assessment := AssessPaymentInfo(&PaymentInfo{
		IsValid:       false,
		Confidence:    0.91,
		InvalidReason: "not_payment_confirmation",
	}, 0.80, 0.70)

	if assessment.Action != OutcomeReject {
		t.Fatalf("AssessPaymentInfo().Action = %q, want %q", assessment.Action, OutcomeReject)
	}
}

func TestAssessPaymentInfoUsesPassBiasedThresholds(t *testing.T) {
	validAssessment := AssessPaymentInfo(&PaymentInfo{
		IsValid:       true,
		TransactionID: "TX-1",
		Confidence:    0.56,
	}, 0.55, 0.90)
	if validAssessment.Action != OutcomeAccept {
		t.Fatalf("valid AssessPaymentInfo().Action = %q, want %q", validAssessment.Action, OutcomeAccept)
	}

	invalidAssessment := AssessPaymentInfo(&PaymentInfo{
		IsValid:       false,
		Confidence:    0.80,
		InvalidReason: "not_payment_confirmation",
	}, 0.55, 0.90)
	if invalidAssessment.Action != OutcomeAskClearer {
		t.Fatalf("invalid AssessPaymentInfo().Action = %q, want %q", invalidAssessment.Action, OutcomeAskClearer)
	}
}
