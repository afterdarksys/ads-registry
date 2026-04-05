-- Migration: 017_artifact_metadata.sql
-- Description: Adds tables to support generic Artifacts across many package formats

CREATE TABLE IF NOT EXISTS universal_artifacts (
    id SERIAL PRIMARY KEY,
    format VARCHAR(50) NOT NULL, -- e.g. 'npm', 'pypi', 'apt'
    namespace VARCHAR(255) NOT NULL, -- e.g. organization or user, or just 'default'
    package_name VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (format, namespace, package_name, version)
);

CREATE TABLE IF NOT EXISTS universal_artifact_blobs (
    id SERIAL PRIMARY KEY,
    artifact_id INTEGER NOT NULL REFERENCES universal_artifacts(id) ON DELETE CASCADE,
    blob_digest VARCHAR(255) NOT NULL, -- references the main blobs table
    file_name VARCHAR(255) NOT NULL, -- e.g., 'package.tgz', 'chart-1.0.tgz', 'libfoo.deb'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(artifact_id, blob_digest)
);

CREATE TABLE IF NOT EXISTS universal_artifact_metadata (
    id SERIAL PRIMARY KEY,
    artifact_id INTEGER NOT NULL REFERENCES universal_artifacts(id) ON DELETE CASCADE,
    raw_data JSONB NOT NULL,
    UNIQUE(artifact_id)
);

CREATE INDEX idx_universal_artifact_metadata_jsonb ON universal_artifact_metadata USING GIN (raw_data);
CREATE INDEX idx_universal_artifacts_lookup ON universal_artifacts(format, namespace, package_name);
