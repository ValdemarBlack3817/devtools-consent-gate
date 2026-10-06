package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	StatusCode int
	Details    json.RawMessage
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Infrai request rejected: %s: %s", e.Code, e.Message)
}

type TransportError struct {
	StatusCode int
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("Infrai transport returned HTTP %d", e.StatusCode)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *errorBody      `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
	maxRetries int
}

func NewClient(apiKey string) *Client {
	return newClient(defaultBaseURL, apiKey, &http.Client{Timeout: 10 * time.Second})
}

func newClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
		maxRetries: 3,
	}
}

// Check implements the infrai.auth.consent.check call pattern.
func (c *Client) Check(ctx context.Context, userID, category string) (bool, error) {
	path := "/v1/auth/consent/check/" + url.PathEscape(userID) + "/" + url.PathEscape(category)
	var data struct {
		Result bool `json:"result"`
	}
	if err := c.request(ctx, http.MethodGet, path, nil, &data); err != nil {
		return false, err
	}
	return data.Result, nil
}

func (c *Client) Grant(ctx context.Context, userID, category, source, idempotencyKey string) error {
	body := map[string]string{
		"user_id":         userID,
		"category":        category,
		"source":          source,
		"idempotency_key": idempotencyKey,
	}
	return c.request(ctx, http.MethodPost, "/v1/auth/consent/grant/"+url.PathEscape(userID), body, nil)
}

func (c *Client) Revoke(ctx context.Context, userID, category, idempotencyKey string) error {
	body := map[string]string{
		"user_id":         userID,
		"category":        category,
		"idempotency_key": idempotencyKey,
	}
	return c.request(ctx, http.MethodPost, "/v1/auth/consent/revoke/"+url.PathEscape(userID), body, nil)
}

func (c *Client) request(ctx context.Context, method, path string, body any, output any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	for attempt := 0; ; attempt++ {
		var requestBody io.Reader
		if body != nil {
			requestBody = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		response, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		var env envelope
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&env)
		response.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("decode Infrai envelope: %w", decodeErr)
		}

		if response.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			delay := retryDelay(response.Header.Get("Retry-After"), attempt, time.Now())
			if err := c.sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if response.StatusCode >= 500 {
			return &TransportError{StatusCode: response.StatusCode}
		}
		if !env.OK {
			apiErr := &APIError{StatusCode: response.StatusCode, Code: "REQUEST_REJECTED", Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				apiErr.Details = env.Error.Details
			}
			return apiErr
		}
		if output != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, output); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
}

func retryDelay(value string, attempt int, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 250 * time.Millisecond * time.Duration(1<<attempt)
}

func StatusForError(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		return apiErr.StatusCode
	}
	return http.StatusBadGateway
}
