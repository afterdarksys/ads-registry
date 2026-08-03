# Experimental Features and Native SBOM Generation Specification

## Metadata

- **Status:** In Review
- **Date:** 2026-08-03
- **Author:** ADS Registry Engineering
- **Reviewer:** Project maintainer
- **Target:** First experimental-features release

## Context

ADS Registry has individual compatibility switches, but it does not have a single contract for high-risk, incomplete, or evolving capabilities. It already stores OCI manifests and blobs, records artifact subject metadata, exposes the OCI referrers API, and can inspect repository-local image layers. Those facilities make native Software Bill of Materials (SBOM) generation a practical first experimental capability.

This work introduces a disabled-by-default experimental configuration namespace and implements asynchronous CycloneDX SBOM generation for pushed OCI image manifests. Generated SBOMs are immutable OCI artifacts referring to the exact source manifest digest. Configuration placeholders are also introduced for universal format translation and cross-repository content deduplication, but their execution engines are intentionally excluded from this release.

The feature is additive. Existing installations, routes, pushes, pulls, and stored content MUST behave exactly as before unless both the global experimental switch and the SBOM-specific switch are enabled.

## Functional Requirements

### Experimental configuration namespace

- FR-1: The service provides a disabled-by-default experimental configuration namespace with validated SBOM, translation, and deduplication switches.

The service MUST accept the following additive top-level configuration shape:

```json
{
  "experimental": {
    "enabled": false,
    "sbom_generation": {
      "enabled": false,
      "formats": ["cyclonedx-json"],
      "mode": "asynchronous",
      "failure_policy": "warn",
      "store_as_oci_referrer": true,
      "sign_results": false,
      "timeout_seconds": 600,
      "max_concurrent_jobs": 4,
      "queue_capacity": 256,
      "max_attempts": 3
    },
    "format_translation": {
      "enabled": false
    },
    "content_deduplication": {
      "enabled": false
    }
  }
}
```

All experimental capabilities MUST default to disabled when the section or any `enabled` field is absent. A capability MUST run only when `experimental.enabled` and its own `enabled` field are both true. The service MUST apply the documented defaults to omitted non-boolean fields without converting an explicit `false` into `true`.

For this release, configuration validation MUST accept only:

- `formats`: exactly one entry, `cyclonedx-json`;
- `mode`: `asynchronous`;
- `failure_policy`: `warn` or `retry`;
- `timeout_seconds`: 1 through 3600;
- `max_concurrent_jobs`: 1 through 32;
- `queue_capacity`: 1 through 10000;
- `max_attempts`: 1 through 10.

Invalid enabled SBOM configuration MUST stop startup with a field-specific error. Unsupported translation or deduplication settings MUST stop startup if their feature is enabled, because those engines do not exist yet.

### Startup reporting

- FR-2: The service reports the effective experimental state safely at startup.

At startup, the service MUST log the effective state of the global experimental switch and each known experimental capability. It MUST emit a prominent warning for every enabled experimental capability. It MUST NOT log secrets or the complete raw configuration.

### Eligible manifest detection

- FR-3: Successful eligible manifest pushes trigger asynchronous SBOM jobs by immutable digest.

After an OCI or Docker image manifest has been stored successfully, the service MUST enqueue SBOM generation using the immutable repository and manifest digest. The push response MUST NOT wait for SBOM generation.

The service MUST NOT automatically generate an SBOM for an image index, manifest list, Helm chart, signature, attestation, or another artifact manifest. Re-pushing a tag that resolves to a new digest MUST create work for the new digest and MUST NOT alter the SBOM for the old digest.

### Durable and idempotent jobs

- FR-4: SBOM work is durable, recoverable, bounded, and idempotent.

The service MUST persist an SBOM job before acknowledging it as queued. A job identity MUST include repository, source digest, requested format, generator version, and an options hash. Enqueuing an existing active or completed identity MUST be idempotent.

Jobs MUST use the states `pending`, `running`, `completed`, or `failed`. A job left `running` by process termination MUST become eligible for recovery after restart. `failure_policy=warn` MUST perform one attempt. `failure_policy=retry` MUST attempt no more than `max_attempts`, using bounded backoff.

If the configured in-process queue is full, the durable job MUST remain `pending`; the manifest push MUST still succeed and the worker dispatcher MUST pick the job up later.

### Safe CycloneDX generation

- FR-5: The service generates a safe, evidence-based CycloneDX SBOM without executing image content.

The generator MUST produce CycloneDX JSON with `bomFormat` set to `CycloneDX`, a supported `specVersion`, a unique serial number, generator metadata, the source image identity, and discovered components. Output MUST be based only on content stored for the source repository and digest.

The first release MUST identify packages recorded in Alpine APK and Debian/Ubuntu dpkg package databases found in image layers. Package records MUST include name and version when present and SHOULD include architecture, package URL, license, and supplier when that information exists in the package database. Unsupported package databases or malformed individual package records MUST be reported as warnings and MUST NOT cause invented components.

The generator MUST apply overlay filesystem semantics, including whiteouts, while reading layers. It MUST NOT execute image content, invoke package hooks, contact external services, follow unsafe paths, or read blobs outside the source repository.

### Immutable OCI referrer storage

- FR-6: Completed SBOMs are stored as immutable, content-addressed OCI referrers.

On successful generation, the service MUST store the CycloneDX document as a content-addressed blob and publish an OCI artifact manifest whose `subject.digest` equals the source image manifest digest. The artifact type MUST be `application/vnd.cyclonedx+json` and the SBOM blob media type MUST be `application/vnd.cyclonedx+json`.

The generated artifact MUST be discoverable through the existing OCI referrers endpoint. Its manifest and blobs MUST pass through the same storage and database integrity rules as client-pushed content. Reprocessing the same job identity MUST resolve to the same logical result without creating duplicate referrer entries.

`store_as_oci_referrer=false` and `sign_results=true` MUST be rejected while SBOM generation is enabled in this release, because no alternative result store or signing implementation is included.

### Job status API

- FR-7: Authorized clients can inspect and manually enqueue SBOM jobs through v2 endpoints.

The service MUST expose:

- `GET /v2/{repository}/sboms/{digest}/status`
- `POST /v2/{repository}/sboms/{digest}/generate`

`GET` MUST require pull authorization for the repository and return the latest matching job state. `POST` MUST require push authorization, verify that the digest is an eligible manifest in the repository, idempotently enqueue a job, and return HTTP 202. Neither endpoint MAY expose internal filesystem paths or unsanitized errors.

The status response MUST use this shape:

```json
{
  "repository": "team/app",
  "source_digest": "sha256:...",
  "format": "cyclonedx-json",
  "status": "completed",
  "attempts": 1,
  "output_manifest_digest": "sha256:...",
  "last_error": "",
  "created_at": "2026-08-03T12:00:00Z",
  "updated_at": "2026-08-03T12:00:01Z"
}
```

The API MUST return the registry's standard JSON error envelope with HTTP 400 for malformed digests, 404 for an unknown eligible source or absent job on `GET`, 409 for an ineligible manifest, and 503 when experimental SBOM generation is disabled.

### Placeholder capability flags

- FR-8: Translation and deduplication flags are recognized but fail closed until implemented.

The configuration model and startup report MUST recognize `format_translation.enabled` and `content_deduplication.enabled`. Both MUST default to false. Enabling either MUST fail validation with an explicit “not implemented” error. No translation, WebAssembly conversion, Helm conversion, chunking, or cross-repository deduplication behavior MAY be activated by this release.

### Shutdown behavior

- FR-9: Shutdown cancels active work safely and leaves interrupted jobs recoverable.

Service shutdown MUST stop accepting new in-memory dispatches, cancel active work through context cancellation, and wait for workers only up to the existing server shutdown deadline. Interrupted jobs MUST remain recoverable and MUST NOT be marked completed unless the OCI artifact is fully stored and indexed.

## Non-Functional Requirements

### NFR-1: Backward compatibility

With experimental features disabled, all existing automated tests MUST pass and no experimental worker, database polling loop, or API side effect MAY run. Existing configuration files without the new section MUST continue to load.

### NFR-2: Push latency isolation

For an enabled asynchronous generator, work performed synchronously in the manifest push path MUST be limited to eligibility checks and one idempotent job write. A repository benchmark MUST show no more than 50 ms p95 added latency for that work against a local database, excluding pre-existing manifest and blob I/O.

### NFR-3: Bounded resource use

Active generators MUST never exceed `max_concurrent_jobs`. The in-memory dispatch channel MUST never exceed `queue_capacity`. Each job MUST be canceled at `timeout_seconds`. Layer extraction MUST retain the existing archive-size, file-count, path-safety, and decompression limits, with tests demonstrating rejection at each limit.

### NFR-4: Security isolation

Generation MUST perform no outbound network requests and execute no bytes from an image. Repository authorization MUST be tested for both status endpoints. Error responses and persisted `last_error` values MUST be sanitized and capped at 2 KiB.

### NFR-5: Integrity and determinism

Every stored blob and manifest digest MUST be computed from the exact bytes persisted and verified using the existing integrity path. Given the same source digest, package data, generator version, format, and options, component identity and ordering MUST be deterministic. Volatile CycloneDX metadata MAY change only where required by the standard and MUST NOT create duplicate completed jobs for one job identity.

### NFR-6: Restart recovery

Job state MUST survive process and database restart in both SQLite and PostgreSQL modes. Integration tests MUST demonstrate recovery of `pending` and interrupted `running` jobs without duplicate completed referrers.

### NFR-7: Observability

Structured logs MUST include repository, source digest, job identity, attempt, state transition, duration, and sanitized failure reason. Existing metrics infrastructure, when enabled, MUST expose counters for queued, completed, failed, and retried jobs and a generation-duration histogram. Repository names and digests MUST NOT be metric labels.

### NFR-8: Test coverage

New configuration, job-store, package-parser, OCI-artifact, authorization, retry, shutdown, and restart-recovery behavior MUST have automated tests. The changed packages MUST pass `go test`, `go vet`, and race-enabled tests for the worker and job-store packages.

## Acceptance Criteria

### AC-1: Disabled compatibility

**Given** an existing configuration with no `experimental` section, **when** the service starts and images are pushed, **then** startup succeeds, no SBOM jobs or artifacts are created, and existing behavior is unchanged. (FR-1, NFR-1)

### AC-2: Double opt-in

**Given** only the global switch or only the feature switch is enabled, **when** an eligible image manifest is pushed, **then** no SBOM job is created. **Given** both switches are enabled, **then** one durable job is created. (FR-1, FR-3)

### AC-3: Validation

**Given** an enabled SBOM feature with an unsupported format, mode, result-store choice, signing choice, or out-of-range limit, **when** configuration is loaded, **then** startup fails with the exact offending field identified. (FR-1, FR-6)

### AC-4: Asynchronous push

**Given** SBOM generation is enabled and a worker is intentionally blocked, **when** an eligible manifest is pushed, **then** the push succeeds without waiting for generation and the persisted job remains visible as pending or running. (FR-3, FR-4, NFR-2)

### AC-5: CycloneDX contents

**Given** an image containing valid APK and dpkg package databases across multiple layers and whiteouts, **when** its job completes, **then** the CycloneDX document contains exactly the packages visible in the final overlay, stable component ordering, source-image metadata, and no deleted packages. (FR-5, NFR-5)

### AC-6: Safe malformed input

**Given** an image with traversal paths, symlink escapes, a decompression bomb, or malformed package records, **when** generation runs, **then** unsafe content is rejected or skipped within configured limits, no external path is accessed, no content is executed, and the job produces a sanitized bounded result or failure. (FR-5, NFR-3, NFR-4)

### AC-7: Referrer discovery

**Given** a completed job, **when** a client queries `/v2/{repository}/referrers/{sourceDigest}`, **then** exactly one matching CycloneDX artifact descriptor is returned and all referenced content can be fetched with verified digests. (FR-6, NFR-5)

### AC-8: Retry and recovery

**Given** `failure_policy=retry` and a transient storage failure, **when** processing resumes, **then** attempts never exceed `max_attempts`. **Given** termination during a running job, **when** the service restarts, **then** the job is recovered without a duplicate referrer. (FR-4, FR-9, NFR-6)

### AC-9: Authorization and status

**Given** callers with no repository access, pull access, and push access, **when** they use the status APIs, **then** no-access callers are denied, pull callers can read status but cannot enqueue, and push callers can idempotently enqueue. (FR-7, NFR-4)

### AC-10: Queue saturation

**Given** a full in-memory dispatch queue, **when** another eligible manifest is pushed, **then** the push succeeds, its durable job stays pending, memory remains bounded, and later dispatch completes it. (FR-4, NFR-3)

### AC-11: Placeholder flags

**Given** format translation or content deduplication is enabled, **when** the service validates configuration, **then** startup fails with a clear not-implemented message and performs no partial behavior. (FR-8)

### AC-12: Verification suite

**Given** the implementation is complete, **when** the repository verification suite is run, **then** all existing and new tests, static analysis, and targeted race tests pass. (NFR-1, NFR-8)

## Edge Cases

- EC-1: A digest exists as a blob but not as an eligible manifest in the requested repository.
- EC-2: Two tags and multiple concurrent pushes resolve to the same manifest digest.
- EC-3: A tag moves while a job for its old digest is pending.
- EC-4: A source manifest or layer is deleted by retention while a job is pending.
- EC-5: The database succeeds in creating a job but the in-memory queue is full or the process exits immediately.
- EC-6: Blob storage succeeds but manifest metadata indexing fails while publishing the SBOM artifact.
- EC-7: A completed artifact exists but a stale job record is pending or running.
- EC-8: An image has no supported package database, an empty filesystem, foreign layers, nondistributable layers, or duplicated packages.
- EC-9: Package metadata contains invalid UTF-8, extremely long fields, duplicate records, missing versions, or misleading path text.
- EC-10: Whiteouts remove a package database or replace a directory across layers.
- EC-11: A manifest uses a valid digest algorithm other than the algorithms currently supported by storage.
- EC-12: Configuration is enabled with zero-value limits, unknown enum values, result storage disabled, or signing requested.
- EC-13: Shutdown occurs during parsing, blob persistence, or artifact-manifest persistence.
- EC-14: Status is requested before dispatch, during retry backoff, after completion, after failure, or with no matching job.

## API Contracts

### Configuration contract

The `experimental` object is optional and additive. Unknown JSON fields retain the loader's current compatibility behavior. Known experimental fields are validated after defaults are applied. Validation errors identify the JSON field path and invalid value without printing the entire configuration.

### Manifest-push hook

The hook receives repository, stored manifest digest, media type, and parsed manifest metadata only after successful storage. It returns after the durable enqueue attempt. A job-write failure is logged according to `failure_policy` but does not roll back an otherwise successful manifest push in this experimental release.

### Status endpoints

`GET /v2/{repository}/sboms/{digest}/status` reads the latest job status. `POST /v2/{repository}/sboms/{digest}/generate` idempotently requests generation. Both endpoints use the existing registry authentication challenge and authorization model. `{repository}` supports nested repository names using the same routing rules as other v2 endpoints. `{digest}` is validated before database or storage access. Responses use `application/json` and do not cache a pending or running state.

### OCI artifact contract

The produced manifest follows the OCI image-manifest artifact pattern, includes the source descriptor as `subject`, includes the CycloneDX blob as a layer/blob descriptor, and uses an empty content-addressed config descriptor where required by the current manifest model. Artifact metadata is recorded through the existing referrer index so normal subject filtering applies.

## Data Models

### Go configuration model

`Config` gains `Experimental ExperimentalConfig`. The nested model contains `Enabled`, `SBOMGeneration`, `FormatTranslation`, and `ContentDeduplication`. Defaults and validation live in the config package and are unit-tested independently from startup wiring.

### Persistent `sbom_jobs` model

Both SQLite and PostgreSQL gain an additive `sbom_jobs` table with:

| Field | Meaning |
|---|---|
| `id` | Stable job identifier |
| `repository` | Canonical repository name |
| `source_digest` | Immutable source manifest digest |
| `format` | Requested SBOM format |
| `generator_version` | Version participating in job identity |
| `options_hash` | Digest of generation-affecting options |
| `status` | `pending`, `running`, `completed`, or `failed` |
| `attempts` | Number of started attempts |
| `output_manifest_digest` | Completed OCI artifact manifest digest, nullable |
| `last_error` | Sanitized bounded failure text, nullable |
| `created_at` | Creation timestamp |
| `updated_at` | Last state-transition timestamp |

A unique constraint covers `(repository, source_digest, format, generator_version, options_hash)`. An index supports dispatch by status and update time. State transitions use conditional updates so multiple workers cannot claim the same job concurrently.

### CycloneDX model

The internal model contains document metadata, source image component, discovered package components, package evidence, and generation warnings. Components are normalized and sorted before serialization. The implementation MUST use typed structures rather than constructing unvalidated JSON maps at the storage boundary.

## Out of Scope

- OS-1: SPDX output.
- OS-2: SBOM signing, verification, admission blocking, quarantine, or policy enforcement.
- OS-3: Synchronous generation or making a manifest push/pull depend on SBOM success.
- OS-4: Aggregate SBOMs for OCI indexes or multi-platform manifest lists.
- OS-5: Language dependency discovery from lockfiles or installed module trees in the first release.
- OS-6: RPM, Windows, or other operating-system package databases in the first release.
- OS-7: Network-based license, supplier, vulnerability, or package enrichment.
- OS-8: Client-submitted SBOM validation beyond existing OCI artifact handling.
- OS-9: Runtime configuration reload.
- OS-10: Universal format translation, OCI-to-WebAssembly conversion, Helm conversion, or any other translation engine.
- OS-11: Chunk-level or cross-repository deduplication and changes to current blob identity or garbage collection.
- OS-12: Promoting any experimental capability to a stable compatibility guarantee.

## Approval Gate

Approval of this specification authorizes the additive configuration model, authenticated v2 status endpoints, additive `sbom_jobs` schema for SQLite and PostgreSQL, background workers, and OCI referrer writes described above. Any material change to those contracts, any destructive migration, or any expansion into the out-of-scope items requires a revised specification and approval.
