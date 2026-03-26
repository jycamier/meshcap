package meshcap

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPDispatcher(t *testing.T) {
	var receivedBody []byte
	var receivedHeaders http.Header
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusAccepted)
	}))
	defer collector.Close()

	dispatch := newHTTPDispatcher(collector.URL, 5*time.Second)

	err := dispatch(DispatchRequest{
		Method:  http.MethodPost,
		Path:    "/ingest",
		Payload: []byte(`{"test":true}`),
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Traceparent":  "00-abc-def-01",
		},
	})
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}

	if string(receivedBody) != `{"test":true}` {
		t.Errorf("body = %q, want {\"test\":true}", receivedBody)
	}
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("Traceparent") != "00-abc-def-01" {
		t.Errorf("Traceparent = %q, want 00-abc-def-01", receivedHeaders.Get("Traceparent"))
	}
}

func TestHTTPDispatcherNon202(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer collector.Close()

	dispatch := newHTTPDispatcher(collector.URL, 5*time.Second)
	err := dispatch(DispatchRequest{
		Method:  http.MethodPost,
		Payload: []byte(`{}`),
		Headers: map[string]string{},
	})

	if err == nil {
		t.Fatal("expected error for non-202 status")
	}
}

func TestHTTPDispatcherConnectionError(t *testing.T) {
	dispatch := newHTTPDispatcher("http://localhost:1", 1*time.Second)
	err := dispatch(DispatchRequest{
		Method:  http.MethodPost,
		Payload: []byte(`{}`),
		Headers: map[string]string{},
	})

	if err == nil {
		t.Fatal("expected error for connection failure")
	}
}

func TestNewEngineWithCollectorURL(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer collector.Close()

	var capturedErr error
	engine := NewEngine(EngineConfig{
		CollectorURL: collector.URL,
		OnError: func(err error) {
			capturedErr = err
		},
	})

	engine.Capture(RequestParams{Method: "GET", Path: "/test"})

	if capturedErr != nil {
		t.Errorf("unexpected error: %v", capturedErr)
	}
}
