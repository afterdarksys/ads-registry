package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type PostgresStore struct {
	db *sql.DB
}

func New(cfg config.DatabaseConfig) (*PostgresStore, error) {
	database, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		database.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		database.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		database.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	if err := database.Ping(); err != nil {
		return nil, err
	}

	s := &PostgresStore{db: database}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return s, nil
}

func (s *PostgresStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS namespaces (
		id SERIAL PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS repositories (
		id SERIAL PRIMARY KEY,
		namespace_id INTEGER NOT NULL REFERENCES namespaces(id),
		name TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(namespace_id, name)
	);

	CREATE TABLE IF NOT EXISTS manifests (
		id SERIAL PRIMARY KEY,
		repo_id INTEGER NOT NULL REFERENCES repositories(id),
		reference TEXT NOT NULL,
		media_type TEXT NOT NULL,
		digest TEXT NOT NULL,
		payload BYTEA NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(repo_id, reference)
	);

	CREATE TABLE IF NOT EXISTS blobs (
		id SERIAL PRIMARY KEY,
		digest TEXT UNIQUE NOT NULL,
		size_bytes BIGINT NOT NULL,
		media_type TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		token_hash TEXT NOT NULL,
		scopes TEXT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS scan_reports (
		id SERIAL PRIMARY KEY,
		digest TEXT UNIQUE NOT NULL,
		scanner TEXT NOT NULL,
		data BYTEA NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	-- Performance indexes
	CREATE INDEX IF NOT EXISTS idx_manifests_digest ON manifests(digest);
	CREATE INDEX IF NOT EXISTS idx_manifests_repo_id ON manifests(repo_id);
	CREATE INDEX IF NOT EXISTS idx_manifests_created_at ON manifests(created_at);
	CREATE INDEX IF NOT EXISTS idx_blobs_created_at ON blobs(created_at);
	CREATE INDEX IF NOT EXISTS idx_scan_reports_digest ON scan_reports(digest);
	CREATE INDEX IF NOT EXISTS idx_scan_reports_created_at ON scan_reports(created_at);
	CREATE INDEX IF NOT EXISTS idx_repositories_namespace_id ON repositories(namespace_id);
	CREATE INDEX IF NOT EXISTS idx_repositories_name ON repositories(name);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) CreateNamespace(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO namespaces (name) VALUES ($1) ON CONFLICT DO NOTHING", name)
	return err
}

func (s *PostgresStore) CreateRepository(ctx context.Context, namespace, name string) error {
	var nsID int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = $1", namespace).Scan(&nsID)
	if err != nil {
		if err == sql.ErrNoRows {
			if err := s.CreateNamespace(ctx, namespace); err != nil {
				return err
			}
			s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = $1", namespace).Scan(&nsID)
		} else {
			return err
		}
	}

	_, err = s.db.ExecContext(ctx, "INSERT INTO repositories (namespace_id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING", nsID, name)
	return err
}

func (s *PostgresStore) PutManifest(ctx context.Context, repo, reference string, mediaType string, digest string, payload []byte) error {
	// Parse namespace/repo path
	ns, repoName := parseRepoPath(repo)

	// Ensure namespace and repo exist
	if err := s.CreateNamespace(ctx, ns); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	if err := s.CreateRepository(ctx, ns, repoName); err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	var repoID int
	err := s.db.QueryRowContext(ctx, `
		SELECT r.id FROM repositories r
		JOIN namespaces n ON r.namespace_id = n.id
		WHERE n.name = $1 AND r.name = $2`, ns, repoName).Scan(&repoID)

	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO manifests (repo_id, reference, media_type, digest, payload) 
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(repo_id, reference) DO UPDATE SET
			media_type=EXCLUDED.media_type,
			digest=EXCLUDED.digest,
			payload=EXCLUDED.payload
	`, repoID, reference, mediaType, digest, payload)

	return err
}

func (s *PostgresStore) GetManifest(ctx context.Context, repo, reference string) (mediaType, digest string, payload []byte, err error) {
	// Parse namespace/repo path
	ns, repoName := parseRepoPath(repo)

	err = s.db.QueryRowContext(ctx, `
		SELECT m.media_type, m.digest, m.payload
		FROM manifests m
		JOIN repositories r ON m.repo_id = r.id
		JOIN namespaces n ON r.namespace_id = n.id
		WHERE n.name = $1 AND r.name = $2 AND m.reference = $3`, ns, repoName, reference).
		Scan(&mediaType, &digest, &payload)

	if err == sql.ErrNoRows {
		return "", "", nil, db.ErrNotFound
	}
	return mediaType, digest, payload, err
}

func (s *PostgresStore) ListManifests(ctx context.Context) ([]db.ManifestRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.name, r.name, m.reference, m.digest 
		FROM manifests m
		JOIN repositories r ON m.repo_id = r.id
		JOIN namespaces n ON r.namespace_id = n.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []db.ManifestRecord
	for rows.Next() {
		var rec db.ManifestRecord
		if err := rows.Scan(&rec.Namespace, &rec.Repo, &rec.Reference, &rec.Digest); err != nil {
			return nil, err
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (s *PostgresStore) PutBlob(ctx context.Context, digest string, size int64, mediaType string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO blobs (digest, size_bytes, media_type) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", digest, size, mediaType)
	return err
}

func (s *PostgresStore) BlobExists(ctx context.Context, digest string) (bool, error) {
	var id int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM blobs WHERE digest = $1", digest).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *PostgresStore) GetBlobSize(ctx context.Context, digest string) (int64, error) {
	var size int64
	err := s.db.QueryRowContext(ctx, "SELECT size_bytes FROM blobs WHERE digest = $1", digest).Scan(&size)
	if err == sql.ErrNoRows {
		return 0, db.ErrNotFound
	}
	return size, err
}

func (s *PostgresStore) GetUserByToken(ctx context.Context, token string) (*db.User, error) {
	var u db.User
	var scopes string
	err := s.db.QueryRowContext(ctx, "SELECT id, username, scopes FROM users WHERE token_hash = $1", token).Scan(&u.ID, &u.Username, &scopes)
	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Scopes = []string{scopes}
	return &u, nil
}

func (s *PostgresStore) SaveScanReport(ctx context.Context, digest string, scanner string, data []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scan_reports (digest, scanner, data) 
		VALUES ($1, $2, $3)
		ON CONFLICT(digest) DO UPDATE SET
			scanner=EXCLUDED.scanner,
			data=EXCLUDED.data,
			created_at=CURRENT_TIMESTAMP
	`, digest, scanner, data)
	return err
}

func (s *PostgresStore) GetScanReport(ctx context.Context, digest string, scanner string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM scan_reports WHERE digest = $1 AND scanner = $2", digest, scanner).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	return data, err
}

func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (*db.User, error) {
	var u db.User
	var scopes string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, username, token_hash, scopes FROM users WHERE username = $1",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &scopes)

	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Scopes = []string{scopes}
	return &u, nil
}

func (s *PostgresStore) AuthenticateUser(ctx context.Context, username, password string) (*db.User, error) {
	user, err := s.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	// Verify password using bcrypt
	if err := verifyPassword(user.PasswordHash, password); err != nil {
		return nil, db.ErrNotFound // Don't reveal user exists
	}

	return user, nil
}

func (s *PostgresStore) CreateUser(ctx context.Context, username, passwordHash string, scopes []string) error {
	scopesJSON := strings.Join(scopes, ",")
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO users (username, token_hash, scopes) VALUES ($1, $2, $3)",
		username, passwordHash, scopesJSON,
	)
	return err
}

// Helper functions for password hashing
func verifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// parseRepoPath splits a repository path into namespace and repository name
// Examples:
//   - "library/ubuntu" -> ("library", "ubuntu")
//   - "myorg/myteam/myapp" -> ("myorg", "myteam/myapp")
//   - "ubuntu" -> ("library", "ubuntu")
func parseRepoPath(repoPath string) (namespace, repo string) {
	parts := strings.Split(repoPath, "/")
	if len(parts) >= 2 {
		namespace = parts[0]
		repo = strings.Join(parts[1:], "/")
	} else {
		// Default to "library" for single-component names (Docker Hub convention)
		namespace = "library"
		repo = repoPath
	}
	return
}
