# Formatter & Storage Interfaces Design

**Date:** 2026-02-21
**Status:** Approved

## Goal

Introduce two orthogonal interfaces in the collector to decouple output format (Parquet, HAR) from storage backend (S3, Azure Blob). This enables any combination of format + storage without combinatorial explosion.

## Interfaces

### Formatter

```go
// internal/format/formatter.go
type Formatter interface {
    Format(records []model.HTTPRequest) ([]byte, error)
    Extension() string // ".parquet", ".har"
}
```

Implementations:
- `ParquetFormatter` — Refactored from `writer/parquet.go`, zstd compression
- `HARFormatter` — HAR 1.2 JSON format, one entry per HTTPRequest

### Storage

```go
// internal/storage/storage.go
type Storage interface {
    Upload(ctx context.Context, key string, data []byte) error
}
```

Implementations:
- `S3Storage` — Refactored from `writer/uploader.go`, AWS SDK v2, custom endpoint support
- `AzureBlobStorage` — Azure SDK with DefaultAzureCredential

## Package Layout

```
internal/
├── format/
│   ├── formatter.go       # Interface
│   ├── parquet.go          # ParquetFormatter
│   ├── parquet_test.go
│   ├── har.go              # HARFormatter
│   └── har_test.go
├── storage/
│   ├── storage.go          # Interface + BuildKey helper
│   ├── s3.go               # S3Storage
│   ├── s3_test.go
│   ├── azure.go            # AzureBlobStorage
│   └── azure_test.go
├── writer/
│   ├── flusher.go          # Refactored: receives Formatter + Storage
│   └── flusher_test.go
```

Files removed: `writer/parquet.go`, `writer/parquet_test.go`, `writer/uploader.go`, `writer/uploader_test.go`

## Flusher Changes

New signature:
```go
func NewFlusher(
    ch <-chan model.HTTPRequest,
    formatter format.Formatter,
    storage storage.Storage,
    prefix string,
    flushInterval time.Duration,
    logger *slog.Logger,
    onFlush func(host string, count int, size int, err error),
) *Flusher
```

Flush pipeline: group by host → formatter.Format(records) → BuildKey(prefix, host, time, extension) → storage.Upload(ctx, key, data) with retry.

## BuildKey

Moved from `writer/uploader.go` to `storage/storage.go`. Updated to accept extension parameter:
```
{prefix}/host={host}/year=YYYY/month=MM/day=DD/hour=HH/{uuid}{ext}
```

## Configuration

New flags/env vars:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--output-format` | `GOR_OUTPUT_FORMAT` | `parquet` | Output format (`parquet`, `har`) |
| `--storage-backend` | `GOR_STORAGE_BACKEND` | `s3` | Storage backend (`s3`, `azure`) |
| `--azure-account` | `GOR_AZURE_ACCOUNT` | `""` | Azure Storage account name |
| `--azure-container` | `GOR_AZURE_CONTAINER` | `""` | Azure Blob container name |

Validation: S3 fields validated only when `storage-backend=s3`, Azure fields only when `storage-backend=azure`.

## Azure Blob Storage

- Uses `azblob` SDK with `DefaultAzureCredential` (supports Managed Identity, env vars, Azure CLI)
- Same Hive-style key structure as S3
- Requires `--azure-account` and `--azure-container`

## HAR Format

- Produces valid HAR 1.2 JSON documents
- One `entry` per HTTPRequest in the `entries` array
- Extension: `.har`

## Wiring (main.go)

```go
var fmtr format.Formatter
switch cfg.OutputFormat {
case "parquet":
    fmtr = format.NewParquetFormatter()
case "har":
    fmtr = format.NewHARFormatter()
}

var store storage.Storage
switch cfg.StorageBackend {
case "s3":
    store, err = storage.NewS3Storage(ctx, cfg, logger)
case "azure":
    store, err = storage.NewAzureBlobStorage(ctx, cfg, logger)
}

flusher := writer.NewFlusher(recordCh, fmtr, store, cfg.Prefix, cfg.FlushInterval, logger, onFlush)
```

## Testing

- Unit tests for HARFormatter (valid HAR 1.2 structure)
- Unit tests for AzureBlobStorage with mock
- Existing Flusher tests adapted for new interfaces
- Existing Parquet tests migrated to format/parquet_test.go
