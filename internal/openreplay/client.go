// Package openreplay provides an authenticated client for the dashboard API.
package openreplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrNotFound indicates that the dashboard project does not exist.
	ErrNotFound = errors.New("OpenReplay project not found")
	// ErrUnauthorized indicates that an owner/admin dashboard JWT is required.
	ErrUnauthorized = errors.New("OpenReplay authorization failed")
)

// Client communicates with a single OpenReplay dashboard API.
type Client struct {
	endpoint url.URL
	token    string
	http     *http.Client
}

// NewClient validates the endpoint and token without contacting the server.
// Endpoints without a path use /api; an existing /api suffix is preserved.
func NewClient(endpoint, token string) (*Client, error) {
	base, err := url.Parse(endpoint)
	if err != nil || base == nil || !base.IsAbs() || (base.Scheme != "http" && base.Scheme != "https") || base.Hostname() == "" || base.Opaque != "" {
		return nil, errors.New("endpoint must be an absolute http or https URL")
	}
	if base.User != nil || base.RawQuery != "" || base.ForceQuery || strings.Contains(endpoint, "#") {
		return nil, errors.New("endpoint must not contain credentials, a query, or a fragment")
	}
	if base.RawPath != "" || (base.Path != "" && base.Path != "/" && base.Path != "/api" && base.Path != "/api/") {
		return nil, errors.New("endpoint path must be /api or empty")
	}
	if token == "" || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return nil, errors.New("token must be a non-empty dashboard JWT without whitespace")
	}
	base.Path = "/api"
	return &Client{
		endpoint: *base,
		token:    token,
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// URL appends a dashboard path without discarding the endpoint's /api prefix.
// It returns an independent URL that callers may safely modify.
func (c *Client) URL(path string) *url.URL {
	result := c.endpoint
	result.Path += "/" + strings.TrimPrefix(path, "/")
	return &result
}

// Do sends a JSON request and decodes the dashboard's data envelope into output.
// Error responses never expose the token, raw transport errors, or raw bodies.
func (c *Client) Do(ctx context.Context, method, path string, body any, output any) error {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return c.requestError(method, path, 0, "could not encode request JSON")
		}
		requestBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL(path).String(), requestBody)
	if err != nil {
		return c.requestError(method, path, 0, "could not create request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("OpenReplay request canceled: %w", ctx.Err())
		}
		return c.requestError(method, path, 0, "request failed; check the endpoint and network connection")
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return c.requestError(method, path, response.StatusCode, "could not read response")
	}
	if response.StatusCode == http.StatusNoContent && output == nil && len(responseBody) == 0 {
		return nil
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors json.RawMessage `json:"errors"`
		Detail json.RawMessage `json:"detail"`
	}
	decodeErr := json.Unmarshal(responseBody, &envelope)
	message := ""
	var messages []string
	if len(envelope.Errors) > 0 && string(envelope.Errors) != "null" {
		if err := json.Unmarshal(envelope.Errors, &messages); err != nil {
			message = "API returned errors"
		} else if len(messages) > 0 {
			message = strings.Join(messages, "; ")
			if message == "" {
				message = "API returned errors"
			}
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if message == "" {
			_ = json.Unmarshal(envelope.Detail, &message)
		}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			message = "a valid owner/admin dashboard JWT is required"
		}
		return classifyAPIError(c.requestError(method, path, response.StatusCode, message), response.StatusCode, messages)
	}
	if decodeErr != nil {
		return c.requestError(method, path, response.StatusCode, "invalid JSON response")
	}
	if message != "" {
		return classifyAPIError(c.requestError(method, path, response.StatusCode, message), response.StatusCode, messages)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return c.requestError(method, path, response.StatusCode, "response is missing data")
	}
	if output != nil {
		if err := json.Unmarshal(envelope.Data, output); err != nil {
			return c.requestError(method, path, response.StatusCode, "could not decode response data")
		}
	}
	return nil
}

// requestError includes only sanitized context and a bounded server message.
func (c *Client) requestError(method, path string, status int, message string) error {
	detail := fmt.Sprintf("OpenReplay %s %s failed", method, path)
	if status != 0 {
		detail += fmt.Sprintf(" (HTTP %d)", status)
	}
	if message != "" {
		detail += ": " + message
	}
	detail = strings.ReplaceAll(detail, c.token, "[REDACTED]")
	detail = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, detail)
	if len(detail) > 1024 {
		detail = detail[:1024] + "..."
	}
	return errors.New(detail)
}

// classifyAPIError preserves sanitized diagnostics while exposing stable error kinds.
func classifyAPIError(err error, status int, messages []string) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	for _, message := range messages {
		if message == "unauthorized" {
			return fmt.Errorf("%w: %v", ErrUnauthorized, err)
		}
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	for _, message := range messages {
		if message == "project not found" {
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		}
	}
	return err
}
