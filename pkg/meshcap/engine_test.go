package meshcap

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestEngineCapture(t *testing.T) {
	var received DispatchRequest
	engine := NewEngine(EngineConfig{
		MaxBodySize: 1048576,
		Dispatch: func(req DispatchRequest) error {
			received = req
			return nil
		},
	})

	engine.Capture(RequestParams{
		Method:      "POST",
		Path:        "/api/users",
		Host:        "example.com",
		HTTPVersion: "HTTP/1.1",
		Headers:     map[string]string{"content-type": "application/json"},
		Body:        []byte(`{"name":"test"}`),
		ClientIP:    "10.0.0.1",
		RequestID:   "req-001",
		Traceparent: "00-aabb-ccdd-01",
		Tracestate:  "vendor=value",
	})

	if received.Method != "POST" {
		t.Errorf("Method = %q, want POST", received.Method)
	}
	if received.Path != "/ingest" {
		t.Errorf("Path = %q, want /ingest", received.Path)
	}
	if received.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header missing")
	}
	if received.Headers["Traceparent"] != "00-aabb-ccdd-01" {
		t.Errorf("Traceparent = %q, want 00-aabb-ccdd-01", received.Headers["Traceparent"])
	}
	if received.Headers["Tracestate"] != "vendor=value" {
		t.Errorf("Tracestate = %q, want vendor=value", received.Headers["Tracestate"])
	}

	var cr CapturedRequest
	if err := json.Unmarshal(received.Payload, &cr); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if cr.ReqMethod != "POST" {
		t.Errorf("ReqMethod = %q, want POST", cr.ReqMethod)
	}
	if cr.ReqHost != "example.com" {
		t.Errorf("ReqHost = %q, want example.com", cr.ReqHost)
	}
}

func TestEngineCaptureBodyLimit(t *testing.T) {
	var received DispatchRequest
	engine := NewEngine(EngineConfig{
		MaxBodySize: 5,
		Dispatch: func(req DispatchRequest) error {
			received = req
			return nil
		},
	})

	engine.Capture(RequestParams{
		Method: "POST",
		Body:   []byte("hello world"),
	})

	var cr CapturedRequest
	if err := json.Unmarshal(received.Payload, &cr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cr.ReqBodySize != 5 {
		t.Errorf("ReqBodySize = %d, want 5", cr.ReqBodySize)
	}
}

func TestEngineDispatchError(t *testing.T) {
	var capturedErr error
	engine := NewEngine(EngineConfig{
		Dispatch: func(req DispatchRequest) error {
			return errors.New("connection refused")
		},
		OnError: func(err error) {
			capturedErr = err
		},
	})

	engine.Capture(RequestParams{Method: "GET", Path: "/"})

	if capturedErr == nil {
		t.Fatal("expected OnError to be called")
	}
	if capturedErr.Error() != "connection refused" {
		t.Errorf("error = %q, want 'connection refused'", capturedErr.Error())
	}
}

func TestEngineDefaults(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Dispatch: func(req DispatchRequest) error { return nil },
	})

	if engine.MaxBodySize() != 1048576 {
		t.Errorf("MaxBodySize = %d, want 1048576", engine.MaxBodySize())
	}
	if engine.Timeout() != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", engine.Timeout())
	}
}

func TestEngineCustomPath(t *testing.T) {
	var received DispatchRequest
	engine := NewEngine(EngineConfig{
		Path: "/custom/ingest",
		Dispatch: func(req DispatchRequest) error {
			received = req
			return nil
		},
	})

	engine.Capture(RequestParams{Method: "GET"})

	if received.Path != "/custom/ingest" {
		t.Errorf("Path = %q, want /custom/ingest", received.Path)
	}
}

func TestEngineNoTraceHeaders(t *testing.T) {
	var received DispatchRequest
	engine := NewEngine(EngineConfig{
		Dispatch: func(req DispatchRequest) error {
			received = req
			return nil
		},
	})

	engine.Capture(RequestParams{Method: "GET"})

	if _, ok := received.Headers["Traceparent"]; ok {
		t.Error("Traceparent should not be set when empty")
	}
	if _, ok := received.Headers["Tracestate"]; ok {
		t.Error("Tracestate should not be set when empty")
	}
}
