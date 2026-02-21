package collector

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/jycamier/meshcap/internal/model"
)

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

	body, err := io.ReadAll(io.LimitReader(r.Body, h.maxBodySize+1))
	if err != nil {
		h.logger.Warn("failed to read ingest body", "error", err)
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if int64(len(body)) > h.maxBodySize {
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	var req model.HTTPRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.Warn("failed to decode ingest payload", "error", err)
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	select {
	case h.recordCh <- req:
		if h.onIngest != nil {
			h.onIngest()
		}
		w.WriteHeader(http.StatusAccepted)
	default:
		h.logger.Warn("record channel full, dropping request", "request_id", req.RequestID)
		if h.onError != nil {
			h.onError()
		}
		http.Error(w, "service overloaded", http.StatusServiceUnavailable)
	}
}
