package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
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
	TamperingDetected bool    `json:"tampering_detected"`
}

// ConfiguredProvider is the minimal receiver configuration Gemini needs to
// analyze a payment screenshot accurately.
type ConfiguredProvider struct {
	Key         string
	Label       string
	Phone       string
	AccountName string
}

// Client communicates with the Gemini REST API.
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClient creates a Gemini REST client.
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
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
		return fmt.Errorf("gemini ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gemini ping returned status %d", resp.StatusCode)
	}
	return nil
}

// BuildAnalysisPrompt creates the Gemini analysis prompt with the current
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
  "is_valid": true if this looks like a genuine payment confirmation screenshot, false otherwise
}

Important:
- For provider, use one of: %s, or return an empty string if you cannot tell confidently.
- For transaction_id, look for labels like "Transaction ID", "Reference No", "Ref No", etc.
- For phone_number, look for the RECIPIENT/RECEIVER phone number. If partially masked (e.g. 09***2220), extract what is visible including asterisks.
- For recipient_name, extract the visible recipient/account holder name tied to the transfer target. Keep Burmese or English text as shown.
- For amount, extract only the numeric value of the FINAL PAID AMOUNT or TRANSFER AMOUNT. Ignore original price if discounted.
- For is_valid, set to false if: the image is not a payment screenshot, appears to be a screenshot of another screenshot, is heavily blurred, or shows no payment information
- Return ONLY the JSON object, nothing else`, providerSummary, receiverSection, strings.Join(providerKeys, ", "))
}

// geminiRequest matches the Gemini REST API request body.
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

// geminiResponse matches the relevant portion of the Gemini REST API response.
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

	prompt := BuildAnalysisPrompt(providers)

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
						Text: prompt,
					},
				},
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		c.model,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("Gemini API error", "status", resp.StatusCode, "body", string(respBody))
		return nil, fmt.Errorf("gemini API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse gemini response: %w", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("gemini API error %d: %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned empty response")
	}

	rawText := geminiResp.Candidates[0].Content.Parts[0].Text
	rawText = strings.TrimSpace(rawText)

	// Strip markdown code fences if present
	rawText = strings.TrimPrefix(rawText, "```json")
	rawText = strings.TrimPrefix(rawText, "```")
	rawText = strings.TrimSuffix(rawText, "```")
	rawText = strings.TrimSpace(rawText)

	var info PaymentInfo
	if err := json.Unmarshal([]byte(rawText), &info); err != nil {
		slog.Error("Failed to parse Gemini JSON output", "raw", rawText, "error", err)
		return nil, fmt.Errorf("failed to parse payment info from Gemini: %w", err)
	}

	return &info, nil
}
