package metrics

import "github.com/prometheus/client_golang/prometheus"

const namespace = "capture"

type Metrics struct {
	RequestsIngested prometheus.Counter
	IngestErrors     prometheus.Counter
	RecordsFlushed   prometheus.Counter
	S3Uploads        *prometheus.CounterVec
	BufferSize       prometheus.Gauge
	S3UploadDuration prometheus.Histogram
	ParquetFileSize  prometheus.Histogram
}

func New() *Metrics {
	m := &Metrics{
		RequestsIngested: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "requests_ingested_total",
			Help:      "Total HTTP requests ingested by the collector.",
		}),

		IngestErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ingest_errors_total",
			Help:      "Total errors while ingesting requests.",
		}),

		RecordsFlushed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "records_flushed_total",
			Help:      "Total HTTP records flushed to parquet files.",
		}),

		S3Uploads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "s3_uploads_total",
			Help:      "Total S3 uploads by result.",
		}, []string{"result"}),

		BufferSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "buffer_size",
			Help:      "Current number of records in the buffer channel.",
		}),

		S3UploadDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "s3_upload_duration_seconds",
			Help:      "Duration of S3 upload operations.",
			Buckets:   prometheus.DefBuckets,
		}),

		ParquetFileSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "parquet_file_size_bytes",
			Help:      "Size of parquet files uploaded to S3.",
			Buckets:   prometheus.ExponentialBuckets(1024, 2, 20),
		}),
	}

	prometheus.MustRegister(
		m.RequestsIngested,
		m.IngestErrors,
		m.RecordsFlushed,
		m.S3Uploads,
		m.BufferSize,
		m.S3UploadDuration,
		m.ParquetFileSize,
	)

	return m
}
