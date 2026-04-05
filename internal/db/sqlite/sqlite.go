package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type SQLiteStore struct {
	db *sql.DB
}

func New(cfg config.DatabaseConfig) (*SQLiteStore, error) {
	// Ensure directory exists if it's a file
	if cfg.DSN != ":memory:" {
		// Basic check, in reality we'd parse the path
		err := os.MkdirAll("data", 0755)
		if err != nil {
			return nil, err
		}
	}

	database, err := sql.Open("sqlite3", cfg.DSN)
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

	s := &SQLiteStore{db: database}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS namespaces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS repositories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		namespace_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(namespace_id, name),
		FOREIGN KEY(namespace_id) REFERENCES namespaces(id)
	);

	CREATE TABLE IF NOT EXISTS manifests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repo_id INTEGER NOT NULL,
		reference TEXT NOT NULL, -- Tag or Digest
		media_type TEXT NOT NULL,
		digest TEXT NOT NULL,
		payload BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(repo_id, reference),
		FOREIGN KEY(repo_id) REFERENCES repositories(id)
	);

	CREATE TABLE IF NOT EXISTS blobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		digest TEXT UNIQUE NOT NULL,
		size_bytes INTEGER NOT NULL,
		media_type TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		token_hash TEXT NOT NULL,
		scopes TEXT NOT NULL, -- JSON array of scopes
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS scan_reports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		digest TEXT UNIQUE NOT NULL,
		scanner TEXT NOT NULL,
		data BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);

	CREATE TABLE IF NOT EXISTS user_groups (
		user_id INTEGER NOT NULL REFERENCES users(id),
		group_id INTEGER NOT NULL REFERENCES groups(id),
		PRIMARY KEY (user_id, group_id)
	);

	CREATE TABLE IF NOT EXISTS quotas (
		namespace_id INTEGER PRIMARY KEY REFERENCES namespaces(id),
		limit_bytes BIGINT NOT NULL,
		used_bytes BIGINT NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS policies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		expression TEXT UNIQUE NOT NULL
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

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) CreateNamespace(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO namespaces (name) VALUES (?)", name)
	return err
}

func (s *SQLiteStore) CreateRepository(ctx context.Context, namespace, name string) error {
	var nsID int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = ?", namespace).Scan(&nsID)
	if err != nil {
		if err == sql.ErrNoRows {
			// Auto create namespace for MVP ease
			if err := s.CreateNamespace(ctx, namespace); err != nil {
				return err
			}
			s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = ?", namespace).Scan(&nsID)
		} else {
			return err
		}
	}

	_, err = s.db.ExecContext(ctx, "INSERT OR IGNORE INTO repositories (namespace_id, name) VALUES (?, ?)", nsID, name)
	return err
}

func (s *SQLiteStore) ListRepositories(ctx context.Context, limit int, last string) ([]string, error) {
	lastNs, lastRepo := "", ""
	if last != "" {
		lastNs, lastRepo = parseRepoPath(last)
	}

	query := `
		SELECT n.name, r.name
		FROM repositories r
		JOIN namespaces n ON r.namespace_id = n.id
	`
	var args []interface{}
	if last != "" {
		query += " WHERE n.name > ? OR (n.name = ? AND r.name > ?)"
		args = append(args, lastNs, lastNs, lastRepo)
	}
	query += " ORDER BY n.name, r.name"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []string
	for rows.Next() {
		var ns, name string
		if err := rows.Scan(&ns, &name); err != nil {
			return nil, err
		}
		if ns == "library" {
			repos = append(repos, name)
		} else {
			repos = append(repos, ns+"/"+name)
		}
	}
	return repos, rows.Err()
}

func (s *SQLiteStore) ListPolicies(ctx context.Context) ([]db.PolicyRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, expression FROM policies ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []db.PolicyRecord
	for rows.Next() {
		var p db.PolicyRecord
		if err := rows.Scan(&p.ID, &p.Expression); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *SQLiteStore) AddPolicy(ctx context.Context, expression string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO policies (expression) VALUES (?) ON CONFLICT(expression) DO NOTHING`, expression)
	return err
}

func (s *SQLiteStore) DeletePolicy(ctx context.Context, id int) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM policies WHERE id = ?`, id)
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

func (s *SQLiteStore) ListTags(ctx context.Context, repo string, limit int, last string) ([]string, error) {
	ns, repoName := parseRepoPath(repo)

	query := `
		SELECT m.reference
		FROM manifests m
		JOIN repositories r ON m.repo_id = r.id
		JOIN namespaces n ON r.namespace_id = n.id
		WHERE n.name = ? AND r.name = ? 
		AND m.reference NOT LIKE 'sha256:%'
	`
	args := []interface{}{ns, repoName}

	if last != "" {
		query += " AND m.reference > ?"
		args = append(args, last)
	}
	query += " ORDER BY m.reference"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *SQLiteStore) PutManifest(ctx context.Context, repo, reference string, mediaType string, digest string, payload []byte) error {
	var repoID int

	// Parse namespace/repo path
	ns, repoName := parseRepoPath(repo)

	// Ensure namespace and repo exist
	if err := s.CreateNamespace(ctx, ns); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	if err := s.CreateRepository(ctx, ns, repoName); err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	err := s.db.QueryRowContext(ctx, `
		SELECT r.id FROM repositories r
		JOIN namespaces n ON r.namespace_id = n.id
		WHERE n.name = ? AND r.name = ?`, ns, repoName).Scan(&repoID)

	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO manifests (repo_id, reference, media_type, digest, payload) 
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, reference) DO UPDATE SET
			media_type=excluded.media_type,
			digest=excluded.digest,
			payload=excluded.payload
	`, repoID, reference, mediaType, digest, payload)

	return err
}

func (s *SQLiteStore) GetManifest(ctx context.Context, repo, reference string) (mediaType, digest string, payload []byte, err error) {
	ns, repoName := parseRepoPath(repo)

	err = s.db.QueryRowContext(ctx, `
		SELECT m.media_type, m.digest, m.payload 
		FROM manifests m
		JOIN repositories r ON m.repo_id = r.id
		JOIN namespaces n ON r.namespace_id = n.id
		WHERE n.name = ? AND r.name = ? AND (m.reference = ? OR m.digest = ?)`, ns, repoName, reference, reference).
		Scan(&mediaType, &digest, &payload)

	if err == sql.ErrNoRows {
		return "", "", nil, db.ErrNotFound
	}
	return mediaType, digest, payload, err
}

func (s *SQLiteStore) DeleteManifest(ctx context.Context, repo, reference string) error {
	ns, repoName := parseRepoPath(repo)
	
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM manifests 
		WHERE (reference = ? OR digest = ?) AND repo_id IN (
			SELECT r.id FROM repositories r
			JOIN namespaces n ON r.namespace_id = n.id
			WHERE n.name = ? AND r.name = ?
		)`, reference, reference, ns, repoName)
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

func (s *SQLiteStore) ListManifests(ctx context.Context) ([]db.ManifestRecord, error) {
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

func (s *SQLiteStore) PutBlob(ctx context.Context, digest string, size int64, mediaType string) error {
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO blobs (digest, size_bytes, media_type) VALUES (?, ?, ?)", digest, size, mediaType)
	return err
}

func (s *SQLiteStore) BlobExists(ctx context.Context, digest string) (bool, error) {
	var id int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM blobs WHERE digest = ?", digest).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *SQLiteStore) GetBlobSize(ctx context.Context, digest string) (int64, error) {
	var size int64
	err := s.db.QueryRowContext(ctx, "SELECT size_bytes FROM blobs WHERE digest = ?", digest).Scan(&size)
	if err == sql.ErrNoRows {
		return 0, db.ErrNotFound
	}
	return size, err
}

func (s *SQLiteStore) GetUserByToken(ctx context.Context, token string) (*db.User, error) {
	// This method is deprecated - use AuthenticateUser instead
	// Kept for backward compatibility
	var u db.User
	var scopes string
	err := s.db.QueryRowContext(ctx, "SELECT id, username, scopes FROM users WHERE token_hash = ?", token).Scan(&u.ID, &u.Username, &scopes)
	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Scopes = []string{scopes}
	return &u, nil
}

func (s *SQLiteStore) GetUserByUsername(ctx context.Context, username string) (*db.User, error) {
	var u db.User
	var scopes string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, username, token_hash, scopes FROM users WHERE username = ?",
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

func (s *SQLiteStore) ListUsers(ctx context.Context) ([]db.User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, username, scopes FROM users ORDER BY username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []db.User
	for rows.Next() {
		var u db.User
		var scopes string
		if err := rows.Scan(&u.ID, &u.Username, &scopes); err != nil {
			return nil, err
		}
		u.Scopes = []string{scopes}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *SQLiteStore) AuthenticateUser(ctx context.Context, username, password string) (*db.User, error) {
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

func (s *SQLiteStore) CreateUser(ctx context.Context, username, passwordHash string, scopes []string) error {
	scopesJSON := strings.Join(scopes, ",")
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO users (username, token_hash, scopes) VALUES (?, ?, ?)",
		username, passwordHash, scopesJSON,
	)
	return err
}

func (s *SQLiteStore) DeleteUser(ctx context.Context, username string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE username = ?", username)
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

func (s *SQLiteStore) UpdateUser(ctx context.Context, username string, scopes []string) error {
	scopesJSON := strings.Join(scopes, ",")
	result, err := s.db.ExecContext(ctx,
		"UPDATE users SET scopes = ? WHERE username = ?",
		scopesJSON, username,
	)
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

func (s *SQLiteStore) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE users SET token_hash = ? WHERE username = ?",
		passwordHash, username,
	)
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

func (s *SQLiteStore) SaveScanReport(ctx context.Context, digest string, scanner string, data []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scan_reports (digest, scanner, data) 
		VALUES (?, ?, ?)
		ON CONFLICT(digest) DO UPDATE SET
			scanner=excluded.scanner,
			data=excluded.data,
			created_at=CURRENT_TIMESTAMP
	`, digest, scanner, data)
	return err
}

func (s *SQLiteStore) GetScanReport(ctx context.Context, digest string, scanner string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM scan_reports WHERE digest = ? AND scanner = ?", digest, scanner).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	return data, err
}

func (s *SQLiteStore) ListScanReports(ctx context.Context) ([]db.ScanReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT digest, scanner, data
		FROM scan_reports
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []db.ScanReport
	for rows.Next() {
		var r db.ScanReport
		if err := rows.Scan(&r.Digest, &r.Scanner, &r.Data); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

// Helper functions for password hashing
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func verifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// --------------------------------------------------------------------------------
// Quotas & Groups
// --------------------------------------------------------------------------------

func (s *SQLiteStore) CreateGroup(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO groups (name) VALUES (?)", name)
	return err
}

func (s *SQLiteStore) ListGroups(ctx context.Context) ([]db.Group, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM groups ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []db.Group
	for rows.Next() {
		var g db.Group
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *SQLiteStore) ListQuotas(ctx context.Context) ([]db.Quota, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT q.namespace_id, n.name, q.limit_bytes, q.used_bytes 
		FROM quotas q
		JOIN namespaces n ON q.namespace_id = n.id
		ORDER BY n.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quotas []db.Quota
	for rows.Next() {
		var q db.Quota
		if err := rows.Scan(&q.NamespaceID, &q.Namespace, &q.LimitBytes, &q.UsedBytes); err != nil {
			return nil, err
		}
		quotas = append(quotas, q)
	}
	return quotas, rows.Err()
}

func (s *SQLiteStore) AddUserToGroup(ctx context.Context, username, groupName string) error {
	var userID, groupID int

	err := s.db.QueryRowContext(ctx, "SELECT id FROM users WHERE username = ?", username).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}

	err = s.db.QueryRowContext(ctx, "SELECT id FROM groups WHERE name = ?", groupName).Scan(&groupID)
	if err != nil {
		if err == sql.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}

	_, err = s.db.ExecContext(ctx, "INSERT OR IGNORE INTO user_groups (user_id, group_id) VALUES (?, ?)", userID, groupID)
	return err
}

func (s *SQLiteStore) CheckQuota(ctx context.Context, namespace string) (*db.Quota, error) {
	var q db.Quota
	err := s.db.QueryRowContext(ctx, `
		SELECT q.namespace_id, n.name, q.limit_bytes, q.used_bytes 
		FROM quotas q
		JOIN namespaces n ON q.namespace_id = n.id
		WHERE n.name = ?
	`, namespace).Scan(&q.NamespaceID, &q.Namespace, &q.LimitBytes, &q.UsedBytes)
	
	if err == sql.ErrNoRows {
		// No quota set for this namespace, treat as unlimited
		return nil, nil
	}
	return &q, err
}

func (s *SQLiteStore) SetQuota(ctx context.Context, namespace string, limitBytes int64) error {
	err := s.CreateNamespace(ctx, namespace)
	if err != nil {
		return err
	}

	var nsID int
	err = s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = ?", namespace).Scan(&nsID)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO quotas (namespace_id, limit_bytes, used_bytes) 
		VALUES (?, ?, 0)
		ON CONFLICT(namespace_id) DO UPDATE SET limit_bytes=excluded.limit_bytes
	`, nsID, limitBytes)
	return err
}

func (s *SQLiteStore) UpdateQuotaUsage(ctx context.Context, namespace string, sizeDelta int64) error {
	var nsID int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = ?", namespace).Scan(&nsID)
	if err != nil {
		if err == sql.ErrNoRows {
			// If namespace doesn't exist yet, there's no quota tracked for it
			return nil
		}
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE quotas SET used_bytes = used_bytes + ? WHERE namespace_id = ?
	`, sizeDelta, nsID)
	return err
}

// --------------------------------------------------------------------------------
// --------------------------------------------------------------------------------
// Upstream Registries (not implemented in SQLite - use PostgreSQL)
// --------------------------------------------------------------------------------

func (s *SQLiteStore) GetUpstream(ctx context.Context, id int) (map[string]interface{}, error) {
	return nil, errors.New("upstream registries not supported in SQLite - use PostgreSQL")
}

func (s *SQLiteStore) GetUpstreamByName(ctx context.Context, name string) (map[string]interface{}, error) {
	return nil, errors.New("upstream registries not supported in SQLite - use PostgreSQL")
}

func (s *SQLiteStore) ListUpstreams(ctx context.Context) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

// --------------------------------------------------------------------------------
// OCI Artifacts (limited support in SQLite - use PostgreSQL for full features)
// --------------------------------------------------------------------------------

func (s *SQLiteStore) SetArtifactMetadata(ctx context.Context, metadata *db.ArtifactMetadata) error {
	// Basic stub - SQLite has limited artifact support
	return nil
}

func (s *SQLiteStore) GetArtifactMetadata(ctx context.Context, digest string) (*db.ArtifactMetadata, error) {
	return nil, db.ErrNotFound
}

func (s *SQLiteStore) ListReferrers(ctx context.Context, subjectDigest string, artifactType string) ([]db.ReferrerDescriptor, error) {
	// Return empty list for SQLite
	return []db.ReferrerDescriptor{}, nil
}

func (s *SQLiteStore) ListArtifactsByType(ctx context.Context, artifactType string, limit int) ([]db.ArtifactDescriptor, error) {
	// Return empty list for SQLite
	return []db.ArtifactDescriptor{}, nil
}

// --------------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------------
// Access Tokens
// --------------------------------------------------------------------------------

func (s *SQLiteStore) CreateAccessToken(ctx context.Context, userID int, name, tokenHash string, scopes []string, expiresAt *time.Time) (int, error) {
	scopesStr := strings.Join(scopes, ",")
	var tokenID int64
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO access_tokens (user_id, name, token_hash, scopes, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, name, tokenHash, scopesStr, expiresAt,
	)
	if err != nil {
		return 0, err
	}
	tokenID, err = result.LastInsertId()
	return int(tokenID), err
}

func (s *SQLiteStore) ListAccessTokens(ctx context.Context, userID int) ([]db.AccessToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, token_hash, scopes, created_at, last_used_at, expires_at
		 FROM access_tokens WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []db.AccessToken
	for rows.Next() {
		var t db.AccessToken
		var scopesStr string
		var lastUsed, expires sql.NullTime

		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &scopesStr, &t.CreatedAt, &lastUsed, &expires); err != nil {
			return nil, err
		}

		if scopesStr != "" {
			t.Scopes = strings.Split(scopesStr, ",")
		}
		if lastUsed.Valid {
			t.LastUsedAt = &lastUsed.Time
		}
		if expires.Valid {
			t.ExpiresAt = &expires.Time
		}

		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *SQLiteStore) GetAccessTokenByHash(ctx context.Context, tokenHash string) (*db.AccessToken, error) {
	var t db.AccessToken
	var scopesStr string
	var lastUsed, expires sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, token_hash, scopes, created_at, last_used_at, expires_at
		 FROM access_tokens WHERE token_hash = ?`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &scopesStr, &t.CreatedAt, &lastUsed, &expires)

	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if scopesStr != "" {
		t.Scopes = strings.Split(scopesStr, ",")
	}
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		t.ExpiresAt = &expires.Time
	}

	return &t, nil
}

func (s *SQLiteStore) DeleteAccessToken(ctx context.Context, tokenID int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM access_tokens WHERE id = ?`, tokenID)
	return err
}

func (s *SQLiteStore) UpdateAccessTokenLastUsed(ctx context.Context, tokenID int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE access_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`,
		tokenID,
	)
	return err
}

// --------------------------------------------------------------------------------

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
