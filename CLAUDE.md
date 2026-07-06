# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

ADS Registry — an artifact registry for containers, k8s, and other artifacts. It is a production-oriented, OCI-compliant Docker container registry (Docker Registry API v2) extended into a universal package repository supporting 8 additional artifact formats (apt, brew, cocoapods, composer, golang, helm, npm, pypi). Features include JWT auth (RSA-256), CEL policy enforcement, Trivy vulnerability scanning, Cosign signature verification, Starlark event automation, multi-tenancy, Prometheus metrics, and HashiCorp Vault integration.

## Commands

```bash
# Build all binaries into ./build (ads-registry, adsradm, credential-provider, migrate-sqlite-to-postgres)
./build.sh            # or: ./build.sh clean | ./build.sh rebuild

# Build just the server
go build -o ads-registry ./cmd/ads-registry

# Run
./ads-registry serve
./ads-registry create-user admin --scopes="*"

# Tests
go test ./...                    # all
go test -cover ./...             # with coverage
go test ./internal/auth/...      # single package

# Docker
docker build -t ads-registry:latest .

# Web dashboard (React + Vite + TypeScript, in web/)
cd web && npm run dev    # dev server
cd web && npm run build  # tsc -b && vite build
cd web && npm run lint   # eslint
```

## Architecture

- **Entry points** (`cmd/`): `ads-registry` (the server), `adsradm` and `artifactadm` (admin CLIs), `credential-provider` (k8s credential provider), `migrate-sqlite-to-postgres`.
- **API layer** (`internal/api/`): `v2/` implements the Docker Registry API v2 (manifests, blobs, uploads); `formats/` has one sub-router per package ecosystem (apt, npm, pypi, helm, etc.); `artifacts/` is the universal artifact CRUD/stats API; plus `management/`, `auth/`, `tenancy/`, and shared `middleware/`. Routing uses go-chi.
- **Storage** (`internal/storage/`): pluggable content-addressable blob storage behind `provider.go` — `local/`, `s3/`, `oci/`, and in-memory backends. Layers are deduplicated by SHA256 digest.
- **Metadata DB** (`internal/db/`): SQLite for dev, PostgreSQL for production (JSONB metadata for universal artifacts). Schema lives in `migrations/` (e.g. `017_artifact_metadata.sql` adds the `universal_artifacts*` tables).
- **Push/pull data flow**: client → auth middleware (JWT/OIDC/LDAP) → v2 or format router → policy engine (CEL admission rules, `internal/policy`) → storage provider + DB metadata → async workers (`internal/queue`) for scanning (`internal/scanner`, Trivy), webhooks, and Starlark automation (`internal/automation`, `internal/scripting`).
- **Enterprise plumbing**: `internal/tenancy` (namespace isolation), `internal/sync` (peer sync — see PEER_SYNC_IMPLEMENTATION.md), `internal/upstreams`/`internal/proxy` (pull-through caching), `internal/vault`, `internal/metrics`, `internal/health` (k8s probes).
- **Config**: `config.json` (dev) / `config.production.json`; examples in `config-examples/`. Server listens on port 5005 by default.

Extensive operational docs live in `docs/` and the top-level `QUICK_START.md` / `QUICK_REFERENCE.md` / `TROUBLESHOOTING_QUICK_REF.md`.
