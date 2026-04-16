package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultGeminiModel    = "gemini-3.1-flash-lite-preview"
	defaultRequestTimeout = 60 * time.Second
)

// PaymentInfo holds the fields extracted from a mobile banking screenshot.
type PaymentInfo struct {
	Provider          string  `json:"provider"`
	TransactionID     string  `json:"transaction_id"`
	PhoneNumber       string  `json:"phone_number"`
	RecipientName     string  `json:"recipient_name"`
	Amount            float64 `json:"amount"`
	Note              string  `json:"note"`
	IsValid           bool    `json:"is_valid"`
	Confidence        float64 `json:"confidence"`
	NeedsClearerImage bool    `json:"needs_clearer_image"`
	InvalidReason     string  `json:"invalid_reason"`
	TamperingDetected bool    `json:"tampering_detected"`
}

// ConfiguredProvider is the minimal receiver configuration the analyzer needs
// to validate screenshot recipients accurately.
type ConfiguredProvider struct {
	Key         string
	Label       string
	Phone       string
	AccountName string
}

// Analyzer is the provider-neutral screenshot analysis contract used by the
// payment flow.
type Analyzer interface {
	AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string, providers []ConfiguredProvider) (*PaymentInfo, error)
	Readiness(ctx context.Context) AnalyzerReadiness
}

type Provider interface {
	Name() string
	AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string, providers []ConfiguredProvider) (*PaymentInfo, error)
	Ping(ctx context.Context) error
}

type ErrorClass string

const (
	ErrorClassTimeout           ErrorClass = "timeout"
	ErrorClassCanceled          ErrorClass = "canceled"
	ErrorClassTransport         ErrorClass = "transport"
	ErrorClassAuth              ErrorClass = "auth"
	ErrorClassRateLimit         ErrorClass = "rate_limit"
	ErrorClassServer            ErrorClass = "server"
	ErrorClassMalformedResponse ErrorClass = "malformed_response"
	ErrorClassClient            ErrorClass = "client"
	ErrorClassUnknown           ErrorClass = "unknown"
)

type ProviderError struct {
	Provider   string
	Class      ErrorClass
	Message    string
	StatusCode int
	Err        error
}

func (e *ProviderError) Error() string {
	base := strings.TrimSpace(e.Message)
	if base == "" && e.Err != nil {
		base = e.Err.Error()
	}
	if base == "" {
		base = "provider request failed"
	}
	if e.Provider == "" {
		return base
	}
	return fmt.Sprintf("%s: %s", e.Provider, base)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func (e *ProviderError) AllowsRetry() bool {
	switch e.Class {
	case ErrorClassTimeout, ErrorClassTransport, ErrorClassRateLimit, ErrorClassServer:
		return true
	default:
		return false
	}
}

func (e *ProviderError) AllowsFailover() bool {
	switch e.Class {
	case ErrorClassTimeout, ErrorClassTransport, ErrorClassAuth, ErrorClassRateLimit, ErrorClassServer, ErrorClassMalformedResponse:
		return true
	default:
		return false
	}
}

type AnalyzerReadiness struct {
	Status    string            `json:"status"`
	Primary   string            `json:"primary"`
	Fallback  string            `json:"fallback,omitempty"`
	Providers map[string]string `json:"providers"`
}

type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClient creates a Gemini REST provider client.
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = defaultGeminiModel
	}
	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}
}

func (c *Client) Name() string {
	return "gemini"
}

// Ping verifies the Gemini API key is valid by listing models.
func (c *Client) Ping(ctx context.Context) error {
	url := "https://generativelanguage.googleapis.com/v1beta/models?pageSize=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}
	req.Header.Set("x-goog-api-key", c.apiKey)

	pingClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := pingClient.Do(req)
	if err != nil {
		return classifyProviderError(c.Name(), 0, err, "ping request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return classifyProviderError(c.Name(), resp.StatusCode, nil, fmt.Sprintf("ping returned status %d", resp.StatusCode))
	}
	return nil
}

// BuildAnalysisPrompt creates the shared analysis prompt with the current
// enabled mobile banking providers.
func BuildAnalysisPrompt(providers []ConfiguredProvider) string {
	providerLabels := make([]string, 0, len(providers))
	providerKeys := make([]string, 0, len(providers))
	providerLines := make([]string, 0, len(providers))
	for _, provider := range providers {
		providerLabels = append(providerLabels, provider.Label)
		providerKeys = append(providerKeys, fmt.Sprintf("%q", provider.Key))

		line := fmt.Sprintf("  - %s: phone %s", provider.Label, provider.Phone)
		if strings.TrimSpace(provider.AccountName) != "" {
			line += fmt.Sprintf(", account name %q", provider.AccountName)
		}
		providerLines = append(providerLines, line)
	}

	providerSummary := "a Myanmar mobile banking app"
	if len(providerLabels) > 0 {
		providerSummary = strings.Join(providerLabels, ", ")
	}

	receiverSection := ""
	if len(providerLines) > 0 {
		receiverSection = fmt.Sprintf("\n\nOur configured receiving accounts are:\n%s\nThe receipt should match one of these providers. The phone number may be partially masked with asterisks. The recipient name may differ by provider, so extract the visible recipient/account name exactly as shown.", strings.Join(providerLines, "\n"))
	}

	return fmt.Sprintf(`Analyze this Myanmar mobile banking payment screenshot.
This is a screenshot from one of these providers: %s.%s

Extract the following fields and respond ONLY with valid JSON (no markdown, no code fences):

{
  "provider": "provider key or empty string if unclear",
  "transaction_id": "the unique transaction/reference ID string",
  "phone_number": "the recipient phone number who received the money (even if partially masked with asterisks, extract whatever is visible)",
  "recipient_name": "the recipient/account holder name who received the money, empty string if not visible",
  "amount": numeric payment amount (no currency symbol, just the number),
  "note": "any note or remark text, empty string if none",
  "is_valid": true if this looks like a genuine payment confirmation screenshot, false otherwise,
  "confidence": number from 0.0 to 1.0 representing confidence in the overall screenshot analysis,
  "needs_clearer_image": true if blur/cropping/low resolution/obstruction make safe verification impossible,
  "invalid_reason": "empty string when valid, otherwise one of unclear_image, missing_required_fields, not_payment_confirmation, tampering_detected, or other short snake_case reason"
}

Important:
- For provider, use one of: %s, or return an empty string if you cannot tell confidently.
- For transaction_id, look for labels like "Transaction ID", "Reference No", "Ref No", etc.
- For phone_number, look for the RECIPIENT/RECEIVER phone number. If partially masked (e.g. 09***2220), extract what is visible including asterisks.
- For recipient_name, extract the visible recipient/account holder name tied to the transfer target. Keep Burmese or English text as shown.
- For amount, extract only the numeric value of the FINAL PAID AMOUNT or TRANSFER AMOUNT. Ignore original price if discounted.
- For is_valid, set to false if: the image is not a payment screenshot, appears to be a screenshot of another screenshot, is heavily blurred, or shows no payment information
- For confidence, return a conservative number. Use high confidence only when the screenshot is clear and the core fields are visible.
- For needs_clearer_image, set true when a safer action is to ask the user for a clearer screenshot rather than reject outright.
- For invalid_reason, use "unclear_image" for blur/crop/obstruction, "missing_required_fields" when key fields are unreadable, "not_payment_confirmation" when the image is not a payment confirmation, and "tampering_detected" when it looks edited or re-captured.
- Return ONLY the JSON object, nothing else`, providerSummary, receiverSection, strings.Join(providerKeys, ", "))
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inline_data,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// AnalyzePaymentScreenshot sends image bytes to Gemini and returns extracted payment info.
func (c *Client) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string, providers []ConfiguredProvider) (*PaymentInfo, error) {
	b64Image := base64.StdEncoding.EncodeToString(imageBytes)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{
						InlineData: &inlineData{
							MimeType: mimeType,
							Data:     b64Image,
						},
					},
					{
						Text: BuildAnalysisPrompt(providers),
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, classifyProviderError(c.Name(), 0, err, "failed to marshal request")
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, classifyProviderError(c.Name(), 0, err, "failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, classifyProviderError(c.Name(), 0, err, "request failed")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, classifyProviderError(c.Name(), 0, err, "failed to read response")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, classifyProviderError(c.Name(), resp.StatusCode, nil, providerErrorMessage(resp.StatusCode, respBody))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, classifyProviderError(c.Name(), http.StatusOK, err, "failed to parse response")
	}

	if geminiResp.Error != nil {
		return nil, classifyProviderError(c.Name(), geminiResp.Error.Code, nil, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, classifyProviderError(c.Name(), http.StatusOK, nil, "empty response")
	}

	return parsePaymentInfo(c.Name(), geminiResp.Candidates[0].Content.Parts[0].Text)
}

func stripJSONFences(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	return strings.TrimSpace(raw)
}

func parsePaymentInfo(providerName, rawText string) (*PaymentInfo, error) {
	rawText = stripJSONFences(rawText)

	var info PaymentInfo
	if err := json.Unmarshal([]byte(rawText), &info); err != nil {
		return nil, classifyProviderError(providerName, http.StatusOK, err, "failed to parse payment info JSON")
	}
	info.Provider = strings.TrimSpace(info.Provider)
	info.TransactionID = strings.TrimSpace(info.TransactionID)
	info.PhoneNumber = strings.TrimSpace(info.PhoneNumber)
	info.RecipientName = strings.TrimSpace(info.RecipientName)
	info.Note = strings.TrimSpace(info.Note)
	info.InvalidReason = normalizeInvalidReason(info.InvalidReason)

	return &info, nil
}

func providerErrorMessage(statusCode int, respBody []byte) string {
	type errorEnvelope struct {
		Error *struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	var envelope errorEnvelope
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
		return envelope.Error.Message
	}
	return fmt.Sprintf("status %d", statusCode)
}

func classifyProviderError(provider string, statusCode int, err error, message string) error {
	if providerErr := newProviderError(provider, statusCode, err, message); providerErr != nil {
		return providerErr
	}
	if err != nil {
		return fmt.Errorf("%s: %w", provider, err)
	}
	return fmt.Errorf("%s: %s", provider, strings.TrimSpace(message))
}

func newProviderError(provider string, statusCode int, err error, message string) *ProviderError {
	class := ErrorClassUnknown
	switch {
	case errors.Is(err, context.Canceled):
		class = ErrorClassCanceled
	case errors.Is(err, context.DeadlineExceeded):
		class = ErrorClassTimeout
	default:
		var netErr net.Error
		if errors.As(err, &netErr) {
			if netErr.Timeout() {
				class = ErrorClassTimeout
			} else {
				class = ErrorClassTransport
			}
		}
	}

	if class == ErrorClassUnknown {
		switch {
		case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
			class = ErrorClassAuth
		case statusCode == http.StatusTooManyRequests || statusCode == http.StatusPaymentRequired:
			class = ErrorClassRateLimit
		case statusCode >= http.StatusInternalServerError:
			class = ErrorClassServer
		case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
			class = ErrorClassClient
		case err != nil && strings.Contains(strings.ToLower(err.Error()), "parse"):
			class = ErrorClassMalformedResponse
		case strings.Contains(strings.ToLower(message), "parse"):
			class = ErrorClassMalformedResponse
		case strings.Contains(strings.ToLower(message), "empty response"):
			class = ErrorClassMalformedResponse
		}
	}

	if class == ErrorClassUnknown {
		class = ErrorClassTransport
	}

	return &ProviderError{
		Provider:   provider,
		Class:      class,
		Message:    strings.TrimSpace(message),
		StatusCode: statusCode,
		Err:        err,
	}
}

func AsProviderError(err error) (*ProviderError, bool) {
	if err == nil {
		return nil, false
	}

	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr, true
	}
	return nil, false
}
