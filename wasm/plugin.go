package main

import (
	"encoding/json"
	"strconv"

	"github.com/jycamier/meshcap/pkg/meshcap"
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
	httpVersion string
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

	// Read HTTP protocol version from Envoy (e.g. "HTTP/1.1", "HTTP/2").
	ctx.httpVersion = "HTTP/1.1"
	if proto, err := proxywasm.GetProperty([]string{"request", "protocol"}); err == nil && len(proto) > 0 {
		ctx.httpVersion = string(proto)
	}

	// Fallback: client IP from Envoy connection source address.
	if ctx.clientIP == "" {
		if addr, err := proxywasm.GetProperty([]string{"source", "address"}); err == nil && len(addr) > 0 {
			ctx.clientIP = meshcap.StripPort(string(addr))
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
		ctx.bodyBuf = meshcap.AppendBodyChunk(ctx.bodyBuf, chunk, ctx.config.MaxBodySize)
	}

	if endOfStream {
		ctx.dispatchToCollector(ctx.bodyBuf)
	}

	return types.ActionContinue
}

func (ctx *httpContext) dispatchToCollector(body []byte) {
	req := meshcap.NewCapturedRequest(meshcap.RequestParams{
		Method:      ctx.method,
		Path:        ctx.path,
		Host:        ctx.host,
		HTTPVersion: ctx.httpVersion,
		Headers:     ctx.headers,
		Body:        body,
		ClientIP:    ctx.clientIP,
		SourcePod:   ctx.sourcePod,
		RequestID:   ctx.requestID,
		Traceparent: ctx.traceparent,
	})

	payload, err := meshcap.Marshal(req)
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
