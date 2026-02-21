# Helm Chart Design — goreplay-parquet-s3

## Context

Deploy GoReplay + parquet-s3 middleware as a Kubernetes workload with Prometheus metrics, autoscaling (KEDA or HPA), and S3-compatible storage support.

## Architecture

The middleware runs as a **sidecar container** in the same Pod as GoReplay, connected via `--middleware` stdin/stdout pipe.

## Chart Templates

| File | Conditional | Purpose |
|------|------------|---------|
| `deployment.yaml` | — | GoReplay + sidecar middleware |
| `configmap.yaml` | — | Env vars GOR_* for middleware config |
| `service.yaml` | — | Expose metrics port (9200) |
| `serviceaccount.yaml` | — | With optional IRSA/Pod Identity annotations |
| `servicemonitor.yaml` | `monitoring.coreos.com/v1` present | Prometheus Operator scraping |
| `scaledobject.yaml` | `keda.sh/v1alpha1` present | KEDA autoscaling |
| `hpa.yaml` | KEDA NOT present | HPA native fallback |

## Autoscaling

- **Metric**: `goreplay_parquet_buffer_size` (gauge)
- **Default threshold**: 70% of `bufferChanSize` (7000 on default 10000)
- **KEDA**: ScaledObject with prometheus trigger, cooldown 300s
- **HPA fallback**: Pods metric type, requires prometheus-adapter

## AWS Authentication

Three methods supported via values.yaml:
1. **IRSA**: `eks.amazonaws.com/role-arn` annotation on ServiceAccount
2. **Pod Identity**: `eks.amazonaws.com/pod-identity-association` annotation
3. **Secret**: `envFrom` an existing Kubernetes Secret

## S3-Compatible Storage

Custom endpoint support (`--s3-endpoint` / `GOR_S3_ENDPOINT`) for MinIO, Localstack, or any S3-compatible proxy. Requires adding `S3Endpoint` to Go config + `BaseEndpoint` on S3 client.

## Monitoring Fallback

If Prometheus Operator CRD not detected: Pod annotations (`prometheus.io/scrape`, `prometheus.io/port`, `prometheus.io/path`).
