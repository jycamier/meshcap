package collector

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jycamier/meshcap/internal/model"
)

func TestHandlerAcceptsValidRequest(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	var ingested atomic.Int32
	h := NewHandler(ch, 1048576, slog.Default(), func() { ingested.Add(1) }, nil)

	req := model.HTTPRequest{
		RequestID:   "test-1",
		CapturedAt:  "2024-01-15T10:30:00Z",
		TimestampNs: 1705312200000000000,
		ReqMethod:   "GET",
		ReqPath:     "/api/users",
		ReqHost:     "example.com",
		ReqVersion:  "HTTP/1.1",
		ReqHeaders:  `{}`,
		ClientIP:    "10.0.0.1",
		SourcePod:   "app-abc123",
	}

	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status 202, got %d", w.Code)
	}
	if ingested.Load() != 1 {
		t.Errorf("expected 1 ingested, got %d", ingested.Load())
	}

	select {
	case received := <-ch:
		if received.RequestID != "test-1" {
			t.Errorf("expected RequestID test-1, got %s", received.RequestID)
		}
	default:
		t.Error("expected record in channel")
	}
}

func TestHandlerRejectsGetMethod(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	h := NewHandler(ch, 1048576, slog.Default(), nil, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ingest", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlerRejectsInvalidJSON(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	var errors atomic.Int32
	h := NewHandler(ch, 1048576, slog.Default(), nil, func() { errors.Add(1) })

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader("not json"))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if errors.Load() != 1 {
		t.Errorf("expected 1 error, got %d", errors.Load())
	}
}

func TestHandlerRejectsTooLargePayload(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	var errors atomic.Int32
	h := NewHandler(ch, 100, slog.Default(), nil, func() { errors.Add(1) })

	largePayload := strings.Repeat("x", 200)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(largePayload))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status 413, got %d", w.Code)
	}
}

func TestHandlerReturns503WhenChannelFull(t *testing.T) {
	ch := make(chan model.HTTPRequest, 1)
	ch <- model.HTTPRequest{} // fill the channel

	var errors atomic.Int32
	h := NewHandler(ch, 1048576, slog.Default(), nil, func() { errors.Add(1) })

	req := model.HTTPRequest{RequestID: "overflow"}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}
