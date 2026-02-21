# Getting Started

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) and Docker Compose

For Kubernetes deployment, you'll also need:
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm 3](https://helm.sh/docs/intro/install/)
- [Istio](https://istio.io/latest/docs/setup/install/) (for WASM plugin)

## Local Development with Docker Compose

The quickest way to run meshcap locally:

```bash
docker compose up
```

This starts:
- **meshcap collector** on port 8080 (ingest) and 9200 (metrics)
- **MinIO** as the S3 backend (console on port 9001)
- A bucket creation job that creates the `capture` bucket

### Send a Test Request

```bash
curl -X POST http://localhost:8080/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "request_id": "test-001",
    "captured_at": "2025-06-15T10:30:00Z",
    "timestamp_ns": 1750000000000000000,
    "req_method": "GET",
    "req_path": "/api/users",
    "req_host": "myapp.example.com",
    "req_http_version": "HTTP/1.1",
    "req_headers": "{\"content-type\":\"application/json\"}",
    "client_ip": "10.0.0.1",
    "source_pod": "myapp-6f7b8c9d-abc12"
  }'
```

A `202 Accepted` response confirms the record was buffered. It will be flushed to MinIO after the flush interval (30s in docker-compose).

### Verify in MinIO

Open [http://localhost:9001](http://localhost:9001) and log in with `minioadmin` / `minioadmin`. Browse the `capture` bucket to find Parquet files organized by host and date.

### Check Metrics

```bash
curl http://localhost:9200/metrics | grep capture_
```

## Building from Source

```bash
# Build the collector binary
make build

# Build the WASM plugin
make build-wasm

# Run tests
make test

# Build Docker image
make docker
```

## Kubernetes Deployment with Helm

### 1. Add the OCI Helm Repository

```bash
helm pull oci://ghcr.io/jycamier/meshcap/chart/meshcap --version <version>
```

Or install directly from the local chart:

```bash
helm install meshcap ./helm/meshcap \
  --namespace capture --create-namespace \
  --set collector.s3Bucket=my-capture-bucket \
  --set collector.s3Region=eu-west-1
```

### 2. Configure S3 Access

**Option A: IAM Roles for Service Accounts (IRSA) — recommended for AWS**

```bash
helm install meshcap ./helm/meshcap \
  --namespace capture --create-namespace \
  --set collector.s3Bucket=my-capture-bucket \
  --set collector.s3Region=eu-west-1 \
  --set aws.serviceAccount.annotations."eks\.amazonaws\.com/role-arn"=arn:aws:iam::123456789:role/meshcap-role
```

**Option B: Existing Kubernetes Secret**

```bash
kubectl create secret generic s3-creds -n capture \
  --from-literal=AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE \
  --from-literal=AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

helm install meshcap ./helm/meshcap \
  --namespace capture --create-namespace \
  --set collector.s3Bucket=my-capture-bucket \
  --set collector.s3Region=eu-west-1 \
  --set aws.existingSecret=s3-creds
```

**Option C: MinIO (development/testing)**

```bash
helm install meshcap ./helm/meshcap \
  --namespace capture --create-namespace \
  --set collector.s3Bucket=capture \
  --set collector.s3Endpoint=http://minio.capture.svc:9000 \
  --set collector.s3Region=us-east-1 \
  --set aws.existingSecret=minio-creds
```

### 3. Verify the Deployment

```bash
# Check the collector is running
kubectl get pods -n capture

# Check logs
kubectl logs -n capture deployment/meshcap-meshcap

# Check metrics
kubectl port-forward -n capture svc/meshcap-meshcap 9200:9200
curl http://localhost:9200/healthz
```

### 4. Deploy the WASM Plugin

Once the collector is running, deploy the Envoy WASM plugin to start capturing traffic. See the [WASM Plugin Guide](wasm-plugin.md) for detailed instructions.

## End-to-End Testing

A comprehensive E2E test script sets up a full environment with Kind, Istio, MinIO, the collector, and the WASM plugin:

```bash
# Prerequisites: kind, kubectl, helm, istioctl, docker, oras, mc (MinIO client)
./e2e/run.sh
```

This creates a disposable Kind cluster, deploys all components, sends test traffic, and verifies that Parquet files appear in MinIO. The cluster is cleaned up automatically on exit (use `--no-cleanup` to keep it).
