package gemini

import (
	"math"
	"strings"
)

type OutcomeAction string

const (
	OutcomeAccept     OutcomeAction = "accept"
	OutcomeReject     OutcomeAction = "reject"
	OutcomeAskClearer OutcomeAction = "ask_clearer"
)

type AnalysisAssessment struct {
	Action      OutcomeAction
	Reason      string
	Confidence  float64
	IsAmbiguous bool
}

const (
	defaultAcceptConfidenceThreshold = 0.55
	defaultRejectConfidenceThreshold = 0.90
)

func normalizeConfidenceThresholds(acceptThreshold, rejectThreshold float64) (float64, float64) {
	if acceptThreshold <= 0 || acceptThreshold > 1 {
		acceptThreshold = defaultAcceptConfidenceThreshold
	}
	if rejectThreshold <= 0 || rejectThreshold > 1 {
		rejectThreshold = defaultRejectConfidenceThreshold
	}
	return acceptThreshold, rejectThreshold
}

func AssessPaymentInfo(info *PaymentInfo, acceptThreshold, rejectThreshold float64) AnalysisAssessment {
	acceptThreshold, rejectThreshold = normalizeConfidenceThresholds(acceptThreshold, rejectThreshold)

	if info == nil {
		return AnalysisAssessment{
			Action:      OutcomeAskClearer,
			Reason:      "empty_result",
			Confidence:  0,
			IsAmbiguous: true,
		}
	}

	confidence := effectiveConfidence(info)
	invalidReason := normalizeInvalidReason(info.InvalidReason)
	rawInvalidReason := invalidReason

	if info.NeedsClearerImage {
		return AnalysisAssessment{
			Action:      OutcomeAskClearer,
			Reason:      "clearer_image_required",
			Confidence:  confidence,
			IsAmbiguous: true,
		}
	}

	if info.IsValid {
		switch {
		case strings.TrimSpace(info.TransactionID) == "":
			return AnalysisAssessment{
				Action:      OutcomeAskClearer,
				Reason:      "missing_transaction_id",
				Confidence:  confidence,
				IsAmbiguous: true,
			}
		case confidence < acceptThreshold:
			return AnalysisAssessment{
				Action:      OutcomeAskClearer,
				Reason:      "confidence_below_accept_threshold",
				Confidence:  confidence,
				IsAmbiguous: true,
			}
		default:
			return AnalysisAssessment{
				Action:     OutcomeAccept,
				Reason:     "accepted",
				Confidence: confidence,
			}
		}
	}

	if invalidReason == "" {
		invalidReason = "invalid_receipt"
	}
	if isUnclearInvalidReason(invalidReason) {
		return AnalysisAssessment{
			Action:      OutcomeAskClearer,
			Reason:      invalidReason,
			Confidence:  confidence,
			IsAmbiguous: true,
		}
	}
	if rawInvalidReason != "" && confidence < rejectThreshold {
		return AnalysisAssessment{
			Action:      OutcomeAskClearer,
			Reason:      "confidence_below_reject_threshold",
			Confidence:  confidence,
			IsAmbiguous: true,
		}
	}

	return AnalysisAssessment{
		Action:     OutcomeReject,
		Reason:     invalidReason,
		Confidence: confidence,
	}
}

func effectiveConfidence(info *PaymentInfo) float64 {
	if info == nil {
		return 0
	}

	confidence := info.Confidence
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return inferredConfidence(info)
	}
	if confidence < 0 || confidence > 1 {
		return inferredConfidence(info)
	}
	if confidence == 0 && hasMeaningfulSignal(info) {
		return inferredConfidence(info)
	}
	return confidence
}

func hasMeaningfulSignal(info *PaymentInfo) bool {
	return strings.TrimSpace(info.Provider) != "" ||
		strings.TrimSpace(info.TransactionID) != "" ||
		strings.TrimSpace(info.PhoneNumber) != "" ||
		strings.TrimSpace(info.RecipientName) != "" ||
		info.Amount > 0 ||
		strings.TrimSpace(info.InvalidReason) != "" ||
		info.IsValid
}

func inferredConfidence(info *PaymentInfo) float64 {
	if info == nil {
		return 0
	}

	score := 0.20
	if info.IsValid {
		score += 0.25
	}
	if strings.TrimSpace(info.TransactionID) != "" {
		score += 0.30
	}
	if info.Amount > 0 {
		score += 0.15
	}
	if strings.TrimSpace(info.Provider) != "" {
		score += 0.10
	}
	if strings.TrimSpace(info.PhoneNumber) != "" {
		score += 0.10
	}
	if strings.TrimSpace(info.RecipientName) != "" {
		score += 0.10
	}
	if info.NeedsClearerImage {
		score -= 0.20
	}
	if strings.TrimSpace(info.InvalidReason) != "" {
		score += 0.05
	}

	switch {
	case score < 0:
		return 0
	case score > 0.99:
		return 0.99
	default:
		return score
	}
}

func normalizeInvalidReason(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	replacer := strings.NewReplacer("-", "_", " ", "_")
	reason = replacer.Replace(reason)
	for strings.Contains(reason, "__") {
		reason = strings.ReplaceAll(reason, "__", "_")
	}
	return strings.Trim(reason, "_")
}

func isUnclearInvalidReason(reason string) bool {
	switch normalizeInvalidReason(reason) {
	case "clearer_image_required",
		"unclear_image",
		"blurry",
		"blurred",
		"cropped",
		"low_resolution",
		"obstructed",
		"partial_capture",
		"missing_required_fields",
		"receiver_not_visible",
		"transaction_id_not_visible",
		"amount_not_visible":
		return true
	default:
		return false
	}
}
