package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Config struct {
	S3Bucket       string
	S3Prefix       string
	S3Region       string
	S3Endpoint     string
	OutputFormat   string
	StorageBackend string
	AzureAccount   string
	AzureContainer string
	FlushInterval  time.Duration
	BufferChanSize int
	MaxBodySize    int
	CollectorPort  int
	MetricsPort    int
	LogLevel       slog.Level
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.S3Bucket, "s3-bucket", envString("GOR_S3_BUCKET", ""), "S3 bucket name")
	flag.StringVar(&cfg.S3Prefix, "s3-prefix", envString("GOR_S3_PREFIX", ""), "S3 key prefix")
	flag.StringVar(&cfg.S3Region, "s3-region", envString("GOR_S3_REGION", ""), "AWS region")
	flag.StringVar(&cfg.S3Endpoint, "s3-endpoint", envString("GOR_S3_ENDPOINT", ""), "Custom S3 endpoint URL (for MinIO, etc)")
	flag.StringVar(&cfg.OutputFormat, "output-format", envString("GOR_OUTPUT_FORMAT", "parquet"), "Output format (parquet, har)")
	flag.StringVar(&cfg.StorageBackend, "storage-backend", envString("GOR_STORAGE_BACKEND", "s3"), "Storage backend (s3, azure)")
	flag.StringVar(&cfg.AzureAccount, "azure-account", envString("GOR_AZURE_ACCOUNT", ""), "Azure Storage account name")
	flag.StringVar(&cfg.AzureContainer, "azure-container", envString("GOR_AZURE_CONTAINER", ""), "Azure Blob container name")
	flushStr := flag.String("flush-interval", envString("GOR_FLUSH_INTERVAL", "5m"), "Flush interval (e.g. 30s, 5m)")
	flag.IntVar(&cfg.BufferChanSize, "buffer-chan-size", envInt("GOR_BUFFER_CHAN_SIZE", 10000), "Buffer channel size")
	flag.IntVar(&cfg.MaxBodySize, "max-body-size", envInt("GOR_MAX_BODY_SIZE", 1048576), "Max body size in bytes")
	flag.IntVar(&cfg.CollectorPort, "collector-port", envInt("GOR_COLLECTOR_PORT", 8080), "Collector HTTP ingest port")
	flag.IntVar(&cfg.MetricsPort, "metrics-port", envInt("GOR_METRICS_PORT", 9200), "Prometheus metrics port")
	logLevelStr := flag.String("log-level", envString("GOR_LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")

	flag.Parse()

	dur, err := time.ParseDuration(*flushStr)
	if err != nil {
		return nil, fmt.Errorf("invalid flush-interval %q: %w", *flushStr, err)
	}
	cfg.FlushInterval = dur

	switch *logLevelStr {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "info":
		cfg.LogLevel = slog.LevelInfo
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log-level %q: must be debug, info, warn, or error", *logLevelStr)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	switch c.StorageBackend {
	case "s3":
		if c.S3Bucket == "" {
			return fmt.Errorf("s3-bucket is required when storage-backend is s3")
		}
	case "azure":
		if c.AzureAccount == "" {
			return fmt.Errorf("azure-account is required when storage-backend is azure")
		}
		if c.AzureContainer == "" {
			return fmt.Errorf("azure-container is required when storage-backend is azure")
		}
	default:
		return fmt.Errorf("invalid storage-backend %q: must be s3 or azure", c.StorageBackend)
	}

	switch c.OutputFormat {
	case "parquet", "har":
	default:
		return fmt.Errorf("invalid output-format %q: must be parquet or har", c.OutputFormat)
	}

	if c.FlushInterval < time.Second {
		return fmt.Errorf("flush-interval must be at least 1s")
	}
	if c.BufferChanSize <= 0 {
		return fmt.Errorf("buffer-chan-size must be positive")
	}
	if c.MaxBodySize <= 0 {
		return fmt.Errorf("max-body-size must be positive")
	}
	if c.CollectorPort <= 0 || c.CollectorPort > 65535 {
		return fmt.Errorf("collector-port must be between 1 and 65535")
	}
	if c.MetricsPort <= 0 || c.MetricsPort > 65535 {
		return fmt.Errorf("metrics-port must be between 1 and 65535")
	}
	return nil
}

func envString(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
