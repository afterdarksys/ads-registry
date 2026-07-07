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

> **Compatibility layer is load-bearing.** `internal/compat/` (Postfix-style per-client workarounds) and the Docker v2 protocol paths in `internal/api/v2/` exist because a Docker client update once broke a working registry. Treat them as high-caution zones: prefer additive, config-gated changes, and read protocol/TLS settings from the existing `compat` config rather than adding parallel knobs.

## Security posture & hardening (2026-07-07)

A full multi-subsystem security audit was run (auth, v2/storage, format routers, db/policy/tenancy, plumbing). The fixes below are **done and verified** (`go build ./cmd/... ./internal/...` clean, `go test ./internal/...` green, plus behavioral checks). Read this before touching auth, storage paths, or config.

### Fixed this session
- **Path-traversal RCE (all 8 package formats + v2 mount + Go proxy).** Root cause was `filepath.Join(rootDir, key)` with no containment check. Central fix: `storage.SafeJoin` in `internal/storage/path.go`, now used by `internal/storage/local/local.go`. Any client-controlled key (`../../etc/...`) is re-anchored under the root. Tests: `internal/storage/path_test.go`. **S3/OCI backends sanitize keys too but should get the same explicit check — see backlog.**
- **Privilege escalation via web/SSO login.** `oauth.go` and `oidc.go` hardcoded `repository:*:*` for every user. Now both derive claims from the user's real scopes via `auth.ScopesToAccess` (single source of truth, in `internal/auth/token.go`). Tests: `internal/auth/scopes_test.go`.
- **admin/admin bootstrap backdoor** (`internal/auth/handler.go`): now gated behind `REGISTRY_ALLOW_BOOTSTRAP_ADMIN=true` (default off) and fails closed if the user write fails (no more JWT for an unpersisted admin).
- **Developer-mode guard** (`cmd/.../serve.go`): was a weak blocklist (`REGISTRY_ENV != production`). Now fail-closed: requires `REGISTRY_ALLOW_DEV_MODE=true` AND `REGISTRY_ENV ∈ {dev,development,local,test}`, else the server refuses to boot. `developer_mode` disables ALL auth and exposes pprof.
- **TLS minimum version**: HTTPS server now sets `MinVersion` (default 1.2) from `compat.TLSCompatibility.MinTLSVersion`; previously unset (Go would negotiate legacy versions).
- **Token refresh** (`handler.go`): re-validates the user against the DB and re-derives scopes; a deleted/downscoped user can no longer refresh stale privileges forever.
- **OIDC**: added a `sync.Mutex` around the in-flight `states` map (was a concurrent-map panic / DoS) and constant-time nonce comparison.
- **Unbounded body reads (DoS)**: capped in v2 `putUpload` (chunked + monolithic), `management` script upload (5 MiB), and `composer`/`golang` format uploads. (npm/apt/helm already had caps; brew/cocoapods/pypi stream.)
- **SQL identifier injection (tenancy)**: schema names now go through `pq.QuoteIdentifier` in `internal/tenancy/{middleware,provision,tenant}.go`, plus an allowlist-regex gate in `CreateTenant`.
- **CEL policy engine**: `cel.CostLimit` + 2s `ContextEval` deadline (was unbounded → CPU DoS).
- **Starlark scripting** (`internal/scripting/starlark.go`): 30s execution timeout (mirrors `internal/automation`).
- **Pull-through proxy** (`internal/proxy/registry_proxy.go`): fixed a fake-digest stub (was `sha256:<len>`; now real SHA-256), fixed a shared-`resp.Body` corruption bug (io.Pipe + TeeReader), capped upstream manifest reads at 32 MiB.
- **Webhooks**: SSRF guard (`isDisallowedWebhookHost`) rejects loopback/link-local/private/ULA targets.
- **Management API**: `createUser` now enforces password length ≥ 8; the three access-token handlers no longer panic on a bad context key (use `auth.UserContext`/`auth.Claims`).
- **LDAP**: warns loudly when `insecure_skip_verify` is set.
- **Config secrets**: removed the committed OIDC client secret and DB passwords from `config.production.json`/`config.json`; added env overrides (below). Hardened defaults: `max_header_bytes` 10MB→1MB, prod `require_configured_db=true`.

### ⚠️ Secrets to ROTATE (were committed to git history)
These are exposed in prior commits and MUST be rotated regardless of the working-tree scrub:
- OIDC client secret for `ads-registry` (Authentik) — previously in `config.production.json`.
- PostgreSQL passwords `ads_registry_password` (prod) and `ads_user:password` (dev).
Consider `git filter-repo`/BFG to purge history if the repo is shared.

### New config options (`internal/config/config.go`, `ServerConfig`)
| Key | Default | Gates |
|---|---|---|
| `max_upload_size` | 10 GiB | blob/artifact upload body cap (see note in backlog — enforced today via handler constants, not yet threaded from config) |
| `max_manifest_size` | 10 MiB | manifest/metadata body cap (same note) |
| `rate_limit_rpm` | 10000 | per-IP requests/minute (wired into serve.go) |
| `cors_allowed_origins` | `[]` | reserved for management/web CORS allowlist (not yet enforced) |
| `require_configured_db` | false | if true, refuse to fall back to SQLite when Postgres is down (wired) |

### Env-var overrides (prefer these over config files for secrets)
`REGISTRY_DB_DSN`, `REGISTRY_OIDC_CLIENT_SECRET`, `REGISTRY_LDAP_BIND_PASSWORD`, `REGISTRY_VAULT_TOKEN`, `REGISTRY_REDIS_PASSWORD`, `REGISTRY_ES_PASSWORD`, `REGISTRY_TLS_CERT`/`REGISTRY_TLS_KEY`, `REGISTRY_ALLOW_DEV_MODE`, `REGISTRY_ALLOW_BOOTSTRAP_ADMIN`, `REGISTRY_ENV`.

Local dev with auth bypass now requires: `REGISTRY_ALLOW_DEV_MODE=true REGISTRY_ENV=local` **and** `developer_mode:true` in config.

### Remaining security backlog (for Opus — prioritized, NOT yet done)
1. **V2 data-plane tenant isolation (HIGH).** `internal/api/v2/router.go` uses a shared `db.Store` and never filters by tenant; `tenancy.TenantScopedDB` exists but is unused by v2 handlers. Cross-tenant repos/manifests are reachable. Also the format routers all hardcode `namespace := "default"` (no tenant scoping).
2. **Cross-repo blob mount source authz (HIGH).** In `startUpload`, the `mount`/`from` params read blobs from the source repo without checking the caller has pull on it. Path-traversal via these params is already blocked by SafeJoin + needs digest-format validation (`^sha256:[0-9a-f]{64}$`); the *authz* check on the source repo is still missing.
3. **S3/OCI streaming uploads (HIGH, reliability).** `internal/storage/{s3,oci}` buffer whole blobs in memory and the `Appender` re-downloads on every chunk (O(n²) memory + bandwidth). Needs S3 multipart / streaming. Also add an explicit `SafeJoin`-style key check there.
4. **Thread `max_upload_size`/`max_manifest_size` config into handlers.** Today caps are package constants; wire the config values through `v2.NewRouter`/`formats.NewRouter` and enforce `max_manifest_size` on manifest PUTs specifically.
5. **JWT audience claim.** `GenerateToken`/`ParseToken` set/verify no `aud`; add one to prevent cross-service token replay if keys are shared.
6. **Access-token auth is a bcrypt DoS amplifier** (`handler.go`): an `adsr_`-prefixed password triggers a bcrypt compare per token for the user. Add a token-ID/prefix lookup instead of scanning all tokens.
7. **`Compatibility.Enabled` is force-set true** in `config.go` defaults (ignores explicit `false`); needs a `*bool` or "configured" sentinel to respect opt-out.
8. **Debugger dumps `Authorization` headers to `logs/traces/*.log` (0644)** — only in dev mode now, but should redact secrets.
9. **Sync queue silently drops jobs when full** (`internal/sync/manager.go`) — add a metric/dead-letter.
10. **Stray root files** `testQuery.go` + `debug_quota.go` both declare `package main`/`func main()` and break `go build ./...` at the repo root (build `./cmd/...` `./internal/...` instead). Likely deletable debug scratch — confirm with Ryan.

### Verify commands
```bash
go build ./cmd/... ./internal/...     # NOT ./... (stray root files break it — see backlog #10)
go test ./internal/...                # full suite green
go vet ./internal/...
```
