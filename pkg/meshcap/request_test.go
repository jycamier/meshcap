package meshcap

import (
	"encoding/json"
	"testing"
)

func TestNewCapturedRequest(t *testing.T) {
	params := RequestParams{
		Method:      "POST",
		Path:        "/api/v1/users",
		Host:        "example.com",
		HTTPVersion: "HTTP/1.1",
		Headers:     map[string]string{"content-type": "application/json"},
		Body:        []byte(`{"name":"test"}`),
		ClientIP:    "10.0.0.1",
		SourcePod:   "frontend-abc123",
		RequestID:   "req-001",
		Traceparent: "00-abcdef1234567890abcdef1234567890-1234567890abcdef-01",
	}

	req := NewCapturedRequest(params)

	if req.ReqMethod != "POST" {
		t.Errorf("ReqMethod = %q, want %q", req.ReqMethod, "POST")
	}
	if req.ReqPath != "/api/v1/users" {
		t.Errorf("ReqPath = %q, want %q", req.ReqPath, "/api/v1/users")
	}
	if req.ReqHost != "example.com" {
		t.Errorf("ReqHost = %q, want %q", req.ReqHost, "example.com")
	}
	if req.ReqVersion != "HTTP/1.1" {
		t.Errorf("ReqVersion = %q, want %q", req.ReqVersion, "HTTP/1.1")
	}
	if req.TraceID != "abcdef1234567890abcdef1234567890" {
		t.Errorf("TraceID = %q, want %q", req.TraceID, "abcdef1234567890abcdef1234567890")
	}
	if req.RequestID != "req-001" {
		t.Errorf("RequestID = %q, want %q", req.RequestID, "req-001")
	}
	if req.ReqBodySize != int64(len(params.Body)) {
		t.Errorf("ReqBodySize = %d, want %d", req.ReqBodySize, len(params.Body))
	}
	if req.ClientIP != "10.0.0.1" {
		t.Errorf("ClientIP = %q, want %q", req.ClientIP, "10.0.0.1")
	}
	if req.SourcePod != "frontend-abc123" {
		t.Errorf("SourcePod = %q, want %q", req.SourcePod, "frontend-abc123")
	}
	if req.CapturedAt == "" {
		t.Error("CapturedAt should not be empty")
	}
	if req.TimestampNs == 0 {
		t.Error("TimestampNs should not be zero")
	}
}

func TestNewCapturedRequestNoBody(t *testing.T) {
	req := NewCapturedRequest(RequestParams{
		Method: "GET",
		Path:   "/health",
		Host:   "svc.local",
	})

	if req.ReqBodySize != 0 {
		t.Errorf("ReqBodySize = %d, want 0", req.ReqBodySize)
	}
	if req.ReqBody != nil {
		t.Errorf("ReqBody should be nil for empty body")
	}
}

func TestMarshal(t *testing.T) {
	req := CapturedRequest{
		RequestID: "r1",
		ReqMethod: "GET",
		ReqPath:   "/test",
		ReqHost:   "example.com",
	}

	data, err := Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded CapturedRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.RequestID != "r1" {
		t.Errorf("decoded.RequestID = %q, want %q", decoded.RequestID, "r1")
	}
	if decoded.ReqMethod != "GET" {
		t.Errorf("decoded.ReqMethod = %q, want %q", decoded.ReqMethod, "GET")
	}
}
