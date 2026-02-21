#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="capture-e2e"
REGISTRY_NAME="kind-registry"
REGISTRY_PORT=5050
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
NAMESPACE="capture"
MC_BIN="/opt/homebrew/bin/mc"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()   { echo -e "${GREEN}[E2E]${NC} $*"; }
warn()  { echo -e "${YELLOW}[E2E]${NC} $*"; }
fail()  { echo -e "${RED}[E2E]${NC} $*" >&2; exit 1; }

cleanup() {
  log "Cleaning up..."
  # Kill background port-forwards
  jobs -p 2>/dev/null | xargs -r kill 2>/dev/null || true
  kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
  docker rm -f "${REGISTRY_NAME}" 2>/dev/null || true
  log "Cleanup done"
}

# ─── 1. Prerequisites ─────────────────────────────────────────────────────────
check_prereqs() {
  log "Checking prerequisites..."
  local missing=()
  for cmd in kind kubectl helm docker istioctl oras "${MC_BIN}"; do
    if ! command -v "$cmd" &>/dev/null; then
      missing+=("$cmd")
    fi
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    fail "Missing required tools: ${missing[*]}"
  fi
  log "All prerequisites found"
}

# ─── 2. Kind Cluster ──────────────────────────────────────────────────────────
create_cluster() {
  if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    log "Cluster '${CLUSTER_NAME}' already exists, reusing"
    return
  fi
  log "Creating Kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --name "${CLUSTER_NAME}" --config "${SCRIPT_DIR}/kind-config.yaml"
  log "Kind cluster created"
}

# ─── 3. OCI Registry ──────────────────────────────────────────────────────────
start_registry() {
  if docker inspect "${REGISTRY_NAME}" &>/dev/null; then
    log "Registry '${REGISTRY_NAME}' already running, reusing"
  else
    log "Starting OCI registry..."
    docker run -d --restart=always -p "${REGISTRY_PORT}:5000" \
      --name "${REGISTRY_NAME}" --network kind registry:2
  fi

  # Connect registry to kind network if not already connected
  if ! docker network inspect kind | grep -q "${REGISTRY_NAME}"; then
    docker network connect kind "${REGISTRY_NAME}" 2>/dev/null || true
  fi

  log "OCI registry ready at localhost:${REGISTRY_PORT}"
}

# ─── 4. Install Istio ─────────────────────────────────────────────────────────
install_istio() {
  if kubectl get namespace istio-system &>/dev/null; then
    log "Istio already installed, skipping"
  else
    log "Installing Istio (minimal profile)..."
    istioctl install --set profile=minimal -y
  fi

  # Enable sidecar injection on default namespace
  kubectl label namespace default istio-injection=enabled --overwrite

  # Allow permissive mTLS so non-mesh pods can reach mesh services
  kubectl apply -f - <<'EOF'
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: default
  namespace: default
spec:
  mtls:
    mode: PERMISSIVE
EOF

  log "Istio installed, sidecar injection enabled on default namespace"
}

# ─── 5. Deploy MinIO ──────────────────────────────────────────────────────────
deploy_minio() {
  log "Deploying MinIO..."
  kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f "${SCRIPT_DIR}/minio.yaml"

  log "Waiting for MinIO to be ready..."
  kubectl rollout status deployment/minio -n "${NAMESPACE}" --timeout=120s

  log "Waiting for bucket creation job..."
  kubectl wait --for=condition=complete job/create-bucket -n "${NAMESPACE}" --timeout=120s
  log "MinIO ready, bucket 'capture' created"
}

# ─── 6. Build & Load Collector ────────────────────────────────────────────────
build_and_load_collector() {
  log "Building collector image..."
  docker build -t collector:e2e "${ROOT_DIR}"

  log "Loading collector image into Kind..."
  kind load docker-image collector:e2e --name "${CLUSTER_NAME}"
  log "Collector image loaded"
}

# ─── 7. Deploy Collector via Helm ─────────────────────────────────────────────
deploy_collector() {
  log "Creating MinIO credentials secret..."
  kubectl create secret generic minio-creds -n "${NAMESPACE}" \
    --from-literal=AWS_ACCESS_KEY_ID=minioadmin \
    --from-literal=AWS_SECRET_ACCESS_KEY=minioadmin \
    --dry-run=client -o yaml | kubectl apply -f -

  log "Deploying collector via Helm..."
  helm upgrade --install collector "${ROOT_DIR}/helm/meshcap/" \
    --namespace "${NAMESPACE}" \
    --set image.repository=collector \
    --set image.tag=e2e \
    --set image.pullPolicy=Never \
    --set collector.s3Bucket=capture \
    --set collector.s3Endpoint=http://minio.capture.svc:9000 \
    --set collector.s3Region=us-east-1 \
    --set collector.flushInterval=10s \
    --set collector.logLevel=debug \
    --set aws.existingSecret=minio-creds \
    --set serviceMonitor.enabled=false

  log "Waiting for collector to be ready..."
  kubectl rollout status deployment/collector-meshcap -n "${NAMESPACE}" --timeout=120s
  log "Collector deployed and ready"
}

# ─── 8. Deploy httpbin ────────────────────────────────────────────────────────
deploy_httpbin() {
  log "Deploying httpbin..."
  kubectl apply -f "${SCRIPT_DIR}/httpbin.yaml"

  log "Waiting for httpbin to be ready..."
  kubectl rollout status deployment/httpbin -n default --timeout=180s
  log "httpbin deployed and ready"
}

# ─── 9. Build & Push WASM Plugin ──────────────────────────────────────────────
build_and_push_wasm() {
  log "Building WASM plugin (Go native)..."
  (cd "${ROOT_DIR}/wasm" && GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .)

  log "Pushing WASM plugin to OCI registry..."
  (cd "${ROOT_DIR}/wasm" && \
    tar czf plugin.wasm.tar.gz plugin.wasm && \
    oras push "localhost:${REGISTRY_PORT}/capture-wasm:e2e" \
      --config /dev/null:application/vnd.oci.image.config.v1+json \
      "plugin.wasm.tar.gz:application/vnd.oci.image.layer.v1.tar+gzip")

  log "WASM plugin pushed"
}

# ─── 10. Apply WasmPlugin CRD ─────────────────────────────────────────────────
apply_wasm_plugin() {
  log "Applying WasmPlugin CRD..."
  kubectl apply -f "${SCRIPT_DIR}/wasmplugin.yaml"

  # Restart httpbin to ensure Envoy re-fetches the WASM module
  kubectl rollout restart deployment/httpbin -n default
  kubectl rollout status deployment/httpbin -n default --timeout=180s

  # Give Envoy time to fetch and load the plugin
  log "Waiting 15s for Envoy to load the WASM plugin..."
  sleep 15
  log "WasmPlugin applied"
}

# ─── 11. Send Traffic ─────────────────────────────────────────────────────────
send_traffic() {
  log "Sending test traffic (10 requests)..."
  kubectl run curl-test --image=curlimages/curl --rm -i --restart=Never --timeout=120s \
    --overrides='{"metadata":{"annotations":{"sidecar.istio.io/inject":"false"}}}' -- \
    sh -c 'for i in $(seq 1 10); do
      echo "Request $i"
      curl -s -X POST http://httpbin.default.svc/post \
        -d "{\"test\":$i}" \
        -H "Content-Type: application/json"
      sleep 1
    done'
  log "Traffic sent"
}

# ─── 12. Verify Results ───────────────────────────────────────────────────────
verify_results() {
  log "Waiting for flush interval (15s)..."
  sleep 15

  log "Setting up MinIO port-forward..."
  # Kill any leftover port-forward on 9000
  lsof -ti :9000 2>/dev/null | xargs kill 2>/dev/null || true
  kubectl port-forward -n "${NAMESPACE}" svc/minio 9000:9000 &
  local pf_pid=$!
  sleep 3

  "${MC_BIN}" alias set e2e http://localhost:9000 minioadmin minioadmin

  log "Listing Parquet files in MinIO..."
  local file_count
  file_count=$("${MC_BIN}" ls --recursive e2e/capture/ 2>/dev/null | grep -c '\.parquet' || true)
  file_count="${file_count:-0}"

  kill "${pf_pid}" 2>/dev/null || true

  if [[ "${file_count}" -lt 1 ]]; then
    fail "No Parquet files found in MinIO bucket 'capture' (expected >= 1)"
  fi
  log "Found ${file_count} Parquet file(s) in MinIO"

  # Check collector metrics
  log "Checking collector metrics..."
  kubectl port-forward -n "${NAMESPACE}" svc/collector-meshcap 9200:9200 &
  local metrics_pf_pid=$!
  sleep 3

  local ingested
  ingested=$(curl -s http://localhost:9200/metrics | grep 'capture_requests_ingested_total' | grep -v '^#' | awk '{print $2}' || echo "0")

  kill "${metrics_pf_pid}" 2>/dev/null || true

  if [[ -z "${ingested}" ]] || [[ "${ingested}" == "0" ]]; then
    warn "capture_requests_ingested_total = ${ingested:-0} (may need more time)"
  else
    log "capture_requests_ingested_total = ${ingested}"
  fi

  log "E2E verification PASSED"
}

# ─── Main ─────────────────────────────────────────────────────────────────────
main() {
  local do_cleanup=true

  # Parse args
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --no-cleanup) do_cleanup=false; shift ;;
      --cleanup-only) cleanup; exit 0 ;;
      *) fail "Unknown argument: $1" ;;
    esac
  done

  if [[ "${do_cleanup}" == true ]]; then
    trap cleanup EXIT
  fi

  check_prereqs
  create_cluster
  start_registry
  install_istio
  deploy_minio
  build_and_load_collector
  deploy_collector
  deploy_httpbin
  build_and_push_wasm
  apply_wasm_plugin
  send_traffic
  verify_results

  log "All E2E tests passed!"
}

main "$@"
