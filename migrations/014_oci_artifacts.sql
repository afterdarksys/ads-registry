-- Migration: OCI Artifact Support
-- Adds support for OCI Referrers API, artifact types, and Helm charts
-- Date: 2026-03-15

-- Artifact metadata table
-- Tracks artifact-specific metadata for manifests
CREATE TABLE IF NOT EXISTS artifact_metadata (
    digest VARCHAR(255) PRIMARY KEY,
    artifact_type VARCHAR(255) NOT NULL,  -- e.g., application/vnd.cncf.helm.chart.content.v1.tar+gzip
    subject_digest VARCHAR(255),          -- For referrers: digest of the subject this artifact refers to
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Helm-specific metadata
    chart_name VARCHAR(255),
    chart_version VARCHAR(100),
    app_version VARCHAR(100),

    -- Generic metadata JSON
    metadata_json TEXT,

    FOREIGN KEY (subject_digest) REFERENCES manifests(digest) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_artifact_type ON artifact_metadata(artifact_type);
CREATE INDEX IF NOT EXISTS idx_subject_digest ON artifact_metadata(subject_digest);
CREATE INDEX IF NOT EXISTS idx_chart_name_version ON artifact_metadata(chart_name, chart_version);

-- Add artifact_type column to manifests table (for easier querying)
ALTER TABLE manifests ADD COLUMN IF NOT EXISTS artifact_type VARCHAR(255);
ALTER TABLE manifests ADD COLUMN IF NOT EXISTS subject_digest VARCHAR(255);

-- Index for referrers lookup
CREATE INDEX IF NOT EXISTS idx_manifests_subject ON manifests(subject_digest) WHERE subject_digest IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_manifests_artifact_type ON manifests(artifact_type) WHERE artifact_type IS NOT NULL;

-- Artifact referrers view
-- Makes it easy to query "what artifacts refer to this digest?"
CREATE OR REPLACE VIEW v_artifact_referrers AS
SELECT
    m.subject_digest,
    m.digest AS referrer_digest,
    m.media_type,
    m.artifact_type,
    m.namespace,
    m.repo,
    m.created_at,
    am.chart_name,
    am.chart_version,
    am.metadata_json
FROM manifests m
LEFT JOIN artifact_metadata am ON m.digest = am.digest
WHERE m.subject_digest IS NOT NULL;

-- Helm charts view
-- Convenient view for listing all Helm charts
CREATE OR REPLACE VIEW v_helm_charts AS
SELECT
    m.digest,
    m.namespace,
    m.repo,
    m.media_type,
    m.created_at,
    am.chart_name,
    am.chart_version,
    am.app_version,
    am.metadata_json
FROM manifests m
INNER JOIN artifact_metadata am ON m.digest = am.digest
WHERE m.artifact_type LIKE 'application/vnd.cncf.helm.%'
   OR m.media_type LIKE 'application/vnd.cncf.helm.%';

-- Artifact types summary view
CREATE OR REPLACE VIEW v_artifact_types AS
SELECT
    artifact_type,
    COUNT(*) as count,
    SUM(size) as total_size
FROM manifests
WHERE artifact_type IS NOT NULL
GROUP BY artifact_type;
