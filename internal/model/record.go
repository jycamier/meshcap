package model

type HTTPRequest struct {
	RequestID   string `parquet:"request_id,zstd" json:"request_id"`
	TraceID     string `parquet:"trace_id,zstd" json:"trace_id"`
	CapturedAt  string `parquet:"captured_at,zstd" json:"captured_at"`
	TimestampNs int64  `parquet:"timestamp_ns" json:"timestamp_ns"`
	ReqMethod   string `parquet:"req_method,zstd" json:"req_method"`
	ReqPath     string `parquet:"req_path,zstd" json:"req_path"`
	ReqHost     string `parquet:"req_host,zstd" json:"req_host"`
	ReqVersion  string `parquet:"req_http_version,zstd" json:"req_http_version"`
	ReqHeaders  string `parquet:"req_headers,zstd" json:"req_headers"`
	ReqBody     []byte `parquet:"req_body,zstd,optional" json:"req_body,omitempty"`
	ReqBodySize int64  `parquet:"req_body_size" json:"req_body_size"`
	ClientIP    string `parquet:"client_ip,zstd" json:"client_ip"`
	SourcePod   string `parquet:"source_pod,zstd" json:"source_pod"`
}
