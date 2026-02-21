package format

import (
	"bytes"
	"io"
	"testing"

	"github.com/jycamier/meshcap/internal/model"
	"github.com/parquet-go/parquet-go"
)

func TestParquetFormatterRoundtrip(t *testing.T) {
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

	f := NewParquetFormatter()
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
	if readBack[0].ClientIP != "10.0.0.1" {
		t.Errorf("record[0].ClientIP = %q, want %q", readBack[0].ClientIP, "10.0.0.1")
	}
	if readBack[1].ReqMethod != "POST" {
		t.Errorf("record[1].ReqMethod = %q, want POST", readBack[1].ReqMethod)
	}
	if string(readBack[1].ReqBody) != `{"key":"value"}` {
		t.Errorf("record[1].ReqBody = %q, want %q", readBack[1].ReqBody, `{"key":"value"}`)
	}
}

func TestParquetFormatterEmpty(t *testing.T) {
	f := NewParquetFormatter()
	_, err := f.Format(nil)
	if err == nil {
		t.Fatal("expected error for nil records")
	}
}

func TestParquetFormatterExtension(t *testing.T) {
	f := NewParquetFormatter()
	if ext := f.Extension(); ext != ".parquet" {
		t.Errorf("Extension() = %q, want %q", ext, ".parquet")
	}
}
