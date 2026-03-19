package v2

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/ryan/ads-registry/internal/db"
)

// getReferrers implements the OCI Referrers API
// GET /v2/{namespace}/{repo}/referrers/{digest}
//
// Returns a list of artifacts that reference the specified digest.
// This is used by tools like Cosign, ORAS, and Notary v2 to discover:
// - Signatures
// - SBOMs
// - Attestations
// - Provenance
// - Other related artifacts
//
// Example:
//   GET /v2/myorg/myapp/referrers/sha256:abc123?artifactType=application/vnd.dev.cosign.simplesigning.v1+json
//
// Returns:
//   {
//     "schemaVersion": 2,
//     "mediaType": "application/vnd.oci.image.index.v1+json",
//     "manifests": [
//       {
//         "digest": "sha256:def456...",
//         "mediaType": "application/vnd.oci.image.manifest.v1+json",
//         "artifactType": "application/vnd.dev.cosign.simplesigning.v1+json",
//         "size": 1234
//       }
//     ]
//   }
func (r *Router) getReferrers(w http.ResponseWriter, req *http.Request) {
	// Extract repository path components using the custom router context extractor
	fullRepo, _ := getRepoContext(req)
	digest := chi.URLParam(req, "digest")

	// Optional filter by artifact type
	artifactType := req.URL.Query().Get("artifactType")
	log.Printf("[REFERRERS] GET %s referrers for %s (artifactType=%s)", fullRepo, digest, artifactType)

	// Get referrers from database
	referrers, err := r.db.ListReferrers(req.Context(), digest, artifactType)
	if err != nil {
		log.Printf("[REFERRERS] Error listing referrers: %v", err)
		http.Error(w, "failed to list referrers", http.StatusInternalServerError)
		return
	}

	// Build OCI Image Index response
	manifests := make([]map[string]interface{}, 0, len(referrers))
	for _, ref := range referrers {
		manifest := map[string]interface{}{
			"digest":    ref.Digest,
			"mediaType": ref.MediaType,
			"size":      ref.Size,
		}

		// Add artifactType if present
		if ref.ArtifactType != "" {
			manifest["artifactType"] = ref.ArtifactType
		}

		// Add annotations if present
		if len(ref.Annotations) > 0 {
			manifest["annotations"] = ref.Annotations
		}

		manifests = append(manifests, manifest)
	}

	// Return OCI Image Index format
	response := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.index.v1+json",
		"manifests":     manifests,
	}

	w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
	w.Header().Set("Docker-Distribution-API-Version", "registry/2.0")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[REFERRERS] Error encoding response: %v", err)
	}

	log.Printf("[REFERRERS] Returned %d referrers for %s", len(manifests), digest)
}

// ListArtifacts provides a custom endpoint to list artifacts by type
// GET /api/v2/artifacts?type={artifactType}&limit={limit}
//
// Examples:
//   GET /api/v2/artifacts?type=application/vnd.cncf.helm.chart.content.v1.tar%2Bgzip
//   GET /api/v2/artifacts?type=application/vnd.dev.cosign.simplesigning.v1%2Bjson
//   GET /api/v2/artifacts?type=application/vnd.cyclonedx%2Bjson
func (r *Router) ListArtifacts(w http.ResponseWriter, req *http.Request) {
	artifactType := req.URL.Query().Get("type")
	if artifactType == "" {
		http.Error(w, "artifactType query parameter is required", http.StatusBadRequest)
		return
	}

	limitStr := req.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	log.Printf("[ARTIFACTS] Listing artifacts of type %s (limit=%d)", artifactType, limit)

	artifacts, err := r.db.ListArtifactsByType(req.Context(), artifactType, limit)
	if err != nil {
		log.Printf("[ARTIFACTS] Error listing artifacts: %v", err)
		http.Error(w, "failed to list artifacts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"artifacts": artifacts,
		"count":     len(artifacts),
	}); err != nil {
		log.Printf("[ARTIFACTS] Error encoding response: %v", err)
	}

	log.Printf("[ARTIFACTS] Returned %d artifacts", len(artifacts))
}

// extractArtifactMetadata extracts artifact metadata from a manifest
// Used during manifest PUT to track artifact types, Helm charts, etc.
func extractArtifactMetadata(manifestPayload []byte, mediaType string, digest string) *db.ArtifactMetadata {
	var manifest map[string]interface{}
	if err := json.Unmarshal(manifestPayload, &manifest); err != nil {
		return nil
	}

	metadata := &db.ArtifactMetadata{
		Digest: digest,
	}

	// Extract artifact type (OCI artifact-specific field)
	if artifactType, ok := manifest["artifactType"].(string); ok {
		metadata.ArtifactType = artifactType
	} else {
		// Fallback to media type if no explicit artifactType
		metadata.ArtifactType = mediaType
	}

	// Extract subject digest (for referrers)
	if subject, ok := manifest["subject"].(map[string]interface{}); ok {
		if subjectDigest, ok := subject["digest"].(string); ok {
			metadata.SubjectDigest = subjectDigest
		}
	}

	// Extract Helm chart metadata from config
	if config, ok := manifest["config"].(map[string]interface{}); ok {
		if configMediaType, ok := config["mediaType"].(string); ok {
			// Helm chart config has specific media type
			if configMediaType == "application/vnd.cncf.helm.config.v1+json" {
				// Extract Helm-specific annotations
				if annotations, ok := manifest["annotations"].(map[string]interface{}); ok {
					if chartName, ok := annotations["org.opencontainers.image.title"].(string); ok {
						metadata.ChartName = chartName
					}
					if chartVersion, ok := annotations["org.opencontainers.image.version"].(string); ok {
						metadata.ChartVersion = chartVersion
					}
					if appVersion, ok := annotations["org.opencontainers.image.ref.name"].(string); ok {
						metadata.AppVersion = appVersion
					}
				}
			}
		}
	}

	// Check if this is a Helm chart by media type
	if mediaType == "application/vnd.cncf.helm.chart.content.v1.tar+gzip" ||
		metadata.ArtifactType == "application/vnd.cncf.helm.chart.content.v1.tar+gzip" {
		// Store full manifest as metadata for Helm charts
		if jsonData, err := json.Marshal(manifest); err == nil {
			metadata.MetadataJSON = string(jsonData)
		}
	}

	return metadata
}
