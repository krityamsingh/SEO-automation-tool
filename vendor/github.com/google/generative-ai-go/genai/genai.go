package genai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/api/option"
)

type Part interface {
	part()
}

type Text string

func (t Text) part() {}

type Blob struct {
	MIMEType string
	Data     []byte
}

func (b Blob) part() {}

type Content struct {
	Parts []Part
	Role  string
}

type Candidate struct {
	Content *Content
}

type GenerateContentResponse struct {
	Candidates []*Candidate
}

type GenerativeModel struct {
	client           *Client
	Name             string
	ResponseMIMEType string
	Temperature      float32
	MaxOutputTokens  int32
}

func (m *GenerativeModel) SetTemperature(t float32) {
	m.Temperature = t
}

func (m *GenerativeModel) SetMaxOutputTokens(tokens int32) {
	m.MaxOutputTokens = tokens
}

type restRequest struct {
	Contents         []restContent         `json:"contents"`
	GenerationConfig *restGenerationConfig `json:"generationConfig,omitempty"`
}

type restContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []restPart `json:"parts"`
}

type restPart struct {
	Text string `json:"text,omitempty"`
}

type restGenerationConfig struct {
	Temperature      float32 `json:"temperature,omitempty"`
	MaxOutputTokens  int32   `json:"maxOutputTokens,omitempty"`
	ResponseMIMEType string  `json:"responseMimeType,omitempty"`
}

type restResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

type KeyStatus struct {
	Index    int    `json:"index"`
	KeyMask  string `json:"key_mask"`
	Provider string `json:"provider"`
	IsActive bool   `json:"is_active"`
}

func (m *GenerativeModel) GenerateContent(ctx context.Context, parts ...Part) (*GenerateContentResponse, error) {
	if m.client == nil || len(m.client.apiKeys) == 0 {
		return nil, fmt.Errorf("gemini client API keys pool is empty")
	}

	modelName := strings.TrimPrefix(m.Name, "models/")

	var promptText strings.Builder
	for _, p := range parts {
		if t, ok := p.(Text); ok {
			promptText.WriteString(string(t))
		}
	}

	maxAttempts := len(m.client.apiKeys)
	if maxAttempts < 3 {
		maxAttempts = 3
	}

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		apiKey := m.client.GetKey()
		provider := getProvider(apiKey)

		var resp *http.Response
		var err error

		if provider == "minimax" {
			// Call MiniMax API
			url := "https://api.minimaxi.chat/v1/chat/completions"
			mmPayload := map[string]interface{}{
				"model": "MiniMax-M3",
				"messages": []map[string]string{
					{"role": "user", "content": promptText.String()},
				},
			}
			jsonBytes, _ := json.Marshal(mmPayload)
			req, reqErr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
			if reqErr != nil {
				return nil, reqErr
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+apiKey)
			resp, err = m.client.httpClient.Do(req)
		} else {
			// Call Google Gemini API
			url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", modelName, apiKey)
			reqBody := restRequest{
				Contents: []restContent{
					{
						Role:  "user",
						Parts: []restPart{{Text: promptText.String()}},
					},
				},
			}
			if m.Temperature > 0 || m.MaxOutputTokens > 0 || m.ResponseMIMEType != "" {
				reqBody.GenerationConfig = &restGenerationConfig{
					Temperature:      m.Temperature,
					MaxOutputTokens:  m.MaxOutputTokens,
					ResponseMIMEType: m.ResponseMIMEType,
				}
			}
			jsonBytes, _ := json.Marshal(reqBody)
			req, reqErr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
			if reqErr != nil {
				return nil, reqErr
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err = m.client.httpClient.Do(req)
		}

		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			slog.Warn("API HTTP request failed, auto-rotating key", "provider", provider, "key_mask", maskKey(apiKey), "error", lastErr)
			m.client.RotateKey()
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			m.client.RotateKey()
			continue
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
			slog.Warn("API key error/quota failure, auto-shifting to next key",
				"provider", provider,
				"key_mask", maskKey(apiKey),
				"http_status", resp.StatusCode,
				"error", string(bodyBytes))
			m.client.RotateKey()
			continue
		}

		if provider == "minimax" {
			var openAiResp struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal(bodyBytes, &openAiResp); err != nil || len(openAiResp.Choices) == 0 {
				lastErr = fmt.Errorf("failed to decode MiniMax response: %s", string(bodyBytes))
				m.client.RotateKey()
				continue
			}
			res := &GenerateContentResponse{
				Candidates: []*Candidate{
					{
						Content: &Content{
							Parts: []Part{Text(openAiResp.Choices[0].Message.Content)},
							Role:  "model",
						},
					},
				},
			}
			return res, nil
		}

		var apiResp restResponse
		if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
			lastErr = fmt.Errorf("failed to decode Gemini API response: %w", err)
			continue
		}

		if apiResp.Error != nil {
			lastErr = fmt.Errorf("Gemini API error (%d %s): %s", apiResp.Error.Code, apiResp.Error.Status, apiResp.Error.Message)
			slog.Warn("Gemini API error, auto-shifting key", "key_mask", maskKey(apiKey), "error", apiResp.Error.Message)
			m.client.RotateKey()
			continue
		}

		res := &GenerateContentResponse{}
		for _, c := range apiResp.Candidates {
			var candParts []Part
			for _, p := range c.Content.Parts {
				candParts = append(candParts, Text(p.Text))
			}
			res.Candidates = append(res.Candidates, &Candidate{
				Content: &Content{
					Parts: candParts,
					Role:  c.Content.Role,
				},
			})
		}

		return res, nil
	}

	return nil, fmt.Errorf("all %d API key attempts/rotations exhausted: %w", maxAttempts, lastErr)
}

type Client struct {
	apiKeys    []string
	currentIdx uint32
	httpClient *http.Client
}

func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	var rawKeys string
	for _, opt := range opts {
		if keyStr, ok := opt.(fmt.Stringer); ok {
			rawKeys = keyStr.String()
		} else {
			str := fmt.Sprintf("%v", opt)
			if strings.HasPrefix(str, "{") && strings.HasSuffix(str, "}") {
				str = strings.Trim(str, "{}")
			}
			rawKeys = str
		}
	}

	rawKeys = strings.ReplaceAll(rawKeys, "\n", ",")
	rawKeys = strings.ReplaceAll(rawKeys, "\r", ",")
	rawKeys = strings.ReplaceAll(rawKeys, " ", ",")
	parts := strings.Split(rawKeys, ",")
	var apiKeys []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && !strings.HasPrefix(p, "xai-") { // Filter out any xai keys
			apiKeys = append(apiKeys, p)
		}
	}

	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("no valid API keys provided")
	}

	slog.Info("initialized API key rotation pool", "total_keys", len(apiKeys))

	return &Client{
		apiKeys: apiKeys,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}, nil
}

func (c *Client) GetKey() string {
	if len(c.apiKeys) == 0 {
		return ""
	}
	idx := atomic.LoadUint32(&c.currentIdx) % uint32(len(c.apiKeys))
	return c.apiKeys[idx]
}

func (c *Client) RotateKey() string {
	if len(c.apiKeys) <= 1 {
		return c.GetKey()
	}
	newVal := atomic.AddUint32(&c.currentIdx, 1)
	newIdx := newVal % uint32(len(c.apiKeys))
	slog.Info("rotated API key", "active_key_index", newIdx, "provider", getProvider(c.apiKeys[newIdx]), "key_mask", maskKey(c.apiKeys[newIdx]), "total_keys", len(c.apiKeys))
	return c.apiKeys[newIdx]
}

func (c *Client) GetKeyStatuses() []KeyStatus {
	current := atomic.LoadUint32(&c.currentIdx) % uint32(len(c.apiKeys))
	statuses := make([]KeyStatus, len(c.apiKeys))
	for i, k := range c.apiKeys {
		statuses[i] = KeyStatus{
			Index:    i,
			KeyMask:  maskKey(k),
			Provider: getProvider(k),
			IsActive: i == int(current),
		}
	}
	return statuses
}

func (c *Client) SelectKey(index int) error {
	if index < 0 || index >= len(c.apiKeys) {
		return fmt.Errorf("invalid key index: %d (valid range: 0-%d)", index, len(c.apiKeys)-1)
	}
	atomic.StoreUint32(&c.currentIdx, uint32(index))
	slog.Info("manually selected active API key", "index", index, "provider", getProvider(c.apiKeys[index]), "key_mask", maskKey(c.apiKeys[index]))
	return nil
}

func (c *Client) GenerativeModel(name string) *GenerativeModel {
	return &GenerativeModel{
		client: c,
		Name:   name,
	}
}

func (c *Client) Close() error {
	return nil
}

func getProvider(key string) string {
	if strings.HasPrefix(key, "sk-api-") {
		return "minimax"
	}
	return "gemini"
}

func maskKey(key string) string {
	if len(key) <= 10 {
		return "..."
	}
	return key[:6] + "..." + key[len(key)-4:]
}
