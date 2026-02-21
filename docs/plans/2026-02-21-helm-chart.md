# Helm Chart Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a Helm chart that deploys GoReplay + parquet-s3 middleware with Prometheus monitoring and autoscaling (KEDA or HPA fallback).

**Architecture:** Sidecar pattern — middleware runs alongside GoReplay in the same Pod, piped via stdin/stdout. Chart auto-detects KEDA and Prometheus Operator CRDs via `.Capabilities.APIVersions` to render the right autoscaling/monitoring resources.

**Tech Stack:** Helm 3, Kubernetes, Prometheus, KEDA, AWS SDK Go v2

---

### Task 1: Add S3 endpoint support to Go code

The S3 uploader needs a custom endpoint option for S3-compatible backends (MinIO, etc).

**Files:**
- Modify: `internal/config/config.go:12-15` (Config struct)
- Modify: `internal/config/config.go:25-39` (Load function)
- Modify: `internal/writer/uploader.go:32-51` (NewS3Uploader)

**Step 1: Add `S3Endpoint` to Config struct**

In `internal/config/config.go`, add the field to the struct after `S3Region`:

```go
type Config struct {
	S3Bucket               string
	S3Prefix               string
	S3Region               string
	S3Endpoint             string
	// ... rest unchanged
}
```

**Step 2: Add the CLI flag in Load()**

After the `s3-region` flag line (line 30), add:

```go
flag.StringVar(&cfg.S3Endpoint, "s3-endpoint", envString("GOR_S3_ENDPOINT", ""), "Custom S3 endpoint URL (for MinIO, etc)")
```

**Step 3: Use endpoint in NewS3Uploader**

In `internal/writer/uploader.go`, change `NewS3Uploader` to pass `BaseEndpoint` when configured:

```go
func NewS3Uploader(ctx context.Context, cfg *appconfig.Config, logger *slog.Logger) (*S3Uploader, error) {
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
		}
	})
	uploader := manager.NewUploader(client)

	return &S3Uploader{
		bucket:   cfg.S3Bucket,
		prefix:   cfg.S3Prefix,
		uploader: uploader,
		logger:   logger,
	}, nil
}
```

**Step 4: Verify**

Run: `go build ./... && go vet ./...`
Run: `go test -race ./...`
Run: `./bin/goreplay-parquet-s3 --help 2>&1 | grep s3-endpoint`
Expected: `  -s3-endpoint string    Custom S3 endpoint URL (for MinIO, etc)`

**Step 5: Commit**

```bash
git add internal/config/config.go internal/writer/uploader.go
git commit -m "feat: add --s3-endpoint flag for S3-compatible backends"
```

---

### Task 2: Helm chart scaffolding

Create the chart directory structure and `Chart.yaml` / `values.yaml`.

**Files:**
- Create: `helm/goreplay-parquet-s3/Chart.yaml`
- Create: `helm/goreplay-parquet-s3/values.yaml`
- Create: `helm/goreplay-parquet-s3/templates/_helpers.tpl`

**Step 1: Create Chart.yaml**

```yaml
apiVersion: v2
name: goreplay-parquet-s3
description: GoReplay middleware that captures HTTP traffic and writes Parquet files to S3
type: application
version: 0.1.0
appVersion: "0.1.0"
```

**Step 2: Create values.yaml**

```yaml
replicaCount: 1

image:
  repository: goreplay-parquet-s3
  tag: latest
  pullPolicy: IfNotPresent

goreplay:
  image:
    repository: buger/goreplay
    tag: latest
    pullPolicy: IfNotPresent
  args:
    - "--input-raw"
    - ":8080"
    - "--input-raw-track-response"
    - "--output-http"
    - "http://localhost:8081"
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi

middleware:
  s3Bucket: ""
  s3Prefix: ""
  s3Region: ""
  s3Endpoint: ""
  flushInterval: "5m"
  bufferChanSize: 10000
  maxPendingCorrelations: 50000
  pendingTimeoutMinutes: 10
  maxBodySize: 1048576
  metricsPort: 9200
  logLevel: "info"
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi

aws:
  serviceAccount:
    create: true
    name: ""
    annotations: {}
    # eks.amazonaws.com/role-arn: arn:aws:iam::123456789:role/my-role
  existingSecret: ""

autoscaling:
  enabled: false
  minReplicas: 1
  maxReplicas: 10
  bufferThresholdPercent: 70
  # KEDA-specific
  cooldownPeriod: 300
  pollingInterval: 30
  prometheusAddress: "http://prometheus-server.monitoring.svc:9090"

serviceMonitor:
  enabled: true
  interval: 15s
  labels: {}

podAnnotations: {}
nodeSelector: {}
tolerations: []
affinity: {}
imagePullSecrets: []
```

**Step 3: Create _helpers.tpl**

```
{{/*
Chart name
*/}}
{{- define "goreplay-parquet-s3.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fullname
*/}}
{{- define "goreplay-parquet-s3.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "goreplay-parquet-s3.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "goreplay-parquet-s3.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "goreplay-parquet-s3.selectorLabels" -}}
app.kubernetes.io/name: {{ include "goreplay-parquet-s3.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name
*/}}
{{- define "goreplay-parquet-s3.serviceAccountName" -}}
{{- if .Values.aws.serviceAccount.name }}
{{- .Values.aws.serviceAccount.name }}
{{- else }}
{{- include "goreplay-parquet-s3.fullname" . }}
{{- end }}
{{- end }}

{{/*
Buffer threshold (absolute value from percent)
*/}}
{{- define "goreplay-parquet-s3.bufferThreshold" -}}
{{- mul .Values.autoscaling.bufferThresholdPercent .Values.middleware.bufferChanSize | div 100 }}
{{- end }}
```

**Step 4: Verify**

Run: `helm lint helm/goreplay-parquet-s3/`
Expected: `1 chart(s) linted, 0 chart(s) failed`

**Step 5: Commit**

```bash
git add helm/
git commit -m "feat: add Helm chart scaffolding with values.yaml"
```

---

### Task 3: Deployment + ConfigMap + ServiceAccount

Core Kubernetes resources.

**Files:**
- Create: `helm/goreplay-parquet-s3/templates/configmap.yaml`
- Create: `helm/goreplay-parquet-s3/templates/serviceaccount.yaml`
- Create: `helm/goreplay-parquet-s3/templates/deployment.yaml`

**Step 1: Create configmap.yaml**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "goreplay-parquet-s3.fullname" . }}
  labels:
    {{- include "goreplay-parquet-s3.labels" . | nindent 4 }}
data:
  GOR_S3_BUCKET: {{ .Values.middleware.s3Bucket | quote }}
  GOR_S3_PREFIX: {{ .Values.middleware.s3Prefix | quote }}
  GOR_S3_REGION: {{ .Values.middleware.s3Region | quote }}
  GOR_S3_ENDPOINT: {{ .Values.middleware.s3Endpoint | quote }}
  GOR_FLUSH_INTERVAL: {{ .Values.middleware.flushInterval | quote }}
  GOR_BUFFER_CHAN_SIZE: {{ .Values.middleware.bufferChanSize | quote }}
  GOR_MAX_PENDING: {{ .Values.middleware.maxPendingCorrelations | quote }}
  GOR_PENDING_TIMEOUT: {{ .Values.middleware.pendingTimeoutMinutes | quote }}
  GOR_MAX_BODY_SIZE: {{ .Values.middleware.maxBodySize | quote }}
  GOR_METRICS_PORT: {{ .Values.middleware.metricsPort | quote }}
  GOR_LOG_LEVEL: {{ .Values.middleware.logLevel | quote }}
```

**Step 2: Create serviceaccount.yaml**

```yaml
{{- if .Values.aws.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "goreplay-parquet-s3.serviceAccountName" . }}
  labels:
    {{- include "goreplay-parquet-s3.labels" . | nindent 4 }}
  {{- with .Values.aws.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
```

**Step 3: Create deployment.yaml**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "goreplay-parquet-s3.fullname" . }}
  labels:
    {{- include "goreplay-parquet-s3.labels" . | nindent 4 }}
spec:
  {{- if not .Values.autoscaling.enabled }}
  replicas: {{ .Values.replicaCount }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "goreplay-parquet-s3.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
        {{- if not (.Capabilities.APIVersions.Has "monitoring.coreos.com/v1") }}
        prometheus.io/scrape: "true"
        prometheus.io/port: {{ .Values.middleware.metricsPort | quote }}
        prometheus.io/path: "/metrics"
        {{- end }}
        {{- with .Values.podAnnotations }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      labels:
        {{- include "goreplay-parquet-s3.selectorLabels" . | nindent 8 }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "goreplay-parquet-s3.serviceAccountName" . }}
      containers:
        - name: goreplay
          image: "{{ .Values.goreplay.image.repository }}:{{ .Values.goreplay.image.tag }}"
          imagePullPolicy: {{ .Values.goreplay.image.pullPolicy }}
          args:
            {{- toYaml .Values.goreplay.args | nindent 12 }}
            - "--middleware"
            - "/usr/local/bin/goreplay-parquet-s3"
          resources:
            {{- toYaml .Values.goreplay.resources | nindent 12 }}
          volumeMounts:
            - name: middleware-bin
              mountPath: /usr/local/bin/goreplay-parquet-s3
              subPath: goreplay-parquet-s3
        - name: middleware
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          command: ["sleep", "infinity"]
          envFrom:
            - configMapRef:
                name: {{ include "goreplay-parquet-s3.fullname" . }}
            {{- if .Values.aws.existingSecret }}
            - secretRef:
                name: {{ .Values.aws.existingSecret }}
            {{- end }}
          ports:
            - name: metrics
              containerPort: {{ .Values.middleware.metricsPort }}
              protocol: TCP
          livenessProbe:
            httpGet:
              path: /healthz
              port: metrics
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /healthz
              port: metrics
            initialDelaySeconds: 2
            periodSeconds: 5
          resources:
            {{- toYaml .Values.middleware.resources | nindent 12 }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

> **Note:** The sidecar pattern above uses the GoReplay `--middleware` flag which pipes stdin/stdout to the middleware binary. The middleware container runs as a sidecar sharing the binary via an init container or volume. An alternative simpler approach is to bake the middleware binary into the GoReplay image. The Deployment template can be refined based on the actual deployment strategy — the key resources (ConfigMap, ServiceAccount, probes, envFrom) are the important parts.

**Step 4: Verify**

Run: `helm template test helm/goreplay-parquet-s3/ --set middleware.s3Bucket=test-bucket`
Expected: Valid YAML output with Deployment, ConfigMap, ServiceAccount

**Step 5: Commit**

```bash
git add helm/
git commit -m "feat: add deployment, configmap, and serviceaccount templates"
```

---

### Task 4: Service + ServiceMonitor

Expose metrics port and configure Prometheus scraping.

**Files:**
- Create: `helm/goreplay-parquet-s3/templates/service.yaml`
- Create: `helm/goreplay-parquet-s3/templates/servicemonitor.yaml`

**Step 1: Create service.yaml**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "goreplay-parquet-s3.fullname" . }}
  labels:
    {{- include "goreplay-parquet-s3.labels" . | nindent 4 }}
spec:
  type: ClusterIP
  ports:
    - port: {{ .Values.middleware.metricsPort }}
      targetPort: metrics
      protocol: TCP
      name: metrics
  selector:
    {{- include "goreplay-parquet-s3.selectorLabels" . | nindent 4 }}
```

**Step 2: Create servicemonitor.yaml**

```yaml
{{- if and .Values.serviceMonitor.enabled (.Capabilities.APIVersions.Has "monitoring.coreos.com/v1") }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "goreplay-parquet-s3.fullname" . }}
  labels:
    {{- include "goreplay-parquet-s3.labels" . | nindent 4 }}
    {{- with .Values.serviceMonitor.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  selector:
    matchLabels:
      {{- include "goreplay-parquet-s3.selectorLabels" . | nindent 6 }}
  endpoints:
    - port: metrics
      path: /metrics
      interval: {{ .Values.serviceMonitor.interval }}
{{- end }}
```

**Step 3: Verify**

Run: `helm template test helm/goreplay-parquet-s3/ --set middleware.s3Bucket=test-bucket | grep -A5 "kind: Service"`
Expected: Service and (no ServiceMonitor since Capabilities won't have the CRD in template mode)

**Step 4: Commit**

```bash
git add helm/
git commit -m "feat: add service and servicemonitor templates"
```

---

### Task 5: KEDA ScaledObject + HPA fallback

Autoscaling resources with Capabilities-based detection.

**Files:**
- Create: `helm/goreplay-parquet-s3/templates/scaledobject.yaml`
- Create: `helm/goreplay-parquet-s3/templates/hpa.yaml`

**Step 1: Create scaledobject.yaml**

```yaml
{{- if and .Values.autoscaling.enabled (.Capabilities.APIVersions.Has "keda.sh/v1alpha1") }}
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: {{ include "goreplay-parquet-s3.fullname" . }}
  labels:
    {{- include "goreplay-parquet-s3.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    name: {{ include "goreplay-parquet-s3.fullname" . }}
  minReplicaCount: {{ .Values.autoscaling.minReplicas }}
  maxReplicaCount: {{ .Values.autoscaling.maxReplicas }}
  cooldownPeriod: {{ .Values.autoscaling.cooldownPeriod }}
  pollingInterval: {{ .Values.autoscaling.pollingInterval }}
  triggers:
    - type: prometheus
      metadata:
        serverAddress: {{ .Values.autoscaling.prometheusAddress }}
        query: avg(goreplay_parquet_buffer_size{namespace="{{ .Release.Namespace }}"})
        threshold: {{ include "goreplay-parquet-s3.bufferThreshold" . | quote }}
{{- end }}
```

**Step 2: Create hpa.yaml**

```yaml
{{- if and .Values.autoscaling.enabled (not (.Capabilities.APIVersions.Has "keda.sh/v1alpha1")) }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "goreplay-parquet-s3.fullname" . }}
  labels:
    {{- include "goreplay-parquet-s3.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "goreplay-parquet-s3.fullname" . }}
  minReplicas: {{ .Values.autoscaling.minReplicas }}
  maxReplicas: {{ .Values.autoscaling.maxReplicas }}
  metrics:
    - type: Pods
      pods:
        metric:
          name: goreplay_parquet_buffer_size
        target:
          type: AverageValue
          averageValue: {{ include "goreplay-parquet-s3.bufferThreshold" . | quote }}
{{- end }}
```

**Step 3: Verify**

Run: `helm template test helm/goreplay-parquet-s3/ --set middleware.s3Bucket=test-bucket --set autoscaling.enabled=true`
Expected: HPA rendered (no KEDA in local template mode). Threshold value = 7000.

**Step 4: Commit**

```bash
git add helm/
git commit -m "feat: add KEDA ScaledObject and HPA fallback templates"
```

---

### Task 6: Helm lint + template tests

Final validation of the complete chart.

**Files:**
- Modify: `Makefile` (add helm targets)

**Step 1: Add Makefile targets**

Append to `Makefile`:

```makefile
helm-lint:
	helm lint helm/goreplay-parquet-s3/ --set middleware.s3Bucket=test-bucket

helm-template:
	helm template test helm/goreplay-parquet-s3/ --set middleware.s3Bucket=test-bucket
```

**Step 2: Run full validation**

Run: `make helm-lint`
Expected: `1 chart(s) linted, 0 chart(s) failed`

Run: `make helm-template > /dev/null`
Expected: exit 0, no errors

Run: `make test`
Expected: all Go tests still pass

**Step 3: Commit**

```bash
git add Makefile helm/
git commit -m "feat: add helm-lint and helm-template Makefile targets"
```
