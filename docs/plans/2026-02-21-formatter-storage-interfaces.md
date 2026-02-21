# Formatter & Storage Interfaces Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Decouple output format from storage backend by introducing `Formatter` and `Storage` interfaces, with Parquet + HAR formatters and S3 + Azure Blob storage implementations.

**Architecture:** Two orthogonal interfaces (`Formatter` and `Storage`) composed by the Flusher. The Flusher calls `Formatter.Format(records)` to serialize, then `Storage.Upload(key, data)` to persist. Format and backend are selected at runtime via CLI flags / env vars.

**Tech Stack:** Go 1.24, `parquet-go`, `azblob` SDK (`github.com/Azure/azure-sdk-for-go/sdk/storage/azblob`), `github.com/Azure/azure-sdk-for-go/sdk/azidentity`

---

### Task 1: Create Formatter Interface + ParquetFormatter

**Files:**
- Create: `internal/format/formatter.go`
- Create: `internal/format/parquet.go`
- Create: `internal/format/parquet_test.go`

**Step 1: Write the Formatter interface**

Create `internal/format/formatter.go`:

```go
package format

import "github.com/jycamier/meshcap/internal/model"

// Formatter serializes HTTP request records into a byte format.
type Formatter interface {
	Format(records []model.HTTPRequest) ([]byte, error)
	Extension() string
}
```

**Step 2: Write the failing test for ParquetFormatter**

Create `internal/format/parquet_test.go` — migrate existing tests from `internal/writer/parquet_test.go`, adapting to call `ParquetFormatter.Format()` instead of `WriteParquetBuffer()`:

```go
package format

import (
	"bytes"
	"io"
	"testing"

	"github.com/jycamier/meshcap/internal/model"
	"github.com/parquet-go/parquet-go"
)

func TestParquetFormatterRoundtrip(t *testing.T) {
	f := NewParquetFormatter()

	records := []model.HTTPRequest{
		{
			RequestID:   "req-1",
			CapturedAt:  "2024-01-15T10:30:00Z",
			TimestampNs: 1705312200000000000,
			ReqMethod:   "GET",
			ReqPath:     "/api/users",
			ReqHost:     "example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{"Content-Type":"application/json"}`,
			ReqBody:     nil,
			ReqBodySize: 0,
			ClientIP:    "10.0.0.1",
			SourcePod:   "app-abc123",
		},
		{
			RequestID:   "req-2",
			CapturedAt:  "2024-01-15T10:30:01Z",
			TimestampNs: 1705312201000000000,
			ReqMethod:   "POST",
			ReqPath:     "/api/submit",
			ReqHost:     "example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{"Content-Type":"application/json"}`,
			ReqBody:     []byte(`{"key":"value"}`),
			ReqBodySize: 15,
			ClientIP:    "10.0.0.2",
			SourcePod:   "app-def456",
		},
	}

	data, err := f.Format(records)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("output is empty")
	}

	reader := parquet.NewGenericReader[model.HTTPRequest](bytes.NewReader(data))
	defer reader.Close()

	if reader.NumRows() != 2 {
		t.Fatalf("NumRows = %d, want 2", reader.NumRows())
	}

	readBack := make([]model.HTTPRequest, 2)
	n, err := reader.Read(readBack)
	if err != nil && err != io.EOF {
		t.Fatalf("Read error: %v", err)
	}
	if n != 2 {
		t.Fatalf("Read n = %d, want 2", n)
	}
	if readBack[0].RequestID != "req-1" {
		t.Errorf("record[0].RequestID = %q, want %q", readBack[0].RequestID, "req-1")
	}
	if string(readBack[1].ReqBody) != `{"key":"value"}` {
		t.Errorf("record[1].ReqBody = %q, want %q", readBack[1].ReqBody, `{"key":"value"}`)
	}
}

func TestParquetFormatterEmpty(t *testing.T) {
	f := NewParquetFormatter()
	_, err := f.Format(nil)
	if err == nil {
		t.Fatal("expected error for empty records")
	}
}

func TestParquetFormatterExtension(t *testing.T) {
	f := NewParquetFormatter()
	if f.Extension() != ".parquet" {
		t.Errorf("Extension() = %q, want .parquet", f.Extension())
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/format/ -v`
Expected: FAIL — `NewParquetFormatter` undefined

**Step 4: Write ParquetFormatter implementation**

Create `internal/format/parquet.go`:

```go
package format

import (
	"bytes"
	"fmt"

	"github.com/jycamier/meshcap/internal/model"
	"github.com/parquet-go/parquet-go"
)

type ParquetFormatter struct{}

func NewParquetFormatter() *ParquetFormatter {
	return &ParquetFormatter{}
}

func (f *ParquetFormatter) Format(records []model.HTTPRequest) ([]byte, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("no records to write")
	}

	var buf bytes.Buffer
	w := parquet.NewGenericWriter[model.HTTPRequest](&buf)

	if _, err := w.Write(records); err != nil {
		return nil, fmt.Errorf("parquet write: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("parquet close: %w", err)
	}

	return buf.Bytes(), nil
}

func (f *ParquetFormatter) Extension() string {
	return ".parquet"
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/format/ -v -race`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/format/
git commit -m "feat: add Formatter interface and ParquetFormatter implementation"
```

---

### Task 2: Create HARFormatter

**Files:**
- Create: `internal/format/har.go`
- Create: `internal/format/har_test.go`

**Step 1: Write the failing test**

Create `internal/format/har_test.go`:

```go
package format

import (
	"encoding/json"
	"testing"

	"github.com/jycamier/meshcap/internal/model"
)

type harDocument struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Version string     `json:"version"`
	Creator harCreator `json:"creator"`
	Entries []harEntry `json:"entries"`
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harEntry struct {
	StartedDateTime string     `json:"startedDateTime"`
	Request         harRequest `json:"request"`
}

type harRequest struct {
	Method      string      `json:"method"`
	URL         string      `json:"url"`
	HTTPVersion string      `json:"httpVersion"`
	Headers     []harHeader `json:"headers"`
	BodySize    int64       `json:"bodySize"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func TestHARFormatterValidStructure(t *testing.T) {
	f := NewHARFormatter()

	records := []model.HTTPRequest{
		{
			RequestID:   "req-1",
			CapturedAt:  "2024-01-15T10:30:00Z",
			TimestampNs: 1705312200000000000,
			ReqMethod:   "GET",
			ReqPath:     "/api/users",
			ReqHost:     "example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{"Content-Type":"application/json","Accept":"*/*"}`,
			ReqBody:     nil,
			ReqBodySize: 0,
			ClientIP:    "10.0.0.1",
			SourcePod:   "app-abc123",
		},
		{
			RequestID:   "req-2",
			CapturedAt:  "2024-01-15T10:30:01Z",
			TimestampNs: 1705312201000000000,
			ReqMethod:   "POST",
			ReqPath:     "/api/submit",
			ReqHost:     "api.example.com",
			ReqVersion:  "HTTP/1.1",
			ReqHeaders:  `{}`,
			ReqBody:     []byte(`{"key":"value"}`),
			ReqBodySize: 15,
			ClientIP:    "10.0.0.2",
			SourcePod:   "app-def456",
		},
	}

	data, err := f.Format(records)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	var doc harDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if doc.Log.Version != "1.2" {
		t.Errorf("version = %q, want 1.2", doc.Log.Version)
	}
	if doc.Log.Creator.Name != "meshcap" {
		t.Errorf("creator.name = %q, want meshcap", doc.Log.Creator.Name)
	}
	if len(doc.Log.Entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(doc.Log.Entries))
	}

	entry0 := doc.Log.Entries[0]
	if entry0.Request.Method != "GET" {
		t.Errorf("entry[0].request.method = %q, want GET", entry0.Request.Method)
	}
	if entry0.Request.URL != "http://example.com/api/users" {
		t.Errorf("entry[0].request.url = %q, want http://example.com/api/users", entry0.Request.URL)
	}
	if entry0.Request.HTTPVersion != "HTTP/1.1" {
		t.Errorf("entry[0].request.httpVersion = %q, want HTTP/1.1", entry0.Request.HTTPVersion)
	}
	if entry0.StartedDateTime != "2024-01-15T10:30:00Z" {
		t.Errorf("entry[0].startedDateTime = %q, want 2024-01-15T10:30:00Z", entry0.StartedDateTime)
	}

	entry1 := doc.Log.Entries[1]
	if entry1.Request.Method != "POST" {
		t.Errorf("entry[1].request.method = %q, want POST", entry1.Request.Method)
	}
	if entry1.Request.BodySize != 15 {
		t.Errorf("entry[1].request.bodySize = %d, want 15", entry1.Request.BodySize)
	}
}

func TestHARFormatterEmpty(t *testing.T) {
	f := NewHARFormatter()
	_, err := f.Format(nil)
	if err == nil {
		t.Fatal("expected error for empty records")
	}
}

func TestHARFormatterExtension(t *testing.T) {
	f := NewHARFormatter()
	if f.Extension() != ".har" {
		t.Errorf("Extension() = %q, want .har", f.Extension())
	}
}

func TestHARFormatterHeadersParsing(t *testing.T) {
	f := NewHARFormatter()

	records := []model.HTTPRequest{
		{
			RequestID:  "req-h",
			CapturedAt: "2024-01-15T10:30:00Z",
			ReqMethod:  "GET",
			ReqPath:    "/test",
			ReqHost:    "example.com",
			ReqVersion: "HTTP/1.1",
			ReqHeaders: `{"X-Custom":"foo","Authorization":"Bearer tok"}`,
		},
	}

	data, err := f.Format(records)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	var doc harDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	headers := doc.Log.Entries[0].Request.Headers
	if len(headers) != 2 {
		t.Fatalf("headers count = %d, want 2", len(headers))
	}
}

func TestHARFormatterInvalidHeaders(t *testing.T) {
	f := NewHARFormatter()

	records := []model.HTTPRequest{
		{
			RequestID:  "req-bad",
			CapturedAt: "2024-01-15T10:30:00Z",
			ReqMethod:  "GET",
			ReqPath:    "/test",
			ReqHost:    "example.com",
			ReqVersion: "HTTP/1.1",
			ReqHeaders: `not valid json`,
		},
	}

	data, err := f.Format(records)
	if err != nil {
		t.Fatalf("Format error: %v (should not fail on bad headers)", err)
	}

	var doc harDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should produce empty headers, not crash
	if len(doc.Log.Entries[0].Request.Headers) != 0 {
		t.Errorf("expected 0 headers for invalid JSON, got %d", len(doc.Log.Entries[0].Request.Headers))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/format/ -run TestHAR -v`
Expected: FAIL — `NewHARFormatter` undefined

**Step 3: Write HARFormatter implementation**

Create `internal/format/har.go`:

```go
package format

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jycamier/meshcap/internal/model"
)

type HARFormatter struct{}

func NewHARFormatter() *HARFormatter {
	return &HARFormatter{}
}

type harDoc struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Version string     `json:"version"`
	Creator harCreator `json:"creator"`
	Entries []harEntry `json:"entries"`
}

type harCreator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type harEntry struct {
	StartedDateTime string      `json:"startedDateTime"`
	Request         harReq      `json:"request"`
	Response        harResponse `json:"response"`
}

type harReq struct {
	Method      string       `json:"method"`
	URL         string       `json:"url"`
	HTTPVersion string       `json:"httpVersion"`
	Headers     []harHeader  `json:"headers"`
	PostData    *harPostData `json:"postData,omitempty"`
	BodySize    int64        `json:"bodySize"`
}

type harHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type harResponse struct {
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
}

func (f *HARFormatter) Format(records []model.HTTPRequest) ([]byte, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("no records to format")
	}

	entries := make([]harEntry, 0, len(records))
	for _, r := range records {
		entry := harEntry{
			StartedDateTime: r.CapturedAt,
			Request: harReq{
				Method:      r.ReqMethod,
				URL:         fmt.Sprintf("http://%s%s", r.ReqHost, r.ReqPath),
				HTTPVersion: r.ReqVersion,
				Headers:     parseHeaders(r.ReqHeaders),
				BodySize:    r.ReqBodySize,
			},
			Response: harResponse{
				Status:     0,
				StatusText: "",
			},
		}

		if len(r.ReqBody) > 0 {
			mimeType := "application/octet-stream"
			hdrs := parseHeaderMap(r.ReqHeaders)
			if ct, ok := hdrs["content-type"]; ok {
				mimeType = ct
			} else if ct, ok := hdrs["Content-Type"]; ok {
				mimeType = ct
			}
			entry.Request.PostData = &harPostData{
				MimeType: mimeType,
				Text:     string(r.ReqBody),
			}
		}

		entries = append(entries, entry)
	}

	doc := harDoc{
		Log: harLog{
			Version: "1.2",
			Creator: harCreator{
				Name:    "meshcap",
				Version: "1.0",
			},
			Entries: entries,
		},
	}

	return json.Marshal(doc)
}

func (f *HARFormatter) Extension() string {
	return ".har"
}

func parseHeaders(headersJSON string) []harHeader {
	m := parseHeaderMap(headersJSON)
	if m == nil {
		return []harHeader{}
	}

	headers := make([]harHeader, 0, len(m))
	for k, v := range m {
		headers = append(headers, harHeader{Name: k, Value: v})
	}
	sort.Slice(headers, func(i, j int) bool {
		return headers[i].Name < headers[j].Name
	})
	return headers
}

func parseHeaderMap(headersJSON string) map[string]string {
	var m map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &m); err != nil {
		return nil
	}
	return m
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/format/ -v -race`
Expected: PASS (all Parquet + HAR tests)

**Step 5: Commit**

```bash
git add internal/format/har.go internal/format/har_test.go
git commit -m "feat: add HARFormatter implementation (HAR 1.2)"
```

---

### Task 3: Create Storage Interface + S3Storage

**Files:**
- Create: `internal/storage/storage.go`
- Create: `internal/storage/s3.go`
- Create: `internal/storage/s3_test.go`

**Step 1: Write the Storage interface and BuildKey helper**

Create `internal/storage/storage.go`:

```go
package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Storage uploads serialized data to a backend.
type Storage interface {
	Upload(ctx context.Context, key string, data []byte) error
}

// BuildKey creates a Hive-partitioned key.
// Format: {prefix}/host={host}/year=YYYY/month=MM/day=DD/hour=HH/{uuid}{ext}
func BuildKey(prefix string, host string, t time.Time, ext string) string {
	safeHost := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, host)

	if safeHost == "" {
		safeHost = "_unknown_"
	}

	key := fmt.Sprintf("host=%s/year=%04d/month=%02d/day=%02d/hour=%02d/%s%s",
		safeHost,
		t.Year(), t.Month(), t.Day(), t.Hour(),
		uuid.New().String(),
		ext,
	)

	if prefix != "" {
		prefix = strings.TrimRight(prefix, "/")
		key = prefix + "/" + key
	}

	return key
}
```

**Step 2: Write BuildKey tests**

Create `internal/storage/s3_test.go` — migrate existing tests from `internal/writer/uploader_test.go`, updating `BuildS3Key` → `BuildKey` and adding ext parameter:

```go
package storage

import (
	"strings"
	"testing"
	"time"
)

func TestBuildKey(t *testing.T) {
	ts := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)

	key := BuildKey("my-prefix", "example.com", ts, ".parquet")

	if !strings.HasPrefix(key, "my-prefix/host=example.com/year=2024/month=03/day=15/hour=14/") {
		t.Errorf("unexpected key prefix: %s", key)
	}
	if !strings.HasSuffix(key, ".parquet") {
		t.Errorf("key should end with .parquet: %s", key)
	}
}

func TestBuildKeyHARExtension(t *testing.T) {
	ts := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)

	key := BuildKey("", "example.com", ts, ".har")

	if !strings.HasSuffix(key, ".har") {
		t.Errorf("key should end with .har: %s", key)
	}
}

func TestBuildKeyNoPrefix(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	key := BuildKey("", "api.example.com", ts, ".parquet")

	if !strings.HasPrefix(key, "host=api.example.com/year=2024/month=01/day=01/hour=00/") {
		t.Errorf("unexpected key: %s", key)
	}
}

func TestBuildKeySpecialCharsInHost(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	key := BuildKey("pfx", "host:8080/path?q=1", ts, ".parquet")

	if strings.Contains(key, ":") || strings.Contains(key, "?") || strings.Contains(key, "/path") {
		t.Errorf("special chars not sanitized in key: %s", key)
	}
	if !strings.Contains(key, "host=host_8080_path_q_1") {
		t.Errorf("expected sanitized host in key: %s", key)
	}
}

func TestBuildKeyEmptyHost(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	key := BuildKey("", "", ts, ".parquet")

	if !strings.Contains(key, "host=_unknown_") {
		t.Errorf("empty host should become _unknown_: %s", key)
	}
}

func TestBuildKeyTrailingSlashPrefix(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	key := BuildKey("prefix/", "example.com", ts, ".parquet")

	if strings.Contains(key, "prefix//") {
		t.Errorf("double slash in key: %s", key)
	}
}

func TestBuildKeyUniqueUUIDs(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	key1 := BuildKey("", "example.com", ts, ".parquet")
	key2 := BuildKey("", "example.com", ts, ".parquet")

	if key1 == key2 {
		t.Error("keys should be unique due to UUID")
	}
}
```

**Step 3: Run tests to verify they pass**

Run: `go test ./internal/storage/ -v -race`
Expected: PASS

**Step 4: Write S3Storage implementation**

Create `internal/storage/s3.go`:

```go
package storage

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appconfig "github.com/jycamier/meshcap/internal/config"
)

type S3Storage struct {
	bucket   string
	uploader *manager.Uploader
	logger   *slog.Logger
}

func NewS3Storage(ctx context.Context, cfg *appconfig.Config, logger *slog.Logger) (*S3Storage, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if cfg.S3Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.S3Region))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint)
			o.UsePathStyle = true
		}
	})
	uploader := manager.NewUploader(client)

	return &S3Storage{
		bucket:   cfg.S3Bucket,
		uploader: uploader,
		logger:   logger,
	}, nil
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte) error {
	_, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("s3 upload %q: %w", key, err)
	}

	s.logger.Info("uploaded file to S3", "key", key, "size", len(data))
	return nil
}
```

**Step 5: Run all storage tests**

Run: `go test ./internal/storage/ -v -race`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/storage/
git commit -m "feat: add Storage interface, BuildKey helper, and S3Storage implementation"
```

---

### Task 4: Create AzureBlobStorage

**Files:**
- Create: `internal/storage/azure.go`
- Create: `internal/storage/azure_test.go`

**Step 1: Add Azure SDK dependencies**

Run: `go get github.com/Azure/azure-sdk-for-go/sdk/storage/azblob github.com/Azure/azure-sdk-for-go/sdk/azidentity`

**Step 2: Write the failing test**

Create `internal/storage/azure_test.go`:

```go
package storage

import (
	"context"
	"testing"

	appconfig "github.com/jycamier/meshcap/internal/config"
)

func TestNewAzureBlobStorageValidation(t *testing.T) {
	cfg := &appconfig.Config{
		AzureAccount:   "",
		AzureContainer: "test",
	}

	_, err := NewAzureBlobStorage(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("expected error for empty account")
	}
}

func TestNewAzureBlobStorageValidationContainer(t *testing.T) {
	cfg := &appconfig.Config{
		AzureAccount:   "myaccount",
		AzureContainer: "",
	}

	_, err := NewAzureBlobStorage(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("expected error for empty container")
	}
}
```

**Step 3: Run test to verify it fails**

Run: `go test ./internal/storage/ -run TestNewAzure -v`
Expected: FAIL — `AzureAccount` field undefined on Config, `NewAzureBlobStorage` undefined

**Step 4: Add Azure fields to Config**

Modify `internal/config/config.go`:

Add fields to the Config struct:
```go
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
```

Add flag parsing in `Load()` (after existing flags, before `flag.Parse()`):
```go
flag.StringVar(&cfg.OutputFormat, "output-format", envString("GOR_OUTPUT_FORMAT", "parquet"), "Output format (parquet, har)")
flag.StringVar(&cfg.StorageBackend, "storage-backend", envString("GOR_STORAGE_BACKEND", "s3"), "Storage backend (s3, azure)")
flag.StringVar(&cfg.AzureAccount, "azure-account", envString("GOR_AZURE_ACCOUNT", ""), "Azure Storage account name")
flag.StringVar(&cfg.AzureContainer, "azure-container", envString("GOR_AZURE_CONTAINER", ""), "Azure Blob container name")
```

Update `Validate()` to conditionally validate based on backend:
```go
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
	// ... rest of existing validation unchanged
}
```

**Step 5: Write AzureBlobStorage implementation**

Create `internal/storage/azure.go`:

```go
package storage

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	appconfig "github.com/jycamier/meshcap/internal/config"
)

type AzureBlobStorage struct {
	client    *azblob.Client
	container string
	logger    *slog.Logger
}

func NewAzureBlobStorage(ctx context.Context, cfg *appconfig.Config, logger *slog.Logger) (*AzureBlobStorage, error) {
	if cfg.AzureAccount == "" {
		return nil, fmt.Errorf("azure-account is required")
	}
	if cfg.AzureContainer == "" {
		return nil, fmt.Errorf("azure-container is required")
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azure credential: %w", err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", cfg.AzureAccount)
	client, err := azblob.NewClient(serviceURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azure blob client: %w", err)
	}

	return &AzureBlobStorage{
		client:    client,
		container: cfg.AzureContainer,
		logger:    logger,
	}, nil
}

func (a *AzureBlobStorage) Upload(ctx context.Context, key string, data []byte) error {
	_, err := a.client.UploadBuffer(ctx, a.container, key, data, &azblob.UploadBufferOptions{})
	if err != nil {
		return fmt.Errorf("azure upload %q: %w", key, err)
	}

	if a.logger != nil {
		a.logger.Info("uploaded file to Azure Blob", "key", key, "size", len(data), "container", a.container)
	}
	return nil
}
```

**Step 6: Run tests**

Run: `go test ./internal/storage/ -v -race`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/storage/azure.go internal/storage/azure_test.go internal/config/config.go go.mod go.sum
git commit -m "feat: add AzureBlobStorage + config flags for output-format and storage-backend"
```

---

### Task 5: Refactor Flusher to Use Interfaces

**Files:**
- Modify: `internal/writer/flusher.go`
- Modify: `internal/writer/flusher_test.go`
- Delete: `internal/writer/parquet.go`
- Delete: `internal/writer/parquet_test.go`
- Delete: `internal/writer/uploader.go`
- Delete: `internal/writer/uploader_test.go`

**Step 1: Update flusher_test.go to use new interfaces**

Replace the `testUploader` mock with mocks for `Formatter` and `Storage`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/writer/ -v`
Expected: FAIL — `NewFlusher` signature mismatch

**Step 3: Rewrite flusher.go**

Replace `internal/writer/flusher.go`:

```go
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
```

**Step 4: Delete old files**

```bash
rm internal/writer/parquet.go internal/writer/parquet_test.go internal/writer/uploader.go internal/writer/uploader_test.go
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/writer/ -v -race`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/writer/ internal/storage/ internal/format/
git rm internal/writer/parquet.go internal/writer/parquet_test.go internal/writer/uploader.go internal/writer/uploader_test.go
git commit -m "refactor: flusher now uses Formatter and Storage interfaces"
```

---

### Task 6: Wire Everything in main.go

**Files:**
- Modify: `cmd/collector/main.go`

**Step 1: Update main.go imports and wiring**

Replace the uploader/writer setup in `run()` with interface-based wiring:

```go
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
```

**Step 2: Build to verify compilation**

Run: `go build ./cmd/collector/`
Expected: SUCCESS

**Step 3: Run all tests**

Run: `go test -race -cover ./...`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add cmd/collector/main.go
git commit -m "feat: wire Formatter and Storage interfaces in main.go"
```

---

### Task 7: Update Documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `doc/architecture.md`
- Modify: `doc/configuration.md`
- Modify: `README.md`

**Step 1: Update CLAUDE.md architecture section**

Add the new packages `internal/format/` and `internal/storage/` to the Key Components section. Update the Flusher description to mention it composes Formatter + Storage.

**Step 2: Update doc/architecture.md**

- Update the component diagram to show Formatter and Storage as separate boxes
- Add descriptions for the new interfaces and implementations
- Update the data flow to show: Flusher → Formatter → Storage

**Step 3: Update doc/configuration.md**

- Add `--output-format` / `GOR_OUTPUT_FORMAT` to the options table
- Add `--storage-backend` / `GOR_STORAGE_BACKEND` to the options table
- Add `--azure-account` / `GOR_AZURE_ACCOUNT` to the options table
- Add `--azure-container` / `GOR_AZURE_CONTAINER` to the options table
- Add Azure Helm values section
- Add Azure configuration example

**Step 4: Update README.md features list**

Add mention of pluggable format (Parquet, HAR) and storage (S3, Azure Blob) backends.

**Step 5: Commit**

```bash
git add CLAUDE.md doc/ README.md
git commit -m "docs: update documentation for Formatter and Storage interfaces"
```

---

### Task 8: Final Verification

**Step 1: Run full test suite**

Run: `go test -race -cover ./...`
Expected: ALL PASS

**Step 2: Build binary**

Run: `make build`
Expected: SUCCESS

**Step 3: Lint**

Run: `make lint`
Expected: PASS

**Step 4: Verify docker-compose still works**

Run: `docker compose build`
Expected: SUCCESS (builds with new code)
