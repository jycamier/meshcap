# GoReleaser + ko + Rename to meshcap — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Rename the project to meshcap and add a GoReleaser config that builds collector (ko), WASM plugin (oras), and Helm chart (helm CLI) — all pushed to ghcr.io.

**Architecture:** GoReleaser orchestrates the release. `kos` builds the collector image via ko. Post-hooks handle WASM build+push (oras) and Helm chart package+push (helm CLI). GitHub Actions triggers on tag push.

**Tech Stack:** GoReleaser >= 2.0, ko 0.18, oras CLI, helm CLI, Go 1.24+, GitHub Actions

---

### Task 1: Rename Go module from goreplay-parquet-s3-middleware to meshcap

**Files:**
- Modify: `go.mod` (line 1)
- Modify: `wasm/go.mod` (line 1)
- Modify: `cmd/collector/main.go` (lines 14-18)
- Modify: `internal/collector/handler.go`
- Modify: `internal/collector/handler_test.go`
- Modify: `internal/writer/uploader.go`
- Modify: `internal/writer/parquet.go`
- Modify: `internal/writer/parquet_test.go`
- Modify: `internal/writer/flusher.go`
- Modify: `internal/writer/flusher_test.go`
- Modify: `Makefile` (line 3)

**Step 1: Update go.mod modules**

In `go.mod`:
```
module github.com/jycamier/meshcap
```

In `wasm/go.mod`:
```
module github.com/jycamier/meshcap/wasm
```

**Step 2: Replace all import paths**

Run:
```bash
find . -name '*.go' -not -path './.git/*' -exec sed -i '' \
  's|github.com/jycamier/goreplay-parquet-s3-middleware|github.com/jycamier/meshcap|g' {} +
```

**Step 3: Update Makefile MODULE**

```makefile
MODULE := github.com/jycamier/meshcap
```

**Step 4: Verify build**

```bash
go build ./...
go test ./...
```
Expected: all pass

**Step 5: Commit**

```bash
git add -A
git commit -m "refactor: rename module to github.com/jycamier/meshcap"
```

---

### Task 2: Rename Helm chart from goreplay-parquet-s3 to meshcap

**Files:**
- Rename: `helm/goreplay-parquet-s3/` → `helm/meshcap/`
- Modify: `helm/meshcap/Chart.yaml`
- Modify: `helm/meshcap/values.yaml` (image.repository)
- Modify: `helm/meshcap/templates/_helpers.tpl` (all template names)
- Modify: `helm/meshcap/templates/deployment.yaml`
- Modify: `helm/meshcap/templates/service.yaml`
- Modify: `helm/meshcap/templates/configmap.yaml`
- Modify: `helm/meshcap/templates/serviceaccount.yaml`
- Modify: `helm/meshcap/templates/servicemonitor.yaml`
- Modify: `helm/meshcap/templates/hpa.yaml`
- Modify: `helm/meshcap/templates/scaledobject.yaml`
- Modify: `Makefile` (helm-lint, helm-template targets)

**Step 1: Rename chart directory**

```bash
mv helm/goreplay-parquet-s3 helm/meshcap
```

**Step 2: Update Chart.yaml**

```yaml
apiVersion: v2
name: meshcap
description: HTTP traffic capture collector that writes Parquet files to S3
type: application
version: 0.2.0
appVersion: "0.2.0"
```

**Step 3: Update values.yaml image.repository**

```yaml
image:
  repository: ghcr.io/jycamier/meshcap
```

**Step 4: Rename all template references**

Replace `goreplay-parquet-s3` with `meshcap` in all templates:
```bash
find helm/meshcap/templates -type f -exec sed -i '' \
  's|goreplay-parquet-s3|meshcap|g' {} +
```

**Step 5: Update Makefile Helm targets**

```makefile
helm-lint:
	helm lint helm/meshcap/ --set collector.s3Bucket=test-bucket

helm-template:
	helm template test helm/meshcap/ --set collector.s3Bucket=test-bucket
```

**Step 6: Verify Helm chart**

```bash
helm lint helm/meshcap/ --set collector.s3Bucket=test-bucket
helm template test helm/meshcap/ --set collector.s3Bucket=test-bucket
```
Expected: no errors

**Step 7: Commit**

```bash
git add -A
git commit -m "refactor: rename Helm chart to meshcap"
```

---

### Task 3: Update E2E references

**Files:**
- Modify: `e2e/run.sh` (Helm chart path, deployment name)
- Modify: `e2e/wasmplugin.yaml` (collectorCluster FQDN)

**Step 1: Update e2e/run.sh**

In `deploy_collector()`, change Helm chart path:
```bash
helm upgrade --install collector "${ROOT_DIR}/helm/meshcap/" \
```

Change rollout status deployment name:
```bash
kubectl rollout status deployment/collector-meshcap -n "${NAMESPACE}" --timeout=120s
```

In `verify_results()`, change service name for port-forward:
```bash
kubectl port-forward -n "${NAMESPACE}" svc/collector-meshcap 9200:9200 &
```

**Step 2: Update e2e/wasmplugin.yaml**

Change collectorCluster:
```yaml
collectorCluster: "outbound|8080||collector-meshcap.capture.svc.cluster.local"
```

**Step 3: Commit**

```bash
git add e2e/run.sh e2e/wasmplugin.yaml
git commit -m "refactor: update E2E references to meshcap"
```

---

### Task 4: Update Makefile build-wasm target

**Files:**
- Modify: `Makefile` (build-wasm target)

**Step 1: Fix build-wasm to use Go native instead of tinygo**

```makefile
build-wasm:
	cd wasm && GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .
```

**Step 2: Commit**

```bash
git add Makefile
git commit -m "fix: use Go native for WASM build instead of tinygo"
```

---

### Task 5: Create .goreleaser.yaml

**Files:**
- Create: `.goreleaser.yaml`

**Step 1: Create .goreleaser.yaml**

```yaml
version: 2

project_name: meshcap

before:
  hooks:
    - go mod tidy

builds:
  - id: collector
    main: ./cmd/collector/
    binary: collector
    env:
      - CGO_ENABLED=0
    goos:
      - linux
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w

kos:
  - id: collector
    build: collector
    repositories:
      - ghcr.io/jycamier/meshcap
    tags:
      - "{{ .Version }}"
      - latest
    bare: true
    preserve_import_paths: false
    base_image: cgr.dev/chainguard/static:latest
    platforms:
      - linux/amd64
      - linux/arm64
    sbom: none
    flags:
      - -trimpath
    ldflags:
      - -s -w

archives:
  - id: collector
    builds:
      - collector
    format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"

release:
  github:
    owner: jycamier
    name: meshcap
  draft: false
  prerelease: auto

# Post-release hooks: build+push WASM plugin and Helm chart
after:
  hooks:
    # WASM plugin: build, tar, push via oras
    - cmd: >-
        sh -c '
        cd wasm &&
        GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm . &&
        tar czf plugin.wasm.tar.gz plugin.wasm &&
        oras push ghcr.io/jycamier/meshcap/wasm-plugin:{{ .Version }}
          --config /dev/null:application/vnd.oci.image.config.v1+json
          plugin.wasm.tar.gz:application/vnd.oci.image.layer.v1.tar+gzip
        '
    # Helm chart: package and push via helm CLI
    - cmd: >-
        sh -c '
        helm package helm/meshcap
          --version {{ .Version }}
          --app-version {{ .Version }} &&
        helm push meshcap-{{ .Version }}.tgz oci://ghcr.io/jycamier/meshcap/chart &&
        rm -f meshcap-{{ .Version }}.tgz
        '
```

**Step 2: Verify config (dry run)**

```bash
goreleaser check
```
Expected: config is valid

If goreleaser is not installed:
```bash
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser check
```

**Step 3: Commit**

```bash
git add .goreleaser.yaml
git commit -m "feat: add GoReleaser config with ko, WASM, and Helm chart"
```

---

### Task 6: Create GitHub Actions release workflow

**Files:**
- Create: `.github/workflows/release.yml`

**Step 1: Create release workflow**

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write
  packages: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"

      - name: Install ko
        uses: ko-build/setup-ko@v0.9

      - name: Install oras
        uses: oras-project/setup-oras@v1

      - name: Login to ghcr.io
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          KO_DOCKER_REPO: ghcr.io/jycamier/meshcap
```

**Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add GitHub Actions release workflow"
```

---

### Task 7: Final verification

**Step 1: Run tests**

```bash
go test ./...
```
Expected: all pass

**Step 2: Lint Helm chart**

```bash
helm lint helm/meshcap/ --set collector.s3Bucket=test-bucket
```
Expected: no errors

**Step 3: Build WASM plugin**

```bash
cd wasm && GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm . && cd ..
```
Expected: produces `wasm/plugin.wasm`

**Step 4: Dry-run GoReleaser (if installed)**

```bash
goreleaser release --snapshot --clean --skip=publish
```
Expected: builds collector binaries + ko image (local)

**Step 5: Commit any remaining changes**

```bash
git add -A && git status
```
If clean, done. Otherwise commit.
