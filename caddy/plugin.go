package meshcapcaddy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/jycamier/meshcap/pkg/meshcap"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(Middleware{})
	httpcaddyfile.RegisterHandlerDirective("meshcap", parseCaddyfile)
}

// Middleware captures HTTP request metadata and forwards it to a meshcap collector.
type Middleware struct {
	// CollectorURL is the full URL of the meshcap collector ingest endpoint (e.g. http://meshcap-collector:8080/ingest).
	CollectorURL string `json:"collector_url"`

	// MaxBodySize is the maximum request body size to capture in bytes. Default: 1048576 (1MB).
	MaxBodySize int `json:"max_body_size,omitempty"`

	logger *zap.Logger
	engine *meshcap.Engine
}

var (
	_ caddy.Provisioner           = (*Middleware)(nil)
	_ caddy.Validator             = (*Middleware)(nil)
	_ caddyhttp.MiddlewareHandler = (*Middleware)(nil)
	_ caddyfile.Unmarshaler       = (*Middleware)(nil)
)

// CaddyModule returns the Caddy module information.
func (Middleware) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.meshcap",
		New: func() caddy.Module { return new(Middleware) },
	}
}

// Provision implements caddy.Provisioner.
func (m *Middleware) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger()
	m.engine = meshcap.NewEngine(meshcap.EngineConfig{
		MaxBodySize:  m.MaxBodySize,
		CollectorURL: m.CollectorURL,
		OnError: func(err error) {
			m.logger.Warn("capture failed", zap.Error(err))
		},
	})
	return nil
}

// Validate implements caddy.Validator.
func (m *Middleware) Validate() error {
	if m.CollectorURL == "" {
		return fmt.Errorf("collector_url is required")
	}
	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Read and buffer the body without consuming it.
	var body []byte
	if r.Body != nil {
		var buf bytes.Buffer
		limited := io.LimitReader(r.Body, int64(m.engine.MaxBodySize())+1)
		if _, err := io.Copy(&buf, limited); err == nil {
			body = buf.Bytes()
		}
		// Restore the body so downstream handlers can read it.
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf.Bytes()), r.Body))
	}

	// Extract headers as flat map.
	headers := make(map[string]string, len(r.Header))
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	go m.engine.Capture(meshcap.RequestParams{
		Method:      r.Method,
		Path:        r.URL.RequestURI(),
		Host:        r.Host,
		HTTPVersion: r.Proto,
		Headers:     headers,
		Body:        body,
		ClientIP:    clientIP(r),
		RequestID:   r.Header.Get("X-Request-Id"),
		Traceparent: r.Header.Get("Traceparent"),
		Tracestate:  r.Header.Get("Tracestate"),
	})

	return next.ServeHTTP(w, r)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return meshcap.StripPort(r.RemoteAddr)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (m *Middleware) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	d.Next() // consume directive name

	for d.NextBlock(0) {
		switch d.Val() {
		case "collector_url":
			if !d.NextArg() {
				return d.ArgErr()
			}
			m.CollectorURL = d.Val()
		case "max_body_size":
			if !d.NextArg() {
				return d.ArgErr()
			}
			size, err := strconv.Atoi(d.Val())
			if err != nil {
				return d.Errf("invalid max_body_size: %v", err)
			}
			m.MaxBodySize = size
		default:
			return d.Errf("unrecognized option: %s", d.Val())
		}
	}
	return nil
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m Middleware
	err := m.UnmarshalCaddyfile(h.Dispenser)
	return &m, err
}
