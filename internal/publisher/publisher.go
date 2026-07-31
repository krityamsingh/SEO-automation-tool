package publisher

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"aeo_geo_seo_agent/internal/util"
)

// Publisher interfaces for various content platforms

type Publisher interface {
	Publish(title, body, meta string) (*PublishResult, error)
	IsConfigured() bool
}

type PublishResult struct {
	URL         string    `json:"url"`
	ID          string    `json:"id"`
	Platform    string    `json:"platform"`
	PublishedAt time.Time `json:"published_at"`
}

type PublisherRegistry struct {
	publishers []Publisher
}

func NewRegistry(publishers ...Publisher) *PublisherRegistry {
	var configured []Publisher
	for _, p := range publishers {
		if p.IsConfigured() {
			configured = append(configured, p)
		}
	}
	return &PublisherRegistry{publishers: configured}
}

func (r *PublisherRegistry) PublishAll(title, body, meta string) []*PublishResult {
	var results []*PublishResult
	for _, p := range r.publishers {
		result, err := p.Publish(title, body, meta)
		if err != nil {
			slog.Error("publish failed", "error", err)
			continue
		}
		results = append(results, result)
	}
	return results
}

func (r *PublisherRegistry) HasPublishers() bool {
	return len(r.publishers) > 0
}

// WordPress Publisher

type WordPressPublisher struct {
	URL         string
	Username    string
	AppPassword string
	client      *http.Client
}

func NewWordPress(url, username, password string) *WordPressPublisher {
	return &WordPressPublisher{
		URL:         url,
		Username:    username,
		AppPassword: password,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *WordPressPublisher) IsConfigured() bool {
	return w.URL != "" && w.Username != "" && w.AppPassword != ""
}

func (w *WordPressPublisher) doWithRetry(reqFunc func() (*http.Request, error)) (*http.Response, error) {
	var resp *http.Response
	var err error

	retryFn := func() error {
		req, err := reqFunc()
		if err != nil {
			return err
		}
		resp, err = w.client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("WordPress publish failed: HTTP %d: %s", resp.StatusCode, string(body))
		}
		return nil
	}

	cfg := util.DefaultRetryConfig()
	err = util.WithRetry(context.Background(), cfg, "wordpress_publish", retryFn)
	return resp, err
}

func (w *WordPressPublisher) Publish(title, body, meta string) (*PublishResult, error) {
	if !w.IsConfigured() {
		return nil, fmt.Errorf("WordPress not configured")
	}

	post := map[string]interface{}{
		"title":   title,
		"content": body,
		"status":  "publish",
	}

	jsonData, _ := json.Marshal(post)
	
	reqFunc := func() (*http.Request, error) {
		req, err := http.NewRequest("POST", w.URL+"/wp-json/wp/v2/posts", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(w.Username, w.AppPassword)
		return req, nil
	}

	resp, err := w.doWithRetry(reqFunc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	id := fmt.Sprintf("%v", result["id"])
	postURL := fmt.Sprintf("%v", result["link"])

	slog.Info("published to WordPress", "title", title, "url", postURL)

	return &PublishResult{
		URL:         postURL,
		ID:          id,
		Platform:    "wordpress",
		PublishedAt: time.Now(),
	}, nil
}

// Medium Publisher

type MediumPublisher struct {
	IntegrationToken string
	client           *http.Client
}

func NewMedium(token string) *MediumPublisher {
	return &MediumPublisher{
		IntegrationToken: token,
		client:           &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *MediumPublisher) IsConfigured() bool {
	return m.IntegrationToken != ""
}

func (m *MediumPublisher) doWithRetry(reqFunc func() (*http.Request, error)) (*http.Response, error) {
	var resp *http.Response
	var err error

	retryFn := func() error {
		req, err := reqFunc()
		if err != nil {
			return err
		}
		resp, err = m.client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("Medium API failed: HTTP %d: %s", resp.StatusCode, string(body))
		}
		return nil
	}

	cfg := util.DefaultRetryConfig()
	err = util.WithRetry(context.Background(), cfg, "medium_api", retryFn)
	return resp, err
}

func (m *MediumPublisher) getUserID() (string, error) {
	reqFunc := func() (*http.Request, error) {
		req, err := http.NewRequest("GET", "https://api.medium.com/v1/me", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+m.IntegrationToken)
		req.Header.Set("Accept", "application/json")
		return req, nil
	}

	resp, err := m.doWithRetry(reqFunc)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Data.ID, nil
}

func (m *MediumPublisher) Publish(title, body, meta string) (*PublishResult, error) {
	if !m.IsConfigured() {
		return nil, fmt.Errorf("Medium not configured")
	}

	userID, err := m.getUserID()
	if err != nil {
		return nil, fmt.Errorf("failed to get Medium user ID: %w", err)
	}

	post := map[string]interface{}{
		"title":         title,
		"contentFormat": "markdown",
		"content":       body,
		"publishStatus": "public",
	}

	jsonData, _ := json.Marshal(post)
	url := fmt.Sprintf("https://api.medium.com/v1/users/%s/posts", userID)
	
	reqFunc := func() (*http.Request, error) {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+m.IntegrationToken)
		return req, nil
	}

	resp, err := m.doWithRetry(reqFunc)
	if err != nil {
		return nil, fmt.Errorf("Medium publish failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	data, _ := result["data"].(map[string]interface{})
	id := fmt.Sprintf("%v", data["id"])
	postURL := fmt.Sprintf("%v", data["url"])

	slog.Info("published to Medium", "title", title, "url", postURL)

	return &PublishResult{
		URL:         postURL,
		ID:          id,
		Platform:    "medium",
		PublishedAt: time.Now(),
	}, nil
}

// Ghost Publisher

type GhostPublisher struct {
	URL         string
	AdminAPIKey string
	client      *http.Client
}

func NewGhost(url, apiKey string) *GhostPublisher {
	return &GhostPublisher{
		URL:         url,
		AdminAPIKey: apiKey,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GhostPublisher) IsConfigured() bool {
	return g.URL != "" && g.AdminAPIKey != ""
}

func (g *GhostPublisher) doWithRetry(reqFunc func() (*http.Request, error)) (*http.Response, error) {
	var resp *http.Response
	var err error

	retryFn := func() error {
		req, err := reqFunc()
		if err != nil {
			return err
		}
		resp, err = g.client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("Ghost publish failed: HTTP %d: %s", resp.StatusCode, string(body))
		}
		return nil
	}

	cfg := util.DefaultRetryConfig()
	err = util.WithRetry(context.Background(), cfg, "ghost_publish", retryFn)
	return resp, err
}

func (g *GhostPublisher) generateGhostJWT() (string, error) {
	parts := strings.Split(g.AdminAPIKey, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid Ghost Admin API key format")
	}
	keyID := parts[0]
	secretHex := parts[1]

	secret, err := hex.DecodeString(secretHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret hex: %w", err)
	}

	header := fmt.Sprintf(`{"alg":"HS256","typ":"JWT","kid":"%s"}`, keyID)
	now := time.Now().Unix()
	payload := fmt.Sprintf(`{"iat":%d,"exp":%d,"aud":"/admin/"}`, now, now+300)

	encodedHeader := base64.RawURLEncoding.EncodeToString([]byte(header))
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))

	signingInput := encodedHeader + "." + encodedPayload

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

func (g *GhostPublisher) Publish(title, body, meta string) (*PublishResult, error) {
	if !g.IsConfigured() {
		return nil, fmt.Errorf("Ghost not configured")
	}

	jwtToken, err := g.generateGhostJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ghost JWT: %w", err)
	}

	post := map[string]interface{}{
		"posts": []map[string]interface{}{
			{
				"title":  title,
				"html":   body,
				"status": "published",
			},
		},
	}

	jsonData, _ := json.Marshal(post)
	
	reqFunc := func() (*http.Request, error) {
		req, err := http.NewRequest("POST", g.URL+"/ghost/api/admin/posts/", bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Ghost "+jwtToken)
		return req, nil
	}

	resp, err := g.doWithRetry(reqFunc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	posts, _ := result["posts"].([]interface{})
	if len(posts) > 0 {
		postData, _ := posts[0].(map[string]interface{})
		id := fmt.Sprintf("%v", postData["id"])
		slug := fmt.Sprintf("%v", postData["slug"])
		postURL := g.URL + "/" + slug + "/"

		slog.Info("published to Ghost", "title", title, "url", postURL)

		return &PublishResult{
			URL:         postURL,
			ID:          id,
			Platform:    "ghost",
			PublishedAt: time.Now(),
		}, nil
	}

	return nil, fmt.Errorf("unexpected Ghost response")
}

// Webhook Publisher

type WebhookPublisher struct {
	URL    string
	client *http.Client
}

func NewWebhook(url string) *WebhookPublisher {
	return &WebhookPublisher{
		URL:    url,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *WebhookPublisher) IsConfigured() bool {
	return w.URL != ""
}

func (w *WebhookPublisher) doWithRetry(reqFunc func() (*http.Request, error)) (*http.Response, error) {
	var resp *http.Response
	var err error

	retryFn := func() error {
		req, err := reqFunc()
		if err != nil {
			return err
		}
		resp, err = w.client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("Webhook publish failed: HTTP %d: %s", resp.StatusCode, string(body))
		}
		return nil
	}

	cfg := util.DefaultRetryConfig()
	err = util.WithRetry(context.Background(), cfg, "webhook_publish", retryFn)
	return resp, err
}

func (w *WebhookPublisher) Publish(title, body, meta string) (*PublishResult, error) {
	if !w.IsConfigured() {
		return nil, fmt.Errorf("Webhook not configured")
	}

	payload := map[string]interface{}{
		"title":     title,
		"body":      body,
		"meta":      meta,
		"platform":  "webhook",
		"timestamp": time.Now().UTC(),
	}

	jsonData, _ := json.Marshal(payload)
	
	reqFunc := func() (*http.Request, error) {
		req, err := http.NewRequest("POST", w.URL, bytes.NewBuffer(jsonData))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "AEOAgent/1.0")
		return req, nil
	}

	resp, err := w.doWithRetry(reqFunc)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	slog.Info("published via webhook", "title", title, "url", w.URL, "status", resp.StatusCode)

	return &PublishResult{
		URL:         w.URL,
		ID:          "",
		Platform:    "webhook",
		PublishedAt: time.Now(),
	}, nil
}
