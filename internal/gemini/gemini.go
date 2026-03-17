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
	"remnawave-tg-shop-bot/internal/receiptai"
	"strings"
	"time"
)

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

func (c *Client) ProviderName() string {
	return "Gemini"
}

func (c *Client) generateContent(ctx context.Context, parts []geminiPart) (string, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: parts,
			},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		c.model,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("Gemini API error", "status", resp.StatusCode, "body", string(respBody))
		return "", fmt.Errorf("gemini API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to parse gemini response: %w", err)
	}

	if geminiResp.Error != nil {
		return "", fmt.Errorf("gemini API error %d: %s", geminiResp.Error.Code, geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned empty response")
	}

	return strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text), nil
}

func (c *Client) CheckHealth(ctx context.Context) error {
	_, err := c.generateContent(ctx, []geminiPart{{Text: receiptai.HealthCheckPrompt}})
	return err
}

// AnalyzePaymentScreenshot sends image bytes to Gemini and returns extracted payment info.
func (c *Client) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string) (*receiptai.PaymentInfo, error) {
	b64Image := base64.StdEncoding.EncodeToString(imageBytes)

	rawText, err := c.generateContent(ctx, []geminiPart{
		{
			InlineData: &inlineData{
				MimeType: mimeType,
				Data:     b64Image,
			},
		},
		{
			Text: receiptai.AnalysisPrompt,
		},
	})
	if err != nil {
		return nil, err
	}

	// Strip markdown code fences if present
	rawText = strings.TrimPrefix(rawText, "```json")
	rawText = strings.TrimPrefix(rawText, "```")
	rawText = strings.TrimSuffix(rawText, "```")
	rawText = strings.TrimSpace(rawText)

	var info receiptai.PaymentInfo
	if err := json.Unmarshal([]byte(rawText), &info); err != nil {
		slog.Error("Failed to parse Gemini JSON output", "raw", rawText, "error", err)
		return nil, fmt.Errorf("failed to parse payment info from Gemini: %w", err)
	}

	return &info, nil
}
