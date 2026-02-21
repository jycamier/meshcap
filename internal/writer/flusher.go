package writer

import (
	"context"
	"log/slog"
	"time"

	"github.com/jycamier/meshcap/internal/format"
	"github.com/jycamier/meshcap/internal/model"
	"github.com/jycamier/meshcap/internal/storage"
)

type Flusher struct {
	ch            <-chan model.HTTPRequest
	formatter     format.Formatter
	storage       storage.Storage
	prefix        string
	flushInterval time.Duration
	logger        *slog.Logger
	onFlush       func(host string, count int, size int, err error)
}

func NewFlusher(
	ch <-chan model.HTTPRequest,
	formatter format.Formatter,
	storage storage.Storage,
	prefix string,
	flushInterval time.Duration,
	logger *slog.Logger,
	onFlush func(host string, count int, size int, err error),
) *Flusher {
	return &Flusher{
		ch:            ch,
		formatter:     formatter,
		storage:       storage,
		prefix:        prefix,
		flushInterval: flushInterval,
		logger:        logger,
		onFlush:       onFlush,
	}
}

func (f *Flusher) Run(ctx context.Context) {
	ticker := time.NewTicker(f.flushInterval)
	defer ticker.Stop()

	var buffer []model.HTTPRequest

	for {
		select {
		case record, ok := <-f.ch:
			if !ok {
				f.flush(context.Background(), buffer)
				return
			}
			buffer = append(buffer, record)

		case <-ticker.C:
			if len(buffer) > 0 {
				f.flush(ctx, buffer)
				buffer = nil
			}

		case <-ctx.Done():
			for record := range f.ch {
				buffer = append(buffer, record)
			}
			f.flush(context.Background(), buffer)
			return
		}
	}
}

func (f *Flusher) flush(ctx context.Context, records []model.HTTPRequest) {
	if len(records) == 0 {
		return
	}

	groups := make(map[string][]model.HTTPRequest)
	for _, r := range records {
		host := r.ReqHost
		if host == "" {
			host = "_unknown_"
		}
		groups[host] = append(groups[host], r)
	}

	for host, hostRecords := range groups {
		data, err := f.formatter.Format(hostRecords)
		if err != nil {
			f.logger.Error("failed to format records", "host", host, "error", err, "records", len(hostRecords))
			if f.onFlush != nil {
				f.onFlush(host, len(hostRecords), 0, err)
			}
			continue
		}

		now := time.Now().UTC()
		key := storage.BuildKey(f.prefix, host, now, f.formatter.Extension())

		err = f.uploadWithRetry(ctx, key, data)
		if f.onFlush != nil {
			f.onFlush(host, len(hostRecords), len(data), err)
		}
		if err != nil {
			f.logger.Error("failed to upload after retries", "key", key, "error", err, "records", len(hostRecords))
		}
	}
}

func (f *Flusher) uploadWithRetry(ctx context.Context, key string, data []byte) error {
	backoffs := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

	var lastErr error
	for attempt := 0; attempt <= len(backoffs); attempt++ {
		lastErr = f.storage.Upload(ctx, key, data)
		if lastErr == nil {
			return nil
		}

		if attempt < len(backoffs) {
			f.logger.Warn("upload failed, retrying",
				"key", key,
				"attempt", attempt+1,
				"backoff", backoffs[attempt],
				"error", lastErr,
			)

			select {
			case <-time.After(backoffs[attempt]):
			case <-ctx.Done():
				return lastErr
			}
		}
	}

	return lastErr
}
