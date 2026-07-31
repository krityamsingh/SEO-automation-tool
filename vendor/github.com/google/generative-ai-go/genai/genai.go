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
	Contents         []restContent          `json:"contents"`
	GenerationConfig *restGenerationConfig  `json:"generationConfig,omitempty"`
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

func (m *GenerativeModel) GenerateContent(ctx context.Context, parts ...Part) (*GenerateContentResponse, error) {
	if m.client == nil || len(m.client.apiKeys) == 0 {
		return nil, fmt.Errorf("gemini client API keys pool is empty")
	}

	modelName := strings.TrimPrefix(m.Name, "models/")

	var restParts []restPart
	for _, p := range parts {
		if t, ok := p.(Text); ok {
			restParts = append(restParts, restPart{Text: string(t)})
		}
	}

	reqBody := restRequest{
		Contents: []restContent{
			{
				Role:  "user",
				Parts: restParts,
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

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	maxAttempts := len(m.client.apiKeys)
	if maxAttempts < 3 {
		maxAttempts = 3 // at least try retries
	}

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		apiKey := m.client.GetKey()
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", modelName, apiKey)

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := m.client.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("gemini HTTP request failed: %w", err)
			slog.Warn("gemini API HTTP error, attempting key rotation", "error", lastErr)
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

		if resp.StatusCode == 429 || resp.StatusCode == 403 || resp.StatusCode == 401 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("gemini API error (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
			slog.Warn("gemini API key rate limited or error, auto-rotating to next key in pool",
				"http_status", resp.StatusCode,
				"total_keys", len(m.client.apiKeys),
				"error", string(bodyBytes))
			m.client.RotateKey()
			continue
		}

		var apiResp restResponse
		if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
			lastErr = fmt.Errorf("failed to decode gemini API response: %w", err)
			continue
		}

		if apiResp.Error != nil {
			lastErr = fmt.Errorf("gemini API error (%d %s): %s", apiResp.Error.Code, apiResp.Error.Status, apiResp.Error.Message)
			slog.Warn("gemini API returned error, auto-rotating key", "error", apiResp.Error.Message)
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

	// Parse keys separated by comma, newline, or space
	rawKeys = strings.ReplaceAll(rawKeys, "\n", ",")
	rawKeys = strings.ReplaceAll(rawKeys, "\r", ",")
	rawKeys = strings.ReplaceAll(rawKeys, " ", ",")
	parts := strings.Split(rawKeys, ",")
	var apiKeys []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			apiKeys = append(apiKeys, p)
		}
	}

	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("no valid Gemini API keys provided")
	}

	slog.Info("initialized Gemini client with API key pool", "key_count", len(apiKeys))

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
	slog.Info("rotated Gemini API key", "new_index", newIdx, "total_keys", len(c.apiKeys))
	return c.apiKeys[newIdx]
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
