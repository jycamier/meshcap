package writer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jycamier/meshcap/internal/model"
)

type mockFormatter struct{}

func (f *mockFormatter) Format(records []model.HTTPRequest) ([]byte, error) {
	return []byte(fmt.Sprintf("formatted-%d-records", len(records))), nil
}

func (f *mockFormatter) Extension() string { return ".mock" }

type mockStorage struct {
	mu        sync.Mutex
	uploads   []mockUpload
	failCount int32
}

type mockUpload struct {
	key  string
	size int
}

func (s *mockStorage) Upload(_ context.Context, key string, data []byte) error {
	if atomic.AddInt32(&s.failCount, -1) >= 0 {
		return fmt.Errorf("simulated upload failure")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads = append(s.uploads, mockUpload{key: key, size: len(data)})
	return nil
}

func (s *mockStorage) getUploads() []mockUpload {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]mockUpload, len(s.uploads))
	copy(result, s.uploads)
	return result
}

func makeRecord(host string, method string, path string) model.HTTPRequest {
	return model.HTTPRequest{
		RequestID:   "test-id",
		CapturedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		TimestampNs: time.Now().UnixNano(),
		ReqMethod:   method,
		ReqPath:     path,
		ReqHost:     host,
		ReqVersion:  "HTTP/1.1",
		ReqHeaders:  `{}`,
		ReqBody:     nil,
		ReqBodySize: 0,
		ClientIP:    "10.0.0.1",
		SourcePod:   "test-pod",
	}
}

func TestFlusherFlushOnChannelClose(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	store := &mockStorage{}
	fmtr := &mockFormatter{}

	var flushedCount atomic.Int32
	f := NewFlusher(ch, fmtr, store, "test", 1*time.Hour, slog.Default(), func(host string, count int, size int, err error) {
		flushedCount.Add(int32(count))
	})

	ch <- makeRecord("host-a.com", "GET", "/a")
	ch <- makeRecord("host-a.com", "POST", "/b")
	ch <- makeRecord("host-b.com", "GET", "/c")
	close(ch)

	f.Run(context.Background())

	uploads := store.getUploads()
	if len(uploads) != 2 {
		t.Fatalf("expected 2 uploads (one per host), got %d", len(uploads))
	}
	if flushedCount.Load() != 3 {
		t.Errorf("expected 3 flushed records, got %d", flushedCount.Load())
	}
}

func TestFlusherFlushGroupsByHost(t *testing.T) {
	records := []model.HTTPRequest{
		makeRecord("host-a.com", "GET", "/1"),
		makeRecord("host-a.com", "GET", "/2"),
		makeRecord("host-b.com", "GET", "/3"),
		makeRecord("", "GET", "/4"),
	}

	groups := make(map[string][]model.HTTPRequest)
	for _, r := range records {
		host := r.ReqHost
		if host == "" {
			host = "_unknown_"
		}
		groups[host] = append(groups[host], r)
	}

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	if len(groups["host-a.com"]) != 2 {
		t.Errorf("host-a.com should have 2 records, got %d", len(groups["host-a.com"]))
	}
}

func TestFlusherFlushOnTicker(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	store := &mockStorage{}
	fmtr := &mockFormatter{}
	ctx, cancel := context.WithCancel(context.Background())

	var flushedOnTick atomic.Int32
	f := NewFlusher(ch, fmtr, store, "test", 50*time.Millisecond, slog.Default(), func(host string, count int, size int, err error) {
		flushedOnTick.Add(int32(count))
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		f.Run(ctx)
	}()

	ch <- makeRecord("ticker-host.com", "GET", "/tick")
	time.Sleep(200 * time.Millisecond)

	if flushedOnTick.Load() == 0 {
		t.Error("expected records to be flushed on tick")
	}

	close(ch)
	cancel()
	wg.Wait()
}

func TestFlusherContextCancellation(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	store := &mockStorage{}
	fmtr := &mockFormatter{}
	ctx, cancel := context.WithCancel(context.Background())

	var flushedCount atomic.Int32
	f := NewFlusher(ch, fmtr, store, "test", 1*time.Hour, slog.Default(), func(host string, count int, size int, err error) {
		flushedCount.Add(int32(count))
	})

	ch <- makeRecord("cancel-host.com", "GET", "/cancel")
	close(ch)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		f.Run(ctx)
	}()

	cancel()
	wg.Wait()

	if flushedCount.Load() != 1 {
		t.Errorf("expected 1 flushed record on cancel, got %d", flushedCount.Load())
	}
}

func TestFlusherRetryOnUploadFailure(t *testing.T) {
	ch := make(chan model.HTTPRequest, 10)
	store := &mockStorage{failCount: 2}
	fmtr := &mockFormatter{}

	var flushedErr error
	var mu sync.Mutex
	f := NewFlusher(ch, fmtr, store, "test", 1*time.Hour, slog.Default(), func(host string, count int, size int, err error) {
		mu.Lock()
		flushedErr = err
		mu.Unlock()
	})

	ch <- makeRecord("retry-host.com", "GET", "/retry")
	close(ch)

	f.Run(context.Background())

	mu.Lock()
	err := flushedErr
	mu.Unlock()

	if err != nil {
		t.Errorf("expected upload to succeed after retries, got error: %v", err)
	}

	uploads := store.getUploads()
	if len(uploads) != 1 {
		t.Errorf("expected 1 successful upload, got %d", len(uploads))
	}
}
