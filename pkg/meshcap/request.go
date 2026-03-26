package meshcap

import (
	"encoding/json"
	"time"
)

// CapturedRequest is the canonical wire format for captured HTTP traffic.
// JSON tags match the collector's expected payload exactly.
type CapturedRequest struct {
	RequestID   string `json:"request_id"`
	TraceID     string `json:"trace_id"`
	CapturedAt  string `json:"captured_at"`
	TimestampNs int64  `json:"timestamp_ns"`
	ReqMethod   string `json:"req_method"`
	ReqPath     string `json:"req_path"`
	ReqHost     string `json:"req_host"`
	ReqVersion  string `json:"req_http_version"`
	ReqHeaders  string `json:"req_headers"`
	ReqBody     []byte `json:"req_body,omitempty"`
	ReqBodySize int64  `json:"req_body_size"`
	ClientIP    string `json:"client_ip"`
	SourcePod   string `json:"source_pod"`
}

// RequestParams holds the raw metadata extracted from an HTTP request
// by a capture plugin (WASM, Caddy, etc.).
type RequestParams struct {
	Method      string
	Path        string
	Host        string
	HTTPVersion string
	Headers     map[string]string
	Body        []byte
	ClientIP    string
	SourcePod   string
	RequestID   string
	Traceparent string
}

// NewCapturedRequest builds a CapturedRequest from raw parameters.
// It populates CapturedAt/TimestampNs automatically and extracts TraceID from the traceparent header.
func NewCapturedRequest(p RequestParams) CapturedRequest {
	now := time.Now().UTC()
	return CapturedRequest{
		RequestID:   p.RequestID,
		TraceID:     ExtractTraceID(p.Traceparent),
		CapturedAt:  now.Format(time.RFC3339Nano),
		TimestampNs: now.UnixNano(),
		ReqMethod:   p.Method,
		ReqPath:     p.Path,
		ReqHost:     p.Host,
		ReqVersion:  p.HTTPVersion,
		ReqHeaders:  MarshalHeaders(p.Headers),
		ReqBody:     p.Body,
		ReqBodySize: int64(len(p.Body)),
		ClientIP:    p.ClientIP,
		SourcePod:   p.SourcePod,
	}
}

// Marshal serializes a CapturedRequest to JSON.
func Marshal(r CapturedRequest) ([]byte, error) {
	return json.Marshal(r)
}
