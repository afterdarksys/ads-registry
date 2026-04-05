package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	
	"github.com/ryan/ads-registry/internal/db"
)

// CreateArtifact registers a new package version into the generic repository
func (s *PostgresStore) CreateArtifact(ctx context.Context, artifact *db.UniversalArtifact) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO universal_artifacts (format, namespace, package_name, version)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (format, namespace, package_name, version) DO UPDATE SET version = EXCLUDED.version
		RETURNING id
	`, artifact.Format, artifact.Namespace, artifact.PackageName, artifact.Version).Scan(&id)

	if err != nil {
		return 0, err
	}

	if artifact.Metadata != nil {
		err = s.StoreArtifactMetadata(ctx, id, artifact.Metadata)
	}

	return id, err
}

// GetArtifact retrieves a specific version of a package
func (s *PostgresStore) GetArtifact(ctx context.Context, format, namespace, packageName, version string) (*db.UniversalArtifact, error) {
	artifact := &db.UniversalArtifact{}
	var metadata []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.format, a.namespace, a.package_name, a.version, a.created_at, m.raw_data
		FROM universal_artifacts a
		LEFT JOIN universal_artifact_metadata m ON a.id = m.artifact_id
		WHERE a.format = $1 AND a.namespace = $2 AND a.package_name = $3 AND a.version = $4
	`, format, namespace, packageName, version).Scan(
		&artifact.ID, &artifact.Format, &artifact.Namespace, &artifact.PackageName, &artifact.Version, &artifact.CreatedAt, &metadata,
	)

	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if len(metadata) > 0 {
		artifact.Metadata = json.RawMessage(metadata)
	}

	// Fetch blobs
	rows, err := s.db.QueryContext(ctx, `
		SELECT blob_digest, file_name, created_at 
		FROM universal_artifact_blobs 
		WHERE artifact_id = $1
	`, artifact.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var b db.ArtifactBlob
			b.ArtifactID = artifact.ID
			if err := rows.Scan(&b.BlobDigest, &b.FileName, &b.CreatedAt); err == nil {
				artifact.Blobs = append(artifact.Blobs, b)
			}
		}
	}

	return artifact, nil
}

// ListArtifacts returns all versions for a package
func (s *PostgresStore) ListArtifacts(ctx context.Context, format, namespace, packageName string) ([]*db.UniversalArtifact, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.format, a.namespace, a.package_name, a.version, a.created_at, m.raw_data
		FROM universal_artifacts a
		LEFT JOIN universal_artifact_metadata m ON a.id = m.artifact_id
		WHERE a.format = $1 AND a.namespace = $2 AND a.package_name = $3
		ORDER BY a.created_at DESC
	`, format, namespace, packageName)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*db.UniversalArtifact
	for rows.Next() {
		artifact := &db.UniversalArtifact{}
		var metadata []byte
		if err := rows.Scan(&artifact.ID, &artifact.Format, &artifact.Namespace, &artifact.PackageName, &artifact.Version, &artifact.CreatedAt, &metadata); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			artifact.Metadata = json.RawMessage(metadata)
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts, rows.Err()
}

// SearchArtifacts allows querying metadata using JSONB operators
func (s *PostgresStore) SearchArtifacts(ctx context.Context, format, namespace string, searchQuery json.RawMessage) ([]*db.UniversalArtifact, error) {
	// For phase 1 we'll implement a basic listing. Real JSONB queries will evaluate `searchQuery`.
	// Mocking full JSONB execution for MVP PyPI/NPM listing.
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.format, a.namespace, a.package_name, a.version, a.created_at, m.raw_data
		FROM universal_artifacts a
		LEFT JOIN universal_artifact_metadata m ON a.id = m.artifact_id
		WHERE a.format = $1 AND a.namespace = $2
		ORDER BY a.package_name ASC, a.created_at DESC
	`, format, namespace)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*db.UniversalArtifact
	for rows.Next() {
		artifact := &db.UniversalArtifact{}
		var metadata []byte
		if err := rows.Scan(&artifact.ID, &artifact.Format, &artifact.Namespace, &artifact.PackageName, &artifact.Version, &artifact.CreatedAt, &metadata); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			artifact.Metadata = json.RawMessage(metadata)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

// StoreArtifactMetadata saves format-specific JSON metadata
func (s *PostgresStore) StoreArtifactMetadata(ctx context.Context, artifactID int64, data json.RawMessage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO universal_artifact_metadata (artifact_id, raw_data)
		VALUES ($1, $2)
		ON CONFLICT (artifact_id) DO UPDATE SET raw_data = EXCLUDED.raw_data
	`, artifactID, []byte(data))
	return err
}

// AttachBlob links a stored blob to the artifact
func (s *PostgresStore) AttachBlob(ctx context.Context, artifactID int64, blobDigest, fileName string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO universal_artifact_blobs (artifact_id, blob_digest, file_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (artifact_id, blob_digest) DO NOTHING
	`, artifactID, blobDigest, fileName)
	return err
}

// DeleteArtifact removes a specific version of a package
func (s *PostgresStore) DeleteArtifact(ctx context.Context, format, namespace, packageName, version string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM universal_artifacts
		WHERE format = $1 AND namespace = $2 AND package_name = $3 AND version = $4
	`, format, namespace, packageName, version)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return db.ErrNotFound
	}

	return nil
}

// DeleteAllArtifactVersions removes all versions of a package
func (s *PostgresStore) DeleteAllArtifactVersions(ctx context.Context, format, namespace, packageName string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM universal_artifacts
		WHERE format = $1 AND namespace = $2 AND package_name = $3
	`, format, namespace, packageName)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return db.ErrNotFound
	}

	return nil
}

// GetPackageNames returns unique package names for a format and namespace
func (s *PostgresStore) GetPackageNames(ctx context.Context, format, namespace string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT package_name
		FROM universal_artifacts
		WHERE format = $1 AND namespace = $2
		ORDER BY package_name ASC
	`, format, namespace)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	return names, rows.Err()
}

// GetArtifactStatistics returns usage statistics for artifacts
func (s *PostgresStore) GetArtifactStatistics(ctx context.Context, format, namespace string) (*db.ArtifactStatistics, error) {
	stats := &db.ArtifactStatistics{
		FormatBreakdown: make(map[string]int64),
	}

	var query string
	var args []interface{}

	if format != "" && namespace != "" {
		query = `
			SELECT
				COUNT(DISTINCT package_name) as packages,
				COUNT(*) as versions
			FROM universal_artifacts
			WHERE format = $1 AND namespace = $2
		`
		args = []interface{}{format, namespace}
	} else if format != "" {
		query = `
			SELECT
				COUNT(DISTINCT package_name) as packages,
				COUNT(*) as versions
			FROM universal_artifacts
			WHERE format = $1
		`
		args = []interface{}{format}
	} else {
		query = `
			SELECT
				COUNT(DISTINCT package_name) as packages,
				COUNT(*) as versions
			FROM universal_artifacts
		`
	}

	err := s.db.QueryRowContext(ctx, query, args...).Scan(&stats.TotalPackages, &stats.TotalVersions)
	if err != nil {
		return nil, err
	}

	// Get format breakdown
	breakdownRows, err := s.db.QueryContext(ctx, `
		SELECT format, COUNT(*) as count
		FROM universal_artifacts
		GROUP BY format
	`)
	if err == nil {
		defer breakdownRows.Close()
		for breakdownRows.Next() {
			var format string
			var count int64
			if err := breakdownRows.Scan(&format, &count); err == nil {
				stats.FormatBreakdown[format] = count
			}
		}
	}

	return stats, nil
}
