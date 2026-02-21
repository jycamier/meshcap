package format

import (
	"encoding/json"
	"testing"

	"github.com/jycamier/meshcap/internal/model"
)

// Private test structs for JSON unmarshaling of HAR documents.
type harDocument struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Version string     `json:"version"`
	Creator harCreator `json:"creator"`
	Entries []harEntry `json:"entries"`
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harEntry struct {
	StartedDateTime string     `json:"startedDateTime"`
	Request         harRequest `json:"request"`
	Response        struct {
		Status     int    `json:"status"`
		StatusText string `json:"statusText"`
	} `json:"response"`
}

type harRequest struct {
	Method      string      `json:"method"`
	URL         string      `json:"url"`
	HTTPVersion string      `json:"httpVersion"`
	Headers     []harHeader `json:"headers"`
	BodySize    int64       `json:"bodySize"`
	PostData    *struct {
		MimeType string `json:"mimeType"`
		Text     string `json:"text"`
	} `json:"postData,omitempty"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func TestHARFormatterValidStructure(t *testing.T) {
	records := []model.HTTPRequest{
		{
			RequestID:   "req-1",
			CapturedAt:  "2024-01-15T10:30:00Z",
			TimestampNs: 1705312200000000000,
			ReqMethod:   "GET",
			ReqPath:     "/api/users",
			ReqHost:     "example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{"Accept":"text/html"}`,
			ReqBody:     nil,
			ReqBodySize: 0,
			ClientIP:    "10.0.0.1",
			SourcePod:   "app-abc123",
		},
		{
			RequestID:   "req-2",
			CapturedAt:  "2024-01-15T10:30:01Z",
			TimestampNs: 1705312201000000000,
			ReqMethod:   "POST",
			ReqPath:     "/api/submit",
			ReqHost:     "example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{"Content-Type":"application/json"}`,
			ReqBody:     []byte(`{"key":"value"}`),
			ReqBodySize: 15,
			ClientIP:    "10.0.0.2",
			SourcePod:   "app-def456",
		},
	}

	f := NewHARFormatter()
	data, err := f.Format(records)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	var doc harDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	// Verify HAR version
	if doc.Log.Version != "1.2" {
		t.Errorf("version = %q, want %q", doc.Log.Version, "1.2")
	}

	// Verify creator
	if doc.Log.Creator.Name != "meshcap" {
		t.Errorf("creator.name = %q, want %q", doc.Log.Creator.Name, "meshcap")
	}
	if doc.Log.Creator.Version != "1.0" {
		t.Errorf("creator.version = %q, want %q", doc.Log.Creator.Version, "1.0")
	}

	// Verify entry count
	if len(doc.Log.Entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(doc.Log.Entries))
	}

	// Verify first entry (GET without body)
	e0 := doc.Log.Entries[0]
	if e0.StartedDateTime != "2024-01-15T10:30:00Z" {
		t.Errorf("entries[0].startedDateTime = %q, want %q", e0.StartedDateTime, "2024-01-15T10:30:00Z")
	}
	if e0.Request.Method != "GET" {
		t.Errorf("entries[0].request.method = %q, want %q", e0.Request.Method, "GET")
	}
	if e0.Request.URL != "http://example.com/api/users" {
		t.Errorf("entries[0].request.url = %q, want %q", e0.Request.URL, "http://example.com/api/users")
	}
	if e0.Request.HTTPVersion != "HTTP/1.1" {
		t.Errorf("entries[0].request.httpVersion = %q, want %q", e0.Request.HTTPVersion, "HTTP/1.1")
	}
	if e0.Request.BodySize != 0 {
		t.Errorf("entries[0].request.bodySize = %d, want 0", e0.Request.BodySize)
	}
	if e0.Request.PostData != nil {
		t.Errorf("entries[0].request.postData should be nil for GET without body")
	}

	// Verify second entry (POST with body)
	e1 := doc.Log.Entries[1]
	if e1.Request.Method != "POST" {
		t.Errorf("entries[1].request.method = %q, want %q", e1.Request.Method, "POST")
	}
	if e1.Request.URL != "http://example.com/api/submit" {
		t.Errorf("entries[1].request.url = %q, want %q", e1.Request.URL, "http://example.com/api/submit")
	}
	if e1.Request.BodySize != 15 {
		t.Errorf("entries[1].request.bodySize = %d, want 15", e1.Request.BodySize)
	}
	if e1.Request.PostData == nil {
		t.Fatal("entries[1].request.postData should not be nil for POST with body")
	}
	if e1.Request.PostData.MimeType != "application/json" {
		t.Errorf("entries[1].request.postData.mimeType = %q, want %q", e1.Request.PostData.MimeType, "application/json")
	}
	if e1.Request.PostData.Text != `{"key":"value"}` {
		t.Errorf("entries[1].request.postData.text = %q, want %q", e1.Request.PostData.Text, `{"key":"value"}`)
	}

	// Verify response stub
	if e0.Response.Status != 0 {
		t.Errorf("entries[0].response.status = %d, want 0", e0.Response.Status)
	}
	if e0.Response.StatusText != "" {
		t.Errorf("entries[0].response.statusText = %q, want empty", e0.Response.StatusText)
	}
}

func TestHARFormatterEmpty(t *testing.T) {
	f := NewHARFormatter()
	_, err := f.Format(nil)
	if err == nil {
		t.Fatal("expected error for nil records")
	}
}

func TestHARFormatterExtension(t *testing.T) {
	f := NewHARFormatter()
	if ext := f.Extension(); ext != ".har" {
		t.Errorf("Extension() = %q, want %q", ext, ".har")
	}
}

func TestHARFormatterHeadersParsing(t *testing.T) {
	records := []model.HTTPRequest{
		{
			RequestID:   "req-1",
			CapturedAt:  "2024-01-15T10:30:00Z",
			TimestampNs: 1705312200000000000,
			ReqMethod:   "GET",
			ReqPath:     "/api/test",
			ReqHost:     "example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{"X-Custom":"foo","Authorization":"Bearer token"}`,
			ReqBody:     nil,
			ReqBodySize: 0,
			ClientIP:    "10.0.0.1",
			SourcePod:   "app-abc123",
		},
	}

	f := NewHARFormatter()
	data, err := f.Format(records)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	var doc harDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	headers := doc.Log.Entries[0].Request.Headers
	if len(headers) != 2 {
		t.Fatalf("header count = %d, want 2", len(headers))
	}

	// Headers should be sorted by name
	if headers[0].Name != "Authorization" || headers[0].Value != "Bearer token" {
		t.Errorf("headers[0] = {%q, %q}, want {%q, %q}", headers[0].Name, headers[0].Value, "Authorization", "Bearer token")
	}
	if headers[1].Name != "X-Custom" || headers[1].Value != "foo" {
		t.Errorf("headers[1] = {%q, %q}, want {%q, %q}", headers[1].Name, headers[1].Value, "X-Custom", "foo")
	}
}

func TestHARFormatterInvalidHeaders(t *testing.T) {
	records := []model.HTTPRequest{
		{
			RequestID:   "req-1",
			CapturedAt:  "2024-01-15T10:30:00Z",
			TimestampNs: 1705312200000000000,
			ReqMethod:   "GET",
			ReqPath:     "/api/test",
			ReqHost:     "example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{not valid json`,
			ReqBody:     nil,
			ReqBodySize: 0,
			ClientIP:    "10.0.0.1",
			SourcePod:   "app-abc123",
		},
	}

	f := NewHARFormatter()
	data, err := f.Format(records)
	if err != nil {
		t.Fatalf("Format should not error on invalid headers JSON, got: %v", err)
	}

	var doc harDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	headers := doc.Log.Entries[0].Request.Headers
	if len(headers) != 0 {
		t.Errorf("header count = %d, want 0 for invalid headers JSON", len(headers))
	}
}
