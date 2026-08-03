# Found Bugs

Review verdict: **request changes**. The blob lifecycle has multiple correctness bugs, including one that can genuinely mutate a blob after digest verification.

## Resolution Status

Implementation completed in the current working tree:

- Findings 1–4 and 6–8 are fixed and covered by regression tests.
- Finding 5 is fixed for unbounded memory use: S3 and OCI now stream multipart uploads. Chunked appends still rewrite prior object data because the current storage-provider interface has no persistent multipart-session abstraction.
- Finding 9 is remediated in the current Git index: the secret files are ignored and staged for removal from tracking while their local working copies are preserved. Any deployed credentials still require rotation, and removing them from existing Git history requires a coordinated history rewrite.
- The root build collision and vet failures are fixed. `go test ./...`, targeted race tests, `go vet ./...`, and `go build ./...` pass.

## Findings

### 1. Critical — Concurrent PATCH and PUT can corrupt a verified blob

PATCH operations lock by upload UUID, but finalization does not acquire that lock (`internal/api/v2/router.go:879` and `internal/api/v2/router.go:948`).

A PUT can hash and rename the temporary file while a PATCH still has it open. The PATCH may then flush additional buffered bytes into the renamed file, leaving content that no longer matches its filename or digest. This is the strongest candidate for the observed blob corruption.

### 2. Critical — Blob existence is global, while files are repository-local

The database keys blobs only by digest (`internal/db/sqlite/sqlite.go:122`), but storage uses `<repository>/<digest>`. HEAD consults only the global database and never checks the repository-local file (`internal/api/v2/router.go:703`).

Consequences:

- A blob uploaded to repository A makes HEAD return 200 for repository B.
- Docker may therefore skip uploading it to repository B.
- The manifest is accepted, but a subsequent GET from repository B returns `blob unknown to storage`.

### 3. High — Singleflight incorrectly merges finalizations across repositories

Finalization is deduplicated solely by digest (`internal/api/v2/router.go:1126`). If two repositories concurrently upload the same digest, only the first callback moves its file. The second request inherits the successful result and returns 201, but its repository-local blob remains missing and its temporary upload is stranded.

The singleflight key must include the repository, or blobs must use genuinely global content-addressed storage.

### 4. High — Manifests can reference nonexistent blobs

Manifest PUT validates the JSON and immediately stores it without verifying that its config and layer descriptors exist in the target repository (`internal/api/v2/router.go:539`).

This permits publication of images that cannot be pulled and compounds the false-positive HEAD behavior described above.

### 5. High — S3 and OCI blob uploads buffer entire layers in memory

Both providers use `bytes.Buffer`. Appending first downloads the entire existing object, and closing uploads the complete buffer:

- `internal/storage/s3/s3.go:61`
- `internal/storage/s3/s3.go:71`
- `internal/storage/oci/oci.go:110`

A multi-gigabyte layer can exhaust server memory. Chunked uploads repeatedly copy the accumulated blob, making them approximately quadratic in transferred data.

### 6. High — Peer sync and scanners use incompatible blob paths

Uploads store blobs as `<repo>/<digest>`, while peer sync reads `blobs/<algorithm>/<prefix>/<hash>/data`:

- `internal/sync/manager.go:319`
- `internal/sync/manager.go:476`

ClamAV and the static analyzer instead read `blobs/<digest>`:

- `internal/scanner/clamav/scanner.go:303`
- `internal/scanner/static_analyzer.go:78`

These components generally cannot find blobs uploaded through the registry API.

Peer sync also panics on a syntactically short digest because `getBlobPath` accesses `hash[:2]` without validating the hash length.

### 7. High — The 10 GiB limit is per request, not per upload

Every PATCH may append up to 10 GiB, so repeated chunks can grow an upload without a total limit (`internal/api/v2/router.go:884`).

`io.LimitReader` also silently truncates unknown-length bodies instead of detecting overflow. This leaves a disk-exhaustion denial-of-service path.

### 8. Medium — Partial blob responses are malformed

Range GET retains the full blob `Content-Length`, sends no `Content-Range`, and does not reject offsets beyond EOF (`internal/api/v2/router.go:709`).

Clients can interpret the response as truncated or retry indefinitely.

### 9. Critical security issue — Private keys and a static service password are tracked

The repository contains tracked secret material:

- `certs/server.key`
- `keys/private.key`
- `authentik.env:1`

If any of these credentials were deployed, they should be considered compromised and rotated. The files should then be removed from Git history and replaced with runtime secret injection.

## Verification

Running `/usr/local/bin/go test ./...` did not pass.

### Root package build failure

The root package contains two `main` functions:

- `debug_quota.go:10`
- `testQuery.go:12`

This produces a `main redeclared in this block` build error.

### Vet failures

`cmd/artifactadm/cmd` fails vet checks because two `fmt.Println` calls contain redundant trailing newlines:

- `cmd/artifactadm/cmd/prune.go:108`
- `cmd/artifactadm/cmd/stats.go:95`

### Test coverage gap

The existing `internal/api/v2` tests pass, but they primarily test route matching. There are no tests covering:

- PATCH versus PUT concurrency for the same upload UUID
- Concurrent same-digest uploads to different repositories
- Repository-local blob existence versus the global blob database
- Manifest references to missing blobs
- Range response headers and invalid offsets
- S3 or OCI large/chunked upload behavior
- Peer-sync and scanner blob path compatibility

## Recommended Fix Order

1. Serialize PATCH and PUT for the same upload UUID and keep the lock alive through digest verification, storage move, and database commit.
2. Choose one blob ownership model: global content-addressed storage or explicit repository-to-blob membership. Make database, HEAD, GET, upload, mount, scanner, and sync paths use it consistently.
3. Fix singleflight isolation and add concurrent upload regression tests.
4. Reject manifests whose referenced blobs are unavailable to the target repository.
5. Replace S3 and OCI whole-object buffering with multipart/streaming upload sessions.
6. Enforce an aggregate upload-session size and detect bodies that exceed the configured maximum.
7. Correct Range response semantics.
8. Rotate and purge tracked secret material.
9. Remove or relocate the duplicate root debug programs and resolve vet failures so the full test command passes.
