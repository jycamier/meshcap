package meshcapcaddy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

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
	client *http.Client
}

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
	if m.MaxBodySize <= 0 {
		m.MaxBodySize = 1048576
	}
	m.client = &http.Client{Timeout: 5 * time.Second}
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
		limited := io.LimitReader(r.Body, int64(m.MaxBodySize)+1)
		if _, err := io.Copy(&buf, limited); err == nil {
			body = meshcap.LimitBody(buf.Bytes(), m.MaxBodySize)
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

	cr := meshcap.NewCapturedRequest(meshcap.RequestParams{
		Method:      r.Method,
		Path:        r.URL.RequestURI(),
		Host:        r.Host,
		HTTPVersion: r.Proto,
		Headers:     headers,
		Body:        body,
		ClientIP:    clientIP(r),
		RequestID:   r.Header.Get("X-Request-Id"),
		Traceparent: r.Header.Get("Traceparent"),
	})

	payload, err := meshcap.Marshal(cr)
	if err != nil {
		m.logger.Warn("failed to marshal captured request", zap.Error(err))
	} else {
		go m.dispatch(payload)
	}

	return next.ServeHTTP(w, r)
}

func (m *Middleware) dispatch(payload []byte) {
	req, err := http.NewRequest(http.MethodPost, m.CollectorURL, bytes.NewReader(payload))
	if err != nil {
		m.logger.Warn("failed to create dispatch request", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(payload)))

	resp, err := m.client.Do(req)
	if err != nil {
		m.logger.Warn("failed to dispatch to collector", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		m.logger.Warn("collector returned unexpected status", zap.Int("status", resp.StatusCode))
	}
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

// Interface guards
var (
	_ caddy.Provisioner           = (*Middleware)(nil)
	_ caddy.Validator             = (*Middleware)(nil)
	_ caddyhttp.MiddlewareHandler = (*Middleware)(nil)
	_ caddyfile.Unmarshaler       = (*Middleware)(nil)
)
