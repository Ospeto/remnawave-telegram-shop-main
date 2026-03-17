package receiptai

import "context"

// PaymentInfo holds the fields extracted from a mobile banking screenshot.
type PaymentInfo struct {
	Provider          string  `json:"provider"`
	TransactionID     string  `json:"transaction_id"`
	PhoneNumber       string  `json:"phone_number"`
	Amount            float64 `json:"amount"`
	Note              string  `json:"note"`
	IsValid           bool    `json:"is_valid"`
	TamperingDetected bool    `json:"tampering_detected"`
}

type Analyzer interface {
	AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string) (*PaymentInfo, error)
	CheckHealth(ctx context.Context) error
	ProviderName() string
}

const AnalysisPrompt = `Analyze this mobile banking payment screenshot from Myanmar. 
This is a screenshot from KPay, WavePay, or AyaPay.

Extract the following fields and respond ONLY with valid JSON (no markdown, no code fences):

{
  "provider": "kpay" or "wavepay" or "ayapay",
  "transaction_id": "the unique transaction/reference ID string",
  "phone_number": "the recipient phone number who received the money (even if partially masked with asterisks, extract whatever is visible)",
  "amount": numeric payment amount (no currency symbol, just the number),
  "note": "any note or remark text, empty string if none",
  "is_valid": true if this looks like a genuine payment confirmation screenshot, false otherwise,
  "tampering_detected": true if there are signs of image manipulation (Photoshop, AI generation, font inconsistencies, unnatural edges, misaligned text, cloned regions, resolution differences between areas, or other editing artifacts), false if the image appears authentic
}

Important:
- For transaction_id, look for labels like "Transaction ID", "Reference No", "Ref No", etc.
- For phone_number, look for the RECIPIENT/RECEIVER phone number. If partially masked (e.g. 09***2220), extract what is visible including asterisks.
- For amount, extract only the numeric value of the FINAL PAID AMOUNT or TRANSFER AMOUNT. Ignore original price if discounted.
- For is_valid, set to false if: the image is not a payment screenshot, appears to be a screenshot of another screenshot, is heavily blurred, or shows no payment information
- For tampering_detected, carefully check for: inconsistent fonts or text sizes, pixel-level editing artifacts, unnatural sharp edges around text or numbers, areas with different compression levels, mismatched drop shadows, any signs of cut-paste or cloning, AI-generated content indicators
- Return ONLY the JSON object, nothing else`

const HealthCheckPrompt = `Reply with exactly OK. No markdown, no extra text.`
