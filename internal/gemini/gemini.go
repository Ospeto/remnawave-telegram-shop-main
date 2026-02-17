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
	Amount            float64 `json:"amount"`
	Note              string  `json:"note"`
	IsValid           bool    `json:"is_valid"`
	TamperingDetected bool    `json:"tampering_detected"`
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

const analysisPrompt = `Analyze this mobile banking payment screenshot from Myanmar. 
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
func (c *Client) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string) (*PaymentInfo, error) {
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
						Text: analysisPrompt,
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
