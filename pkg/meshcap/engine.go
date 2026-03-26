package meshcap

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// DispatchRequest contains everything a dispatcher needs to send a captured request.
// The engine prepares Method, Path and Headers — the dispatcher converts them to its runtime format.
type DispatchRequest struct {
	Method  string
	Path    string
	Payload []byte
	Headers map[string]string
}

// DispatchFunc sends a dispatch request to the collector.
type DispatchFunc func(req DispatchRequest) error

// OnErrorFunc is called when capture or dispatch fails.
type OnErrorFunc func(err error)

// EngineConfig configures the capture engine.
type EngineConfig struct {
	MaxBodySize  int
	Path         string
	Timeout      time.Duration
	CollectorURL string
	Dispatch     DispatchFunc
	OnError      OnErrorFunc
}

// Engine encapsulates the full capture pipeline: body limiting, serialization, and dispatch.
type Engine struct {
	maxBodySize int
	path        string
	timeout     time.Duration
	dispatch    DispatchFunc
	onError     OnErrorFunc
}

// NewEngine creates a capture engine from the given configuration.
// If Dispatch is nil and CollectorURL is set, a built-in HTTP dispatcher is used.
func NewEngine(cfg EngineConfig) *Engine {
	maxBodySize := cfg.MaxBodySize
	if maxBodySize <= 0 {
		maxBodySize = 1048576
	}
	path := cfg.Path
	if path == "" {
		path = "/ingest"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	dispatch := cfg.Dispatch
	if dispatch == nil && cfg.CollectorURL != "" {
		dispatch = newHTTPDispatcher(cfg.CollectorURL, timeout)
	}
	return &Engine{
		maxBodySize: maxBodySize,
		path:        path,
		timeout:     timeout,
		dispatch:    dispatch,
		onError:     cfg.OnError,
	}
}

// Capture processes request parameters and dispatches them to the collector.
// Errors are reported via the OnError callback — the caller does not need to handle them.
func (e *Engine) Capture(p RequestParams) {
	p.Body = LimitBody(p.Body, e.maxBodySize)
	req := NewCapturedRequest(p)
	payload, err := Marshal(req)
	if err != nil {
		e.reportError(fmt.Errorf("marshal: %w", err))
		return
	}
	headers := map[string]string{
		"Content-Type":   "application/json",
		"Content-Length": strconv.Itoa(len(payload)),
	}
	if p.Traceparent != "" {
		headers["Traceparent"] = p.Traceparent
	}
	if p.Tracestate != "" {
		headers["Tracestate"] = p.Tracestate
	}
	if err := e.dispatch(DispatchRequest{
		Method:  http.MethodPost,
		Path:    e.path,
		Payload: payload,
		Headers: headers,
	}); err != nil {
		e.reportError(err)
	}
}

func (e *Engine) reportError(err error) {
	if e.onError != nil {
		e.onError(err)
	}
}

// MaxBodySize returns the configured maximum body size.
func (e *Engine) MaxBodySize() int {
	return e.maxBodySize
}

// Timeout returns the configured dispatch timeout.
func (e *Engine) Timeout() time.Duration {
	return e.timeout
}
