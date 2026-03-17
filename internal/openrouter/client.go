package openrouter

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

type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "google/gemini-2.5-flash"
	}

	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) ProviderName() string {
	return "OpenRouter"
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type contentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *imageURLPart `json:"image_url,omitempty"`
}

type imageURLPart struct {
	URL string `json:"url"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content interface{} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) generateContent(ctx context.Context, messages []chatMessage, maxTokens int) (string, error) {
	reqBody := chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0,
		MaxTokens:   maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openrouter API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("OpenRouter API error", "status", resp.StatusCode, "body", string(respBody))
		return "", fmt.Errorf("openrouter API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return "", fmt.Errorf("failed to parse OpenRouter response: %w", err)
	}

	if completion.Error != nil {
		return "", fmt.Errorf("openrouter API error: %s", completion.Error.Message)
	}

	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("openrouter returned empty response")
	}

	return parseMessageContent(completion.Choices[0].Message.Content)
}

func parseMessageContent(content interface{}) (string, error) {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value), nil
	case []interface{}:
		var parts []string
		for _, item := range value {
			part, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			text, _ := part["text"].(string)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("openrouter response did not include text content")
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("unsupported OpenRouter content type %T", content)
	}
}

func (c *Client) CheckHealth(ctx context.Context) error {
	_, err := c.generateContent(ctx, []chatMessage{
		{
			Role:    "user",
			Content: receiptai.HealthCheckPrompt,
		},
	}, 16)
	return err
}

func (c *Client) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string) (*receiptai.PaymentInfo, error) {
	b64Image := base64.StdEncoding.EncodeToString(imageBytes)
	rawText, err := c.generateContent(ctx, []chatMessage{
		{
			Role: "user",
			Content: []contentPart{
				{
					Type: "text",
					Text: receiptai.AnalysisPrompt,
				},
				{
					Type: "image_url",
					ImageURL: &imageURLPart{
						URL: fmt.Sprintf("data:%s;base64,%s", mimeType, b64Image),
					},
				},
			},
		},
	}, 400)
	if err != nil {
		return nil, err
	}

	rawText = strings.TrimPrefix(rawText, "```json")
	rawText = strings.TrimPrefix(rawText, "```")
	rawText = strings.TrimSuffix(rawText, "```")
	rawText = strings.TrimSpace(rawText)

	var info receiptai.PaymentInfo
	if err := json.Unmarshal([]byte(rawText), &info); err != nil {
		slog.Error("Failed to parse OpenRouter JSON output", "raw", rawText, "error", err)
		return nil, fmt.Errorf("failed to parse payment info from OpenRouter: %w", err)
	}

	return &info, nil
}
