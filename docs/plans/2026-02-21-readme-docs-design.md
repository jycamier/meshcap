# README & Documentation Design

**Date:** 2026-02-21
**Status:** Approved

## Overview

Create a polished README.md as the project's landing page, with a `/doc/` directory containing 4 modular documentation pages. Style: attractive & visual with Mermaid diagrams. Language: English.

## README.md Structure

1. **Header** — Title, tagline, badges (Go, License, CI, Go Report Card)
2. **Overview** — 2-3 sentences + Mermaid data flow diagram
3. **Features** — Bullet list of key capabilities
4. **Quick Start** — docker-compose up in 5 lines
5. **Documentation** — Table of contents linking to `/doc/` pages
6. **Contributing + License** — Short section

## /doc/ Pages

### `doc/architecture.md`
- Full Mermaid diagram (components + data flow)
- Component descriptions (Handler, Flusher, Parquet Writer, S3 Uploader, Metrics)
- S3 key structure and partitioning strategy
- Graceful shutdown flow

### `doc/getting-started.md`
- Prerequisites
- Local development with docker-compose
- Kubernetes deployment with Helm
- Verification steps

### `doc/configuration.md`
- Complete options table (flag, env var, default, description)
- Helm values reference
- Common configuration examples

### `doc/wasm-plugin.md`
- Proxy-wasm lifecycle explanation
- Building the WASM binary
- Istio WasmPlugin CRD deployment
- Plugin configuration
- Troubleshooting
