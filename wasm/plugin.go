package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/proxy-wasm/proxy-wasm-go-sdk/proxywasm"
	"github.com/proxy-wasm/proxy-wasm-go-sdk/proxywasm/types"
)

var version = "dev"

// pluginConfig is deserialized from the WasmPlugin CRD pluginConfig field.
type pluginConfig struct {
	CollectorCluster string `json:"collectorCluster"`
	MaxBodySize      int    `json:"maxBodySize"`
}

// pluginContext holds plugin-level config, shared across all HTTP streams.
type pluginContext struct {
	types.DefaultPluginContext
	config pluginConfig
}

func (p *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	if pluginConfigurationSize == 0 {
		proxywasm.LogWarn("no plugin configuration provided, using defaults")
		p.config = pluginConfig{
			CollectorCluster: "outbound|8080||collector.capture.svc.cluster.local",
			MaxBodySize:      1048576,
		}
		return types.OnPluginStartStatusOK
	}

	data, err := proxywasm.GetPluginConfiguration()
	if err != nil {
		proxywasm.LogCriticalf("failed to get plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	if err := json.Unmarshal(data, &p.config); err != nil {
		proxywasm.LogCriticalf("failed to parse plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	if p.config.MaxBodySize <= 0 {
		p.config.MaxBodySize = 1048576
	}

	proxywasm.LogInfof("capture plugin started: version=%s cluster=%s maxBodySize=%d",
		version, p.config.CollectorCluster, p.config.MaxBodySize)
	return types.OnPluginStartStatusOK
}

func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpContext{
		contextID: contextID,
		config:    &p.config,
	}
}

// capturedRequest is the JSON payload sent to the collector.
type capturedRequest struct {
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

// httpContext handles a single HTTP request stream.
type httpContext struct {
	types.DefaultHttpContext
	contextID uint32
	config    *pluginConfig

	method      string
	path        string
	host        string
	headers     map[string]string
	clientIP    string
	sourcePod   string
	requestID   string
	traceparent string
	tracestate  string
	bodyBuf     []byte
}

func (ctx *httpContext) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		proxywasm.LogWarnf("failed to get request headers: %v", err)
		return types.ActionContinue
	}

	ctx.headers = make(map[string]string, len(headers))
	for _, h := range headers {
		key, val := h[0], h[1]
		ctx.headers[key] = val

		switch key {
		case ":method":
			ctx.method = val
		case ":path":
			ctx.path = val
		case ":authority":
			ctx.host = val
		case "x-forwarded-for":
			ctx.clientIP = val
		case "x-request-id":
			ctx.requestID = val
		case "traceparent":
			ctx.traceparent = val
		case "tracestate":
			ctx.tracestate = val
		}
	}

	// Try to get source pod from Envoy metadata (try multiple keys for Istio compat).
	for _, key := range []string{"WORKLOAD_NAME", "POD_NAME", "NAME"} {
		if pod, err := proxywasm.GetProperty([]string{"node", "metadata", key}); err == nil && len(pod) > 0 {
			ctx.sourcePod = string(pod)
			break
		}
	}

	// Fallback: client IP from Envoy connection source address.
	if ctx.clientIP == "" {
		if addr, err := proxywasm.GetProperty([]string{"source", "address"}); err == nil && len(addr) > 0 {
			ctx.clientIP = stripPort(string(addr))
		}
	}

	if endOfStream {
		ctx.dispatchToCollector(nil)
	}

	return types.ActionContinue
}

func (ctx *httpContext) OnHttpRequestBody(bodySize int, endOfStream bool) types.Action {
	chunk, err := proxywasm.GetHttpRequestBody(0, bodySize)
	if err != nil {
		proxywasm.LogWarnf("failed to get request body chunk: %v", err)
	} else if len(chunk) > 0 {
		remaining := ctx.config.MaxBodySize - len(ctx.bodyBuf)
		if remaining > 0 {
			if len(chunk) > remaining {
				chunk = chunk[:remaining]
			}
			ctx.bodyBuf = append(ctx.bodyBuf, chunk...)
		}
	}

	if endOfStream {
		ctx.dispatchToCollector(ctx.bodyBuf)
	}

	return types.ActionContinue
}

// stripPort removes the port suffix from an address like "10.0.0.1:1234".
func stripPort(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

func (ctx *httpContext) dispatchToCollector(body []byte) {
	now := time.Now().UTC()

	// Extract trace_id from traceparent (format: version-trace_id-parent_id-trace_flags).
	var traceID string
	if parts := strings.SplitN(ctx.traceparent, "-", 4); len(parts) >= 2 {
		traceID = parts[1]
	}

	req := capturedRequest{
		RequestID:   ctx.requestID,
		TraceID:     traceID,
		CapturedAt:  now.Format(time.RFC3339Nano),
		TimestampNs: now.UnixNano(),
		ReqMethod:   ctx.method,
		ReqPath:     ctx.path,
		ReqHost:     ctx.host,
		ReqVersion:  "HTTP/1.1",
		ReqHeaders:  marshalHeaders(ctx.headers),
		ReqBody:     body,
		ReqBodySize: int64(len(body)),
		ClientIP:    ctx.clientIP,
		SourcePod:   ctx.sourcePod,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		proxywasm.LogErrorf("failed to marshal captured request: %v", err)
		return
	}

	dispatchHeaders := [][2]string{
		{":method", "POST"},
		{":path", "/ingest"},
		{":authority", "collector"},
		{"content-type", "application/json"},
		{"content-length", strconv.Itoa(len(payload))},
	}
	if ctx.traceparent != "" {
		dispatchHeaders = append(dispatchHeaders, [2]string{"traceparent", ctx.traceparent})
	}
	if ctx.tracestate != "" {
		dispatchHeaders = append(dispatchHeaders, [2]string{"tracestate", ctx.tracestate})
	}

	_, err = proxywasm.DispatchHttpCall(
		ctx.config.CollectorCluster,
		dispatchHeaders,
		payload,
		nil,
		5000,
		func(numHeaders, bodySize, numTrailers int) {
			status, err := proxywasm.GetHttpCallResponseHeaders()
			if err != nil {
				proxywasm.LogWarnf("failed to get dispatch response headers: %v", err)
				return
			}
			for _, h := range status {
				if h[0] == ":status" && h[1] != "202" {
					proxywasm.LogWarnf("collector returned status %s for request %s", h[1], ctx.requestID)
				}
			}
		},
	)
	if err != nil {
		proxywasm.LogErrorf("failed to dispatch to collector: %v", err)
	}
}

func marshalHeaders(h map[string]string) string {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Sprintf("{\"error\": %q}", err.Error())
	}
	return string(data)
}

