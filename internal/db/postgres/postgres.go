package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type PostgresStore struct {
	db *sql.DB
}

// DB returns the underlying database connection
func (s *PostgresStore) DB() *sql.DB {
	return s.db
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

	CREATE TABLE IF NOT EXISTS groups (
		id SERIAL PRIMARY KEY,
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

	CREATE TABLE IF NOT EXISTS upstream_registries (
		id SERIAL PRIMARY KEY,
		name TEXT UNIQUE NOT NULL,
		type TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		region TEXT NOT NULL,
		access_key_id TEXT NOT NULL,
		secret_access_key TEXT NOT NULL,
		current_token TEXT,
		token_expiry TIMESTAMP WITH TIME ZONE,
		last_refresh TIMESTAMP WITH TIME ZONE,
		refresh_fail_count INTEGER DEFAULT 0,
		enabled BOOLEAN DEFAULT TRUE,
		cache_enabled BOOLEAN DEFAULT TRUE,
		cache_ttl INTEGER DEFAULT 3600,
		pull_only BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS policies (
		id SERIAL PRIMARY KEY,
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

func (s *PostgresStore) ListRepositories(ctx context.Context, limit int, last string) ([]string, error) {
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
	paramIdx := 1
	if last != "" {
		query += fmt.Sprintf(" WHERE n.name > $%d OR (n.name = $%d AND r.name > $%d)", paramIdx, paramIdx+1, paramIdx+2)
		args = append(args, lastNs, lastNs, lastRepo)
		paramIdx += 3
	}
	query += " ORDER BY n.name, r.name"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", paramIdx)
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

func (s *PostgresStore) ListPolicies(ctx context.Context) ([]db.PolicyRecord, error) {
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

func (s *PostgresStore) AddPolicy(ctx context.Context, expression string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO policies (expression) VALUES ($1) ON CONFLICT(expression) DO NOTHING`, expression)
	return err
}

func (s *PostgresStore) DeletePolicy(ctx context.Context, id int) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM policies WHERE id = $1`, id)
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

func (s *PostgresStore) ListTags(ctx context.Context, repo string, limit int, last string) ([]string, error) {
	ns, repoName := parseRepoPath(repo)

	query := `
		SELECT m.reference
		FROM manifests m
		JOIN repositories r ON m.repo_id = r.id
		JOIN namespaces n ON r.namespace_id = n.id
		WHERE n.name = $1 AND r.name = $2 AND m.reference NOT LIKE 'sha256:%'
	`
	args := []interface{}{ns, repoName}
	paramIdx := 3

	if last != "" {
		query += fmt.Sprintf(" AND m.reference > $%d", paramIdx)
		args = append(args, last)
		paramIdx++
	}
	query += " ORDER BY m.reference"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", paramIdx)
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
		WHERE n.name = $1 AND r.name = $2 AND (m.reference = $3 OR m.digest = $3)`, ns, repoName, reference).
		Scan(&mediaType, &digest, &payload)

	if err == sql.ErrNoRows {
		return "", "", nil, db.ErrNotFound
	}
	return mediaType, digest, payload, err
}

func (s *PostgresStore) DeleteManifest(ctx context.Context, repo, reference string) error {
	ns, repoName := parseRepoPath(repo)

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM manifests 
		WHERE (reference = $1 OR digest = $1) AND repo_id IN (
			SELECT r.id FROM repositories r
			JOIN namespaces n ON r.namespace_id = n.id
			WHERE n.name = $2 AND r.name = $3
		)`, reference, ns, repoName)
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

func (s *PostgresStore) ListScanReports(ctx context.Context) ([]db.ScanReport, error) {
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

func (s *PostgresStore) ListUsers(ctx context.Context) ([]db.User, error) {
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

func (s *PostgresStore) DeleteUser(ctx context.Context, username string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE username = $1", username)
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

func (s *PostgresStore) UpdateUser(ctx context.Context, username string, scopes []string) error {
	scopesJSON := strings.Join(scopes, ",")
	result, err := s.db.ExecContext(ctx,
		"UPDATE users SET scopes = $1 WHERE username = $2",
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

func (s *PostgresStore) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE users SET token_hash = $1 WHERE username = $2",
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

// Helper functions for password hashing
func verifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// --------------------------------------------------------------------------------
// Quotas & Groups
// --------------------------------------------------------------------------------

func (s *PostgresStore) CreateGroup(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO groups (name) VALUES ($1) ON CONFLICT DO NOTHING", name)
	return err
}

func (s *PostgresStore) AddUserToGroup(ctx context.Context, username, groupName string) error {
	var userID, groupID int

	err := s.db.QueryRowContext(ctx, "SELECT id FROM users WHERE username = $1", username).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}

	err = s.db.QueryRowContext(ctx, "SELECT id FROM groups WHERE name = $1", groupName).Scan(&groupID)
	if err != nil {
		if err == sql.ErrNoRows {
			return db.ErrNotFound
		}
		return err
	}

	_, err = s.db.ExecContext(ctx, "INSERT INTO user_groups (user_id, group_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", userID, groupID)
	return err
}

func (s *PostgresStore) ListGroups(ctx context.Context) ([]db.Group, error) {
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

func (s *PostgresStore) ListQuotas(ctx context.Context) ([]db.Quota, error) {
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

func (s *PostgresStore) CheckQuota(ctx context.Context, namespace string) (*db.Quota, error) {
	var q db.Quota
	err := s.db.QueryRowContext(ctx, `
		SELECT q.namespace_id, n.name, q.limit_bytes, q.used_bytes 
		FROM quotas q
		JOIN namespaces n ON q.namespace_id = n.id
		WHERE n.name = $1
	`, namespace).Scan(&q.NamespaceID, &q.Namespace, &q.LimitBytes, &q.UsedBytes)
	
	if err == sql.ErrNoRows {
		// No quota set for this namespace, treat as unlimited
		return nil, nil
	}
	return &q, err
}

func (s *PostgresStore) SetQuota(ctx context.Context, namespace string, limitBytes int64) error {
	err := s.CreateNamespace(ctx, namespace)
	if err != nil {
		return err
	}

	var nsID int
	err = s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = $1", namespace).Scan(&nsID)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO quotas (namespace_id, limit_bytes, used_bytes) 
		VALUES ($1, $2, 0)
		ON CONFLICT(namespace_id) DO UPDATE SET limit_bytes=EXCLUDED.limit_bytes
	`, nsID, limitBytes)
	return err
}

func (s *PostgresStore) UpdateQuotaUsage(ctx context.Context, namespace string, sizeDelta int64) error {
	var nsID int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM namespaces WHERE name = $1", namespace).Scan(&nsID)
	if err != nil {
		if err == sql.ErrNoRows {
			// If namespace doesn't exist yet, there's no quota tracked for it
			return nil
		}
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE quotas SET used_bytes = used_bytes + $1 WHERE namespace_id = $2
	`, sizeDelta, nsID)
	return err
}

// --------------------------------------------------------------------------------
// Upstream Registries
// --------------------------------------------------------------------------------

func (s *PostgresStore) CreateUpstream(ctx context.Context, name, upstreamType, endpoint, region, accessKeyID, secretAccessKey string, enabled, cacheEnabled, pullOnly bool, cacheTTL int) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO upstream_registries
		(name, type, endpoint, region, access_key_id, secret_access_key, enabled, cache_enabled, cache_ttl, pull_only)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, name, upstreamType, endpoint, region, accessKeyID, secretAccessKey, enabled, cacheEnabled, cacheTTL, pullOnly).Scan(&id)
	return id, err
}

func (s *PostgresStore) GetUpstream(ctx context.Context, id int) (map[string]interface{}, error) {
	var upstream map[string]interface{}
	var name, upstreamType, endpoint, region, accessKeyID, secretAccessKey string
	var currentToken sql.NullString
	var tokenExpiry, lastRefresh sql.NullTime
	var refreshFailCount int
	var enabled, cacheEnabled, pullOnly bool
	var cacheTTL int
	var createdAt, updatedAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, endpoint, region, access_key_id, secret_access_key,
		       current_token, token_expiry, last_refresh, refresh_fail_count,
		       enabled, cache_enabled, cache_ttl, pull_only, created_at, updated_at
		FROM upstream_registries WHERE id = $1
	`, id).Scan(&id, &name, &upstreamType, &endpoint, &region, &accessKeyID, &secretAccessKey,
		&currentToken, &tokenExpiry, &lastRefresh, &refreshFailCount,
		&enabled, &cacheEnabled, &cacheTTL, &pullOnly, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	upstream = map[string]interface{}{
		"id":                 id,
		"name":               name,
		"type":               upstreamType,
		"endpoint":           endpoint,
		"region":             region,
		"access_key_id":      accessKeyID,
		"secret_access_key":  secretAccessKey,
		"current_token":      currentToken.String,
		"token_expiry":       tokenExpiry.Time,
		"last_refresh":       lastRefresh.Time,
		"refresh_fail_count": refreshFailCount,
		"enabled":            enabled,
		"cache_enabled":      cacheEnabled,
		"cache_ttl":          cacheTTL,
		"pull_only":          pullOnly,
		"created_at":         createdAt,
		"updated_at":         updatedAt,
	}

	return upstream, nil
}

func (s *PostgresStore) GetUpstreamByName(ctx context.Context, name string) (map[string]interface{}, error) {
	var id int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM upstream_registries WHERE name = $1", name).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetUpstream(ctx, id)
}

func (s *PostgresStore) ListUpstreams(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, endpoint, region, enabled, cache_enabled, pull_only, last_refresh
		FROM upstream_registries
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var upstreams []map[string]interface{}
	for rows.Next() {
		var id int
		var name, upstreamType, endpoint, region string
		var enabled, cacheEnabled, pullOnly bool
		var lastRefresh sql.NullTime

		if err := rows.Scan(&id, &name, &upstreamType, &endpoint, &region, &enabled, &cacheEnabled, &pullOnly, &lastRefresh); err != nil {
			return nil, err
		}

		upstream := map[string]interface{}{
			"id":            id,
			"name":          name,
			"type":          upstreamType,
			"endpoint":      endpoint,
			"region":        region,
			"enabled":       enabled,
			"cache_enabled": cacheEnabled,
			"pull_only":     pullOnly,
			"last_refresh":  lastRefresh.Time,
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, rows.Err()
}

func (s *PostgresStore) UpdateUpstreamToken(ctx context.Context, id int, token string, expiry time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE upstream_registries
		SET current_token = $1, token_expiry = $2, last_refresh = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
	`, token, expiry, id)
	return err
}

func (s *PostgresStore) DeleteUpstream(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM upstream_registries WHERE id = $1", id)
	return err
}

// --------------------------------------------------------------------------------
// OCI Artifacts (Referrers API, Helm Charts, etc.)
// --------------------------------------------------------------------------------

// SetArtifactMetadata stores metadata for an OCI artifact
func (s *PostgresStore) SetArtifactMetadata(ctx context.Context, metadata *db.ArtifactMetadata) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO artifact_metadata (digest, artifact_type, subject_digest, chart_name, chart_version, app_version, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (digest) DO UPDATE
		SET artifact_type = $2, subject_digest = $3, chart_name = $4, chart_version = $5, app_version = $6, metadata_json = $7, updated_at = CURRENT_TIMESTAMP
	`, metadata.Digest, metadata.ArtifactType, metadata.SubjectDigest, metadata.ChartName, metadata.ChartVersion, metadata.AppVersion, metadata.MetadataJSON)

	// Also update the manifests table for easier querying
	if err == nil {
		_, err = s.db.ExecContext(ctx, `
			UPDATE manifests
			SET artifact_type = $1, subject_digest = $2
			WHERE digest = $3
		`, metadata.ArtifactType, metadata.SubjectDigest, metadata.Digest)
	}

	return err
}

// GetArtifactMetadata retrieves metadata for an OCI artifact
func (s *PostgresStore) GetArtifactMetadata(ctx context.Context, digest string) (*db.ArtifactMetadata, error) {
	var metadata db.ArtifactMetadata
	var chartName, chartVersion, appVersion, metadataJSON, subjectDigest sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT digest, artifact_type, subject_digest, chart_name, chart_version, app_version, metadata_json
		FROM artifact_metadata
		WHERE digest = $1
	`, digest).Scan(&metadata.Digest, &metadata.ArtifactType, &subjectDigest, &chartName, &chartVersion, &appVersion, &metadataJSON)

	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	metadata.SubjectDigest = subjectDigest.String
	metadata.ChartName = chartName.String
	metadata.ChartVersion = chartVersion.String
	metadata.AppVersion = appVersion.String
	metadata.MetadataJSON = metadataJSON.String

	return &metadata, nil
}

// ListReferrers implements the OCI Referrers API
// Returns all artifacts that reference the given subject digest
func (s *PostgresStore) ListReferrers(ctx context.Context, subjectDigest string, artifactType string) ([]db.ReferrerDescriptor, error) {
	query := `
		SELECT m.digest, m.media_type, COALESCE(m.artifact_type, '') as artifact_type, m.size
		FROM manifests m
		WHERE m.subject_digest = $1
	`
	args := []interface{}{subjectDigest}

	if artifactType != "" {
		query += " AND m.artifact_type = $2"
		args = append(args, artifactType)
	}

	query += " ORDER BY m.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var referrers []db.ReferrerDescriptor
	for rows.Next() {
		var ref db.ReferrerDescriptor
		if err := rows.Scan(&ref.Digest, &ref.MediaType, &ref.ArtifactType, &ref.Size); err != nil {
			return nil, err
		}
		referrers = append(referrers, ref)
	}

	return referrers, rows.Err()
}

// ListArtifactsByType lists all artifacts of a specific type
func (s *PostgresStore) ListArtifactsByType(ctx context.Context, artifactType string, limit int) ([]db.ArtifactDescriptor, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			m.digest,
			n.name as namespace,
			r.name as repo,
			m.artifact_type,
			m.media_type,
			m.size,
			m.created_at,
			COALESCE(am.chart_name, ''),
			COALESCE(am.chart_version, ''),
			COALESCE(am.app_version, '')
		FROM manifests m
		INNER JOIN repositories r ON m.repo_id = r.id
		INNER JOIN namespaces n ON r.namespace_id = n.id
		LEFT JOIN artifact_metadata am ON m.digest = am.digest
		WHERE m.artifact_type = $1
		ORDER BY m.created_at DESC
		LIMIT $2
	`, artifactType, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []db.ArtifactDescriptor
	for rows.Next() {
		var artifact db.ArtifactDescriptor
		var createdAt time.Time

		err := rows.Scan(
			&artifact.Digest,
			&artifact.Namespace,
			&artifact.Repo,
			&artifact.ArtifactType,
			&artifact.MediaType,
			&artifact.Size,
			&createdAt,
			&artifact.ChartName,
			&artifact.ChartVersion,
			&artifact.AppVersion,
		)
		if err != nil {
			return nil, err
		}

		artifact.CreatedAt = createdAt.Format(time.RFC3339)
		artifacts = append(artifacts, artifact)
	}

	return artifacts, rows.Err()
}

// --------------------------------------------------------------------------------
// Helpers
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

// --------------------------------------------------------------------------------
// Access Tokens
// --------------------------------------------------------------------------------

func (s *PostgresStore) CreateAccessToken(ctx context.Context, userID int, name, tokenHash string, scopes []string, expiresAt *time.Time) (int, error) {
	scopesStr := strings.Join(scopes, ",")
	var tokenID int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO access_tokens (user_id, name, token_hash, scopes, expires_at) 
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, name, tokenHash, scopesStr, expiresAt,
	).Scan(&tokenID)
	return tokenID, err
}

func (s *PostgresStore) ListAccessTokens(ctx context.Context, userID int) ([]db.AccessToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, scopes, created_at, last_used_at, expires_at 
		 FROM access_tokens WHERE user_id = $1 ORDER BY created_at DESC`,
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
		var lastUsed sql.NullTime
		var expires sql.NullTime

		err := rows.Scan(&t.ID, &t.UserID, &t.Name, &scopesStr, &t.CreatedAt, &lastUsed, &expires)
		if err != nil {
			return nil, err
		}

		t.Scopes = []string{scopesStr}
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

func (s *PostgresStore) GetAccessTokenByHash(ctx context.Context, tokenHash string) (*db.AccessToken, error) {
	var t db.AccessToken
	var scopesStr string
	var lastUsed sql.NullTime
	var expires sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, scopes, created_at, last_used_at, expires_at 
		 FROM access_tokens WHERE token_hash = $1`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.Name, &scopesStr, &t.CreatedAt, &lastUsed, &expires)

	if err == sql.ErrNoRows {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	t.Scopes = []string{scopesStr}
	t.TokenHash = tokenHash
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		t.ExpiresAt = &expires.Time
	}

	return &t, nil
}

func (s *PostgresStore) DeleteAccessToken(ctx context.Context, tokenID int) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM access_tokens WHERE id = $1", tokenID)
	return err
}

func (s *PostgresStore) UpdateAccessTokenLastUsed(ctx context.Context, tokenID int) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE access_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1",
		tokenID,
	)
	return err
}
