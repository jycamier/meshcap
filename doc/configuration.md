# Configuration Reference

meshcap follows the [12-factor app](https://12factor.net/config) methodology. Every setting can be configured via CLI flags or environment variables. In Kubernetes, the Helm chart maps values to environment variables via a ConfigMap.

## Collector Options

| CLI Flag | Environment Variable | Default | Description |
|----------|---------------------|---------|-------------|
| `-output-format` | `GOR_OUTPUT_FORMAT` | `parquet` | Output format (`parquet`, `har`) |
| `-storage-backend` | `GOR_STORAGE_BACKEND` | `s3` | Storage backend (`s3`, `azure`) |
| `-s3-bucket` | `GOR_S3_BUCKET` | *(required when backend=s3)* | S3 bucket name |
| `-s3-prefix` | `GOR_S3_PREFIX` | `""` | S3 key prefix |
| `-s3-region` | `GOR_S3_REGION` | `""` | AWS region |
| `-s3-endpoint` | `GOR_S3_ENDPOINT` | `""` | Custom S3 endpoint URL (for MinIO, etc.) |
| `-azure-account` | `GOR_AZURE_ACCOUNT` | `""` | Azure Storage account name (required when backend=azure) |
| `-azure-container` | `GOR_AZURE_CONTAINER` | `""` | Azure Blob container name (required when backend=azure) |
| `-flush-interval` | `GOR_FLUSH_INTERVAL` | `5m` | Flush interval (e.g. `30s`, `5m`) |
| `-buffer-chan-size` | `GOR_BUFFER_CHAN_SIZE` | `10000` | Buffer channel capacity |
| `-max-body-size` | `GOR_MAX_BODY_SIZE` | `1048576` | Max request body size in bytes (1 MB) |
| `-collector-port` | `GOR_COLLECTOR_PORT` | `8080` | HTTP ingest port |
| `-metrics-port` | `GOR_METRICS_PORT` | `9200` | Prometheus metrics port |
| `-log-level` | `GOR_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |

Environment variables take precedence over defaults but are overridden by CLI flags.

## Helm Values

The Helm chart (`helm/meshcap/`) exposes the following values:

### Image

| Value | Default | Description |
|-------|---------|-------------|
| `image.repository` | `ghcr.io/jycamier/meshcap` | Container image |
| `image.tag` | `latest` | Image tag |
| `image.pullPolicy` | `IfNotPresent` | Pull policy |

### Collector

| Value | Default | Description |
|-------|---------|-------------|
| `collector.port` | `8080` | Ingest port |
| `collector.outputFormat` | `parquet` | Output format (`parquet`, `har`) |
| `collector.storageBackend` | `s3` | Storage backend (`s3`, `azure`) |
| `collector.s3Bucket` | `""` | S3 bucket name (required when backend=s3) |
| `collector.s3Prefix` | `""` | S3 key prefix |
| `collector.s3Region` | `""` | AWS region |
| `collector.s3Endpoint` | `""` | Custom S3 endpoint |
| `collector.azureAccount` | `""` | Azure Storage account (required when backend=azure) |
| `collector.azureContainer` | `""` | Azure Blob container (required when backend=azure) |
| `collector.flushInterval` | `5m` | Flush interval |
| `collector.bufferChanSize` | `10000` | Buffer channel size |
| `collector.maxBodySize` | `1048576` | Max body size (bytes) |
| `collector.metricsPort` | `9200` | Metrics port |
| `collector.logLevel` | `info` | Log level |

### Resources

| Value | Default | Description |
|-------|---------|-------------|
| `resources.requests.cpu` | `200m` | CPU request |
| `resources.requests.memory` | `256Mi` | Memory request |
| `resources.limits.cpu` | `1` | CPU limit |
| `resources.limits.memory` | `512Mi` | Memory limit |

### AWS / S3 Authentication

| Value | Default | Description |
|-------|---------|-------------|
| `aws.serviceAccount.create` | `true` | Create a ServiceAccount |
| `aws.serviceAccount.name` | `""` | ServiceAccount name (auto-generated if empty) |
| `aws.serviceAccount.annotations` | `{}` | Annotations (e.g. IRSA role ARN) |
| `aws.existingSecret` | `""` | Name of existing Secret with `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` |

### Autoscaling

| Value | Default | Description |
|-------|---------|-------------|
| `autoscaling.enabled` | `false` | Enable KEDA autoscaling |
| `autoscaling.minReplicas` | `1` | Minimum replicas |
| `autoscaling.maxReplicas` | `10` | Maximum replicas |
| `autoscaling.bufferThresholdPercent` | `70` | Buffer usage threshold for scaling |
| `autoscaling.cooldownPeriod` | `300` | Cooldown period (seconds) |
| `autoscaling.pollingInterval` | `30` | Polling interval (seconds) |
| `autoscaling.prometheusAddress` | `http://prometheus-server.monitoring.svc:9090` | Prometheus server URL |

### Monitoring

| Value | Default | Description |
|-------|---------|-------------|
| `serviceMonitor.enabled` | `true` | Create Prometheus ServiceMonitor |
| `serviceMonitor.interval` | `15s` | Scrape interval |
| `serviceMonitor.labels` | `{}` | Additional labels |

### Scheduling

| Value | Default | Description |
|-------|---------|-------------|
| `replicaCount` | `1` | Number of replicas |
| `podAnnotations` | `{}` | Pod annotations |
| `nodeSelector` | `{}` | Node selector |
| `tolerations` | `[]` | Tolerations |
| `affinity` | `{}` | Affinity rules |
| `imagePullSecrets` | `[]` | Image pull secrets |

## Common Configurations

### Development with MinIO

```bash
./bin/collector \
  -s3-bucket capture \
  -s3-endpoint http://localhost:9000 \
  -s3-region us-east-1 \
  -flush-interval 30s \
  -log-level debug
```

### Production on AWS (Parquet)

```bash
./bin/collector \
  -output-format parquet \
  -storage-backend s3 \
  -s3-bucket my-traffic-data \
  -s3-prefix production \
  -s3-region eu-west-1 \
  -flush-interval 5m \
  -buffer-chan-size 50000
```

### Azure Blob Storage

```bash
./bin/collector \
  -output-format parquet \
  -storage-backend azure \
  -azure-account mystorageaccount \
  -azure-container captures \
  -flush-interval 5m
```

Authentication uses `DefaultAzureCredential`, which tries (in order): environment variables, Managed Identity, Azure CLI.

### HAR Format Output

```bash
./bin/collector \
  -output-format har \
  -storage-backend s3 \
  -s3-bucket my-traffic-data \
  -s3-prefix har-captures \
  -s3-region eu-west-1
```

### High-Throughput Tuning

For high-traffic environments, consider:

```yaml
# Helm values
collector:
  bufferChanSize: 100000
  flushInterval: "1m"
  maxBodySize: 524288  # 512 KB to reduce memory pressure

resources:
  requests:
    cpu: "1"
    memory: 1Gi
  limits:
    cpu: "4"
    memory: 2Gi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 20
  bufferThresholdPercent: 60
```
