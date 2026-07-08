// Package anthropic is a minimal client for the Anthropic Messages and
// Models APIs — just what xlocal needs for translating strings.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"
	maxAttempts    = 3
)

type Client struct {
	APIKey     string
	Model      string // resolved model ID used for Complete
	BaseURL    string
	HTTPClient *http.Client
	retryDelay time.Duration
}

func New(apiKey, model string) *Client {
	return &Client{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		retryDelay: 2 * time.Second,
	}
}

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// Complete sends a single-user-message request and returns the trimmed text
// response. Server-side errors (429, 5xx, 529 overloaded) are retried with
// backoff; client errors fail immediately.
func (c *Client) Complete(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(messagesRequest{
		Model:     c.Model,
		MaxTokens: 1000,
		Messages:  []message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(attempt-1)):
			}
		}

		text, retryable, err := c.completeOnce(ctx, body)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !retryable {
			return "", err
		}
	}

	return "", fmt.Errorf("giving up after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) completeOnce(ctx context.Context, body []byte) (text string, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return "", retryable, fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", false, err
	}

	var sb strings.Builder
	for _, block := range parsed.Content {
		sb.WriteString(block.Text)
	}
	text = strings.TrimSpace(sb.String())
	if text == "" {
		return "", true, fmt.Errorf("no text in response")
	}
	return text, false, nil
}

type modelsResponse struct {
	Data []struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"created_at"`
	} `json:"data"`
}

// ResolveModel turns a model alias like "sonnet" into the newest matching
// model ID via the Models API. Full model IDs ("claude-...") pass through
// without a network call.
func (c *Client) ResolveModel(ctx context.Context, model string) (string, error) {
	if strings.HasPrefix(model, "claude-") {
		return model, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/models?limit=100", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", apiVersion)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("listing models failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}

	alias := strings.ToLower(model)
	var matches []struct {
		ID        string
		CreatedAt time.Time
	}
	for _, m := range parsed.Data {
		if strings.Contains(strings.ToLower(m.ID), alias) {
			matches = append(matches, struct {
				ID        string
				CreatedAt time.Time
			}{m.ID, m.CreatedAt})
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no model matches %q (try sonnet, opus, haiku, or a full model ID)", model)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].CreatedAt.After(matches[j].CreatedAt)
	})
	return matches[0].ID, nil
}
