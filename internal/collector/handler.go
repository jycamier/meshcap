package collector

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/jycamier/meshcap/internal/model"
	"github.com/jycamier/meshcap/pkg/meshcap"
)

var tracer = otel.Tracer("meshcap/collector")

type Handler struct {
	recordCh    chan<- model.HTTPRequest
	maxBodySize int64
	logger      *slog.Logger
	onIngest    func()
	onError     func()
}

func NewHandler(
	recordCh chan<- model.HTTPRequest,
	maxBodySize int,
	logger *slog.Logger,
	onIngest func(),
	onError func(),
) *Handler {
	return &Handler{
		recordCh:    recordCh,
		maxBodySize: int64(maxBodySize),
		logger:      logger,
		onIngest:    onIngest,
		onError:     onError,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := tracer.Start(ctx, "ingest")
	defer span.End()

	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBodySize+1))
	if err != nil {
		h.logger.Warn("failed to read ingest body", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read body")
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if int64(len(body)) > h.maxBodySize {
		span.SetStatus(codes.Error, "payload too large")
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	var cr meshcap.CapturedRequest
	if err := json.Unmarshal(body, &cr); err != nil {
		h.logger.Warn("failed to decode ingest payload", "error", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid JSON")
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	sc := trace.SpanContextFromContext(ctx)
	if sc.HasTraceID() {
		cr.TraceID = sc.TraceID().String()
	}

	req := model.FromCapturedRequest(cr)

	span.SetAttributes(
		attribute.String("request_id", req.RequestID),
		attribute.String("req_host", req.ReqHost),
	)

	select {
	case h.recordCh <- req:
		if h.onIngest != nil {
			h.onIngest()
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		h.logger.Warn("record channel full, dropping request", "request_id", req.RequestID)
		span.SetStatus(codes.Error, "channel full")
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "service overloaded", http.StatusServiceUnavailable)
	}
}
