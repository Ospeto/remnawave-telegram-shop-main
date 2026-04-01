package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultOpenRouterModel = "openai/gpt-4.1-mini"

type OpenRouterClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenRouterClient(apiKey, model string) *OpenRouterClient {
	if model == "" {
		model = defaultOpenRouterModel
	}
	return &OpenRouterClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}
}

func (c *OpenRouterClient) Name() string {
	return "openrouter"
}

func (c *OpenRouterClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/key", nil)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	pingClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := pingClient.Do(req)
	if err != nil {
		return classifyProviderError(c.Name(), 0, err, "ping request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return classifyProviderError(c.Name(), resp.StatusCode, nil, providerErrorMessage(resp.StatusCode, respBody))
	}
	return nil
}

type openRouterRequest struct {
	Model       string              `json:"model"`
	Messages    []openRouterMessage `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
	Stream      bool                `json:"stream"`
	Metadata    map[string]string   `json:"metadata,omitempty"`
}

type openRouterMessage struct {
	Role    string                  `json:"role"`
	Content []openRouterMessagePart `json:"content"`
}

type openRouterMessagePart struct {
	Type     string                  `json:"type"`
	Text     string                  `json:"text,omitempty"`
	ImageURL *openRouterImageContent `json:"image_url,omitempty"`
}

type openRouterImageContent struct {
	URL string `json:"url"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenRouterClient) AnalyzePaymentScreenshot(ctx context.Context, imageBytes []byte, mimeType string, providers []ConfiguredProvider) (*PaymentInfo, error) {
	imageData := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imageBytes))
	reqBody := openRouterRequest{
		Model: c.model,
		Messages: []openRouterMessage{
			{
				Role: "user",
				Content: []openRouterMessagePart{
					{
						Type: "text",
						Text: BuildAnalysisPrompt(providers),
					},
					{
						Type: "image_url",
						ImageURL: &openRouterImageContent{
							URL: imageData,
						},
					},
				},
			},
		},
		MaxTokens:   400,
		Temperature: 0,
		Stream:      false,
		Metadata: map[string]string{
			"feature": "mobile_payment_verification",
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, classifyProviderError(c.Name(), 0, err, "failed to marshal request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, classifyProviderError(c.Name(), 0, err, "failed to create request")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

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

	var payload openRouterResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, classifyProviderError(c.Name(), http.StatusOK, err, "failed to parse response")
	}

	if payload.Error != nil {
		return nil, classifyProviderError(c.Name(), http.StatusOK, nil, payload.Error.Message)
	}

	if len(payload.Choices) == 0 {
		return nil, classifyProviderError(c.Name(), http.StatusOK, nil, "empty response")
	}

	rawText, err := extractOpenRouterText(payload.Choices[0].Message.Content)
	if err != nil {
		return nil, classifyProviderError(c.Name(), http.StatusOK, err, "failed to parse response content")
	}

	return parsePaymentInfo(c.Name(), rawText)
}

func extractOpenRouterText(content any) (string, error) {
	switch value := content.(type) {
	case string:
		return value, nil
	case []any:
		var parts []string
		for _, item := range value {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := obj["type"].(string)
			if partType != "" && partType != "text" && partType != "output_text" {
				continue
			}
			if text, ok := obj["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("no text content returned")
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("unsupported content type %T", content)
	}
}
