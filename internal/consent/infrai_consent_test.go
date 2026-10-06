package consent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGrantRequestAndRateLimitRetry(t *testing.T) {
	calls := 0
	var body map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/auth/consent/grant/user-42" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"code":"RATE_LIMITED","message":"retry later"},"metadata":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"data":{},"error":null,"metadata":{}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, "test-key", server.Client())
	var delays []time.Duration
	client.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	err := client.Grant(context.Background(), "user-42", "developer_tools", "settings", "evt-4:developer_tools:grant")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(delays) != 1 || delays[0] != 2*time.Second {
		t.Fatalf("calls=%d delays=%v", calls, delays)
	}
	if body["user_id"] != "user-42" || body["category"] != "developer_tools" || body["source"] != "settings" || body["idempotency_key"] == "" {
		t.Fatalf("body = %#v", body)
	}
}

func TestEnvelopeErrorPrecedesHTTPStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"data":null,"error":{"code":"CONSENT_INPUT_REJECTED","message":"category is required"},"metadata":{}}`))
	}))
	defer server.Close()

	client := newClient(server.URL, "test-key", server.Client())
	err := client.Revoke(context.Background(), "user-42", "developer_tools", "evt-5:developer_tools:revoke")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "CONSENT_INPUT_REJECTED" || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("error = %#v", err)
	}
}
