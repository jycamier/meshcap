# GoReleaser + ko Release Pipeline Design

**Date:** 2026-02-21
**Status:** Approved

## Context

The project (being renamed from `goreplay-parquet-s3-middleware` to **meshcap**) needs a release pipeline that builds and publishes three artifacts to `ghcr.io`:

1. **Collector** container image
2. **WASM plugin** OCI artifact (proxy-wasm spec)
3. **Helm chart** OCI artifact

## Decision

Use **GoReleaser** with **ko** integration to build everything from a single `.goreleaser.yaml` at the project root.

## Architecture

### Collector Image (`kos` section)

- Built via GoReleaser's `kos` integration (ko as library)
- Base image: `cgr.dev/chainguard/static:latest`
- Published to: `ghcr.io/jycamier/meshcap`
- Multi-platform: `linux/amd64`, `linux/arm64`
- Tags: `{{ .Version }}`, `latest`

### WASM Plugin (`hooks.post` section)

- Built via Go native: `GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .`
- Packaged as tar.gz and pushed via `oras` to OCI registry
- Published to: `ghcr.io/jycamier/meshcap/wasm-plugin`
- Media types:
  - Config: `application/vnd.oci.image.config.v1+json`
  - Layer: `application/vnd.oci.image.layer.v1.tar+gzip`
- Tags: `{{ .Version }}`

### Helm Chart (`after.hooks` via helm CLI)

- Packaged with `helm package` and pushed with `helm push` in GoReleaser post-hooks
- Published to: `oci://ghcr.io/jycamier/meshcap/chart`
- Chart version and appVersion set to `{{ .Version }}`

## Release Flow

```
git tag v1.0.0
goreleaser release

  1. kos → build collector → push ghcr.io/jycamier/meshcap:1.0.0
  2. hooks.post → build WASM → oras push ghcr.io/jycamier/meshcap/wasm-plugin:1.0.0
  3. hooks.post → helm package + helm push → oci://ghcr.io/jycamier/meshcap/chart
  4. GitHub release with changelog + checksums
```

## Project Rename

The project will be renamed from `goreplay-parquet-s3-middleware` to `meshcap`:

- Go module: `github.com/jycamier/meshcap`
- Helm chart name: `meshcap`
- All internal references updated accordingly

## CI/CD

GitHub Actions workflow (`.github/workflows/release.yml`) triggered on tag push `v*`:
- Checkout, setup Go, install ko + oras
- Login to ghcr.io via `GITHUB_TOKEN`
- Run `goreleaser release`

## Dependencies

- **ko** >= 0.15 (for `kos` GoReleaser integration)
- **oras** CLI (for WASM OCI push in hooks)
- **GoReleaser** >= 2.0 (for `kos` section + hooks)
- **Go** >= 1.24 (for `-buildmode=c-shared` WASM support)
