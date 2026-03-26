package meshcapcaddy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/jycamier/meshcap/pkg/meshcap"
	"go.uber.org/zap"
)

func TestServeHTTP(t *testing.T) {
	// Set up a fake collector that records received payloads.
	received := make(chan []byte, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusAccepted)
	}))
	defer collector.Close()

	m := &Middleware{
		CollectorURL: collector.URL,
		MaxBodySize:  1048576,
		logger:       zap.NewNop(),
		client:       &http.Client{Timeout: 5 * time.Second},
	}

	// Downstream handler that verifies the body is still readable.
	nextCalled := false
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		nextCalled = true
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("next handler failed to read body: %v", err)
		}
		if !strings.Contains(string(body), "test payload") {
			t.Errorf("next handler got body %q, want containing 'test payload'", body)
		}
		w.WriteHeader(http.StatusOK)
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/test", strings.NewReader(`{"data":"test payload"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("Traceparent", "00-aabb-ccdd-01")
	rec := httptest.NewRecorder()

	err := m.ServeHTTP(rec, req, next)
	if err != nil {
		t.Fatalf("ServeHTTP() error = %v", err)
	}
	if !nextCalled {
		t.Error("next handler was not called")
	}

	// Wait for the async dispatch.
	select {
	case payload := <-received:
		var cr meshcap.CapturedRequest
		if err := json.Unmarshal(payload, &cr); err != nil {
			t.Fatalf("failed to unmarshal collector payload: %v", err)
		}
		if cr.ReqMethod != "POST" {
			t.Errorf("ReqMethod = %q, want POST", cr.ReqMethod)
		}
		if cr.ReqPath != "/api/test" {
			t.Errorf("ReqPath = %q, want /api/test", cr.ReqPath)
		}
		if cr.RequestID != "req-123" {
			t.Errorf("RequestID = %q, want req-123", cr.RequestID)
		}
		if cr.TraceID != "aabb" {
			t.Errorf("TraceID = %q, want aabb", cr.TraceID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for collector to receive payload")
	}
}

func TestServeHTTPNoBody(t *testing.T) {
	received := make(chan []byte, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusAccepted)
	}))
	defer collector.Close()

	m := &Middleware{
		CollectorURL: collector.URL,
		MaxBodySize:  1048576,
		logger:       zap.NewNop(),
		client:       &http.Client{Timeout: 5 * time.Second},
	}

	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	err := m.ServeHTTP(rec, req, next)
	if err != nil {
		t.Fatalf("ServeHTTP() error = %v", err)
	}

	select {
	case payload := <-received:
		var cr meshcap.CapturedRequest
		if err := json.Unmarshal(payload, &cr); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if cr.ReqMethod != "GET" {
			t.Errorf("ReqMethod = %q, want GET", cr.ReqMethod)
		}
		if cr.ReqBodySize != 0 {
			t.Errorf("ReqBodySize = %d, want 0", cr.ReqBodySize)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for collector payload")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name string
		xff  string
		addr string
		want string
	}{
		{
			name: "xff header present",
			xff:  "1.2.3.4",
			addr: "10.0.0.1:5678",
			want: "1.2.3.4",
		},
		{
			name: "no xff, use remote addr",
			xff:  "",
			addr: "10.0.0.1:5678",
			want: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.addr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			got := clientIP(r)
			if got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
