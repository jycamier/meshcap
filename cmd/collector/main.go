package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jycamier/meshcap/internal/collector"
	"github.com/jycamier/meshcap/internal/config"
	"github.com/jycamier/meshcap/internal/format"
	"github.com/jycamier/meshcap/internal/metrics"
	"github.com/jycamier/meshcap/internal/model"
	"github.com/jycamier/meshcap/internal/storage"
	"github.com/jycamier/meshcap/internal/writer"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	m := metrics.New()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	metrics.StartServer(ctx, cfg.MetricsPort, logger)

	// Select formatter
	var fmtr format.Formatter
	switch cfg.OutputFormat {
	case "parquet":
		fmtr = format.NewParquetFormatter()
	case "har":
		fmtr = format.NewHARFormatter()
	default:
		return fmt.Errorf("unsupported output format: %s", cfg.OutputFormat)
	}

	// Select storage backend
	var store storage.Storage
	switch cfg.StorageBackend {
	case "s3":
		store, err = storage.NewS3Storage(ctx, cfg, logger)
		if err != nil {
			return fmt.Errorf("s3 storage: %w", err)
		}
	case "azure":
		store, err = storage.NewAzureBlobStorage(ctx, cfg, logger)
		if err != nil {
			return fmt.Errorf("azure storage: %w", err)
		}
	default:
		return fmt.Errorf("unsupported storage backend: %s", cfg.StorageBackend)
	}

	recordCh := make(chan model.HTTPRequest, cfg.BufferChanSize)

	var wg sync.WaitGroup
	onFlush := func(host string, count int, size int, flushErr error) {
		m.RecordsFlushed.Add(float64(count))
		if flushErr != nil {
			m.S3Uploads.WithLabelValues("error").Inc()
		} else {
			m.S3Uploads.WithLabelValues("success").Inc()
			m.ParquetFileSize.Observe(float64(size))
		}
	}
	flusher := writer.NewFlusher(recordCh, fmtr, store, cfg.S3Prefix, cfg.FlushInterval, logger, onFlush)

	wg.Add(1)
	go func() {
		defer wg.Done()
		flusher.Run(ctx)
	}()

	handler := collector.NewHandler(
		recordCh,
		cfg.MaxBodySize,
		logger,
		func() {
			m.RequestsIngested.Inc()
			m.BufferSize.Set(float64(len(recordCh)))
		},
		func() {
			m.IngestErrors.Inc()
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/ingest", handler)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.CollectorPort),
		Handler: mux,
	}

	logger.Info("collector starting",
		"version", version,
		"port", cfg.CollectorPort,
		"format", cfg.OutputFormat,
		"storage", cfg.StorageBackend,
		"flush_interval", cfg.FlushInterval,
		"metrics_port", cfg.MetricsPort,
	)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("collector server error", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down collector")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("collector server shutdown error", "error", err)
	}

	close(recordCh)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("shutdown complete")
	case <-time.After(60 * time.Second):
		logger.Error("shutdown timed out after 60s")
	}

	return nil
}
