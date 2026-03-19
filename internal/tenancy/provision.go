package tenancy

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// provisionTenantSchema provisions a new tenant schema with all necessary tables
// This creates a complete isolated schema for a tenant based on migrations 013 and 014
func provisionTenantSchema(ctx context.Context, tx *sql.Tx, schemaName string) error {
	// Create the schema
	_, err := tx.ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaName))
	if err != nil {
		return fmt.Errorf("failed to create schema %s: %w", schemaName, err)
	}

	// Set search path to new schema for subsequent operations
	_, err = tx.ExecContext(ctx, fmt.Sprintf("SET search_path TO %s, public", schemaName))
	if err != nil {
		return fmt.Errorf("failed to set search path: %w", err)
	}

	// Execute tenant schema DDL
	ddl := getTenantSchemaDDL()

	// Split DDL into individual statements and execute
	statements := splitSQLStatements(ddl)
	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}

		_, err = tx.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("failed to execute statement %d in tenant schema: %w\nStatement: %s", i, err, stmt)
		}
	}

	// Reset search path to public
	_, err = tx.ExecContext(ctx, "SET search_path TO public")
	if err != nil {
		return fmt.Errorf("failed to reset search path: %w", err)
	}

	return nil
}

// getTenantSchemaDDL returns the complete DDL for a tenant schema
// This includes all tables from migrations 013 and 014, but adapted for tenant isolation
func getTenantSchemaDDL() string {
	return `
-- ============================================================================
-- TENANT SCHEMA DDL (Based on migrations 013 & 014)
-- This DDL creates all tables needed for a tenant in their isolated schema
-- ============================================================================

-- Core registry tables (namespaces, repositories, manifests, blobs)
CREATE TABLE IF NOT EXISTS namespaces (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS repositories (
    id SERIAL PRIMARY KEY,
    namespace_id INTEGER REFERENCES namespaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    owner_id INTEGER,
    owner_group_id INTEGER,
    created_by_id INTEGER,
    visibility VARCHAR(20) DEFAULT 'private' CHECK (visibility IN ('public', 'private', 'internal')),
    UNIQUE (namespace_id, name)
);

CREATE INDEX idx_repositories_namespace ON repositories(namespace_id);
CREATE INDEX idx_repositories_owner ON repositories(owner_id);
CREATE INDEX idx_repositories_created_at ON repositories(created_at);

CREATE TABLE IF NOT EXISTS manifests (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    digest VARCHAR(255) UNIQUE NOT NULL,
    media_type VARCHAR(255) NOT NULL,
    payload BYTEA NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    artifact_type VARCHAR(255),
    subject_digest VARCHAR(255),
    size BIGINT
);

CREATE INDEX idx_manifests_repository ON manifests(repository_id);
CREATE INDEX idx_manifests_digest ON manifests(digest);
CREATE INDEX idx_manifests_artifact_type ON manifests(artifact_type);
CREATE INDEX idx_manifests_subject_digest ON manifests(subject_digest);

CREATE TABLE IF NOT EXISTS tags (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (repository_id, name)
);

CREATE INDEX idx_tags_repository ON tags(repository_id);
CREATE INDEX idx_tags_manifest ON tags(manifest_id);

CREATE TABLE IF NOT EXISTS blobs (
    id SERIAL PRIMARY KEY,
    digest VARCHAR(255) UNIQUE NOT NULL,
    size_bytes BIGINT NOT NULL,
    media_type VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_blobs_digest ON blobs(digest);

CREATE TABLE IF NOT EXISTS blob_uploads (
    id SERIAL PRIMARY KEY,
    uuid VARCHAR(255) UNIQUE NOT NULL,
    repository_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    started_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    digest VARCHAR(255),
    size_bytes BIGINT DEFAULT 0
);

-- Users and authentication
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    scopes TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_users_username ON users(username);

-- Groups (for LDAP/AD sync)
CREATE TABLE IF NOT EXISTS groups (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    ldap_dn VARCHAR(500),
    ad_sid VARCHAR(255),
    external_id VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by_id INTEGER REFERENCES users(id)
);

CREATE INDEX idx_groups_ldap_dn ON groups(ldap_dn);
CREATE INDEX idx_groups_ad_sid ON groups(ad_sid);

-- Group memberships with roles
CREATE TABLE IF NOT EXISTS group_members (
    group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'member',
    added_at TIMESTAMP DEFAULT NOW(),
    added_by_id INTEGER REFERENCES users(id),
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX idx_group_members_user ON group_members(user_id);
CREATE INDEX idx_group_members_group ON group_members(group_id);

-- Repository permissions
CREATE TABLE IF NOT EXISTS repository_permissions (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
    permission VARCHAR(50) NOT NULL,
    granted_at TIMESTAMP DEFAULT NOW(),
    granted_by_id INTEGER REFERENCES users(id),
    CHECK (user_id IS NOT NULL OR group_id IS NOT NULL),
    CHECK (user_id IS NULL OR group_id IS NULL)
);

CREATE INDEX idx_repo_perms_repo ON repository_permissions(repository_id);
CREATE INDEX idx_repo_perms_user ON repository_permissions(user_id);
CREATE INDEX idx_repo_perms_group ON repository_permissions(group_id);

-- Add foreign key for owner_group_id
ALTER TABLE repositories
ADD CONSTRAINT fk_repositories_owner_group
FOREIGN KEY (owner_group_id) REFERENCES groups(id);

-- Quotas
CREATE TABLE IF NOT EXISTS quotas (
    id SERIAL PRIMARY KEY,
    namespace_id INTEGER REFERENCES namespaces(id) ON DELETE CASCADE,
    limit_bytes BIGINT NOT NULL,
    used_bytes BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Scan reports
CREATE TABLE IF NOT EXISTS scan_reports (
    id SERIAL PRIMARY KEY,
    digest VARCHAR(255) UNIQUE NOT NULL,
    scanner VARCHAR(100) NOT NULL,
    payload BYTEA,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_scan_reports_digest ON scan_reports(digest);

-- Security scanners configuration
CREATE TABLE IF NOT EXISTS security_scanners (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    scanner_type VARCHAR(50) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    priority INTEGER DEFAULT 100,
    timeout_seconds INTEGER DEFAULT 300,
    config JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_scanners_type ON security_scanners(scanner_type);
CREATE INDEX idx_scanners_enabled ON security_scanners(enabled);

-- Security scan jobs
CREATE TABLE IF NOT EXISTS security_scan_jobs (
    id SERIAL PRIMARY KEY,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    scanner_id INTEGER REFERENCES security_scanners(id) ON DELETE CASCADE,
    status VARCHAR(50) DEFAULT 'pending',
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_ms INTEGER,
    error_message TEXT,
    river_job_id BIGINT,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_scan_jobs_manifest ON security_scan_jobs(manifest_id);
CREATE INDEX idx_scan_jobs_scanner ON security_scan_jobs(scanner_id);
CREATE INDEX idx_scan_jobs_status ON security_scan_jobs(status);

-- Vulnerability findings
CREATE TABLE IF NOT EXISTS vulnerability_findings (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    cve_id VARCHAR(50),
    package_name VARCHAR(255),
    package_version VARCHAR(100),
    severity VARCHAR(20),
    cvss_score DECIMAL(3, 1),
    description TEXT,
    fix_version VARCHAR(100),
    status VARCHAR(50) DEFAULT 'open',
    acknowledged_at TIMESTAMP,
    acknowledged_by_id INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_vuln_findings_manifest ON vulnerability_findings(manifest_id);
CREATE INDEX idx_vuln_findings_severity ON vulnerability_findings(severity);
CREATE INDEX idx_vuln_findings_status ON vulnerability_findings(status);

-- Malware findings
CREATE TABLE IF NOT EXISTS malware_findings (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    detection_type VARCHAR(100),
    threat_level VARCHAR(20),
    file_path VARCHAR(1000),
    signature_name VARCHAR(255),
    indicators JSONB,
    status VARCHAR(50) DEFAULT 'open',
    resolved_at TIMESTAMP,
    resolved_by_id INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_malware_findings_manifest ON malware_findings(manifest_id);
CREATE INDEX idx_malware_findings_threat_level ON malware_findings(threat_level);

-- Static analysis findings
CREATE TABLE IF NOT EXISTS static_analysis_findings (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    category VARCHAR(100),
    severity VARCHAR(20),
    rule_id VARCHAR(255),
    file_path VARCHAR(1000),
    line_number INTEGER,
    column_number INTEGER,
    message TEXT,
    secret_value_hash VARCHAR(64),
    secret_entropy DECIMAL(5, 2),
    cwe_id VARCHAR(20),
    owasp_category VARCHAR(100),
    suppressed BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_static_findings_manifest ON static_analysis_findings(manifest_id);
CREATE INDEX idx_static_findings_severity ON static_analysis_findings(severity);

-- Behavioral profiles
CREATE TABLE IF NOT EXISTS behavioral_profiles (
    id SERIAL PRIMARY KEY,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    baseline_cpu_percent DECIMAL(5, 2),
    baseline_memory_mb INTEGER,
    network_connections JSONB,
    file_access_patterns JSONB,
    syscall_patterns JSONB,
    confidence_score DECIMAL(5, 2),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_behavioral_profiles_manifest ON behavioral_profiles(manifest_id);

-- Behavioral anomalies
CREATE TABLE IF NOT EXISTS behavioral_anomalies (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    profile_id INTEGER REFERENCES behavioral_profiles(id),
    anomaly_type VARCHAR(100),
    deviation_score DECIMAL(5, 2),
    description TEXT,
    status VARCHAR(50) DEFAULT 'open',
    investigated_at TIMESTAMP,
    investigated_by_id INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_behavioral_anomalies_manifest ON behavioral_anomalies(manifest_id);

-- Security audit log
CREATE TABLE IF NOT EXISTS security_audit_log (
    id BIGSERIAL PRIMARY KEY,
    action VARCHAR(100) NOT NULL,
    user_id INTEGER REFERENCES users(id),
    resource_type VARCHAR(50),
    resource_id INTEGER,
    details JSONB,
    ip_address INET,
    user_agent TEXT,
    before_state JSONB,
    after_state JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_security_audit_action ON security_audit_log(action);
CREATE INDEX idx_security_audit_created_at ON security_audit_log(created_at);

-- OCI Artifacts metadata (Migration 014)
CREATE TABLE IF NOT EXISTS artifact_metadata (
    id SERIAL PRIMARY KEY,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE UNIQUE,
    artifact_type VARCHAR(255) NOT NULL,
    subject_digest VARCHAR(255),
    chart_name VARCHAR(255),
    chart_version VARCHAR(100),
    app_version VARCHAR(100),
    metadata_json JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_artifact_metadata_type ON artifact_metadata(artifact_type);
CREATE INDEX idx_artifact_metadata_subject ON artifact_metadata(subject_digest);
CREATE INDEX idx_artifact_helm_chart ON artifact_metadata(chart_name, chart_version);

-- Views for OCI artifacts
CREATE OR REPLACE VIEW v_artifact_referrers AS
SELECT
    m.subject_digest,
    m.digest AS referrer_digest,
    m.artifact_type,
    m.media_type,
    m.size,
    am.metadata_json,
    m.created_at
FROM manifests m
LEFT JOIN artifact_metadata am ON am.manifest_id = m.id
WHERE m.subject_digest IS NOT NULL;

CREATE OR REPLACE VIEW v_helm_charts AS
SELECT
    r.id AS repository_id,
    n.name || '/' || r.name AS repository_path,
    am.chart_name,
    am.chart_version,
    am.app_version,
    m.digest,
    m.size,
    am.metadata_json,
    m.created_at
FROM artifact_metadata am
JOIN manifests m ON m.id = am.manifest_id
JOIN repositories r ON r.id = m.repository_id
JOIN namespaces n ON n.id = r.namespace_id
WHERE am.artifact_type LIKE '%helm.chart%';

-- Trigger to auto-set repository owner
CREATE OR REPLACE FUNCTION set_repository_owner()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.owner_id IS NULL AND NEW.created_by_id IS NOT NULL THEN
        NEW.owner_id := NEW.created_by_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_set_repository_owner
BEFORE INSERT ON repositories
FOR EACH ROW
EXECUTE FUNCTION set_repository_owner();

-- Insert default security scanners
INSERT INTO security_scanners (name, scanner_type, priority, config) VALUES
('trivy-cve', 'cve', 10, '{"severity": ["CRITICAL", "HIGH", "MEDIUM", "LOW"], "scan_os": true, "scan_app": true}'),
('trivy-secrets', 'static', 20, '{"detect_secrets": true}'),
('clamav', 'malware', 30, '{"scan_archives": true, "max_file_size": 104857600}'),
('yara', 'malware', 40, '{"rules_path": "/etc/yara/rules"}'),
('semgrep', 'static', 50, '{"rulesets": ["security", "owasp-top-10"]}'),
('docker-bench', 'static', 60, '{"check_dockerfile": true}'),
('falco', 'behavioral', 70, '{"runtime_monitoring": true}')
ON CONFLICT (name) DO NOTHING;
`
}

// splitSQLStatements splits a multi-statement SQL string into individual statements
func splitSQLStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	lines := strings.Split(sql, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comment lines
		if strings.HasPrefix(trimmed, "--") {
			continue
		}

		for _, char := range line {
			// Track quote state
			if char == '\'' || char == '"' {
				if !inQuote {
					inQuote = true
					quoteChar = char
				} else if char == quoteChar {
					inQuote = false
				}
			}

			// Detect statement end (semicolon outside quotes)
			if char == ';' && !inQuote {
				current.WriteRune(char)
				statements = append(statements, current.String())
				current.Reset()
				continue
			}

			current.WriteRune(char)
		}
		current.WriteRune('\n')
	}

	// Add remaining statement if any
	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}

// ReadMigrationFiles reads migration files from disk for tenant provisioning
// This is used in development when migrations are updated
func ReadMigrationFiles(migrationsDir string) (string, error) {
	// Read migration 013
	migration013, err := os.ReadFile(filepath.Join(migrationsDir, "013_ownership_and_security.sql"))
	if err != nil {
		return "", fmt.Errorf("failed to read migration 013: %w", err)
	}

	// Read migration 014
	migration014, err := os.ReadFile(filepath.Join(migrationsDir, "014_oci_artifacts.sql"))
	if err != nil {
		return "", fmt.Errorf("failed to read migration 014: %w", err)
	}

	// Combine migrations
	combined := string(migration013) + "\n\n" + string(migration014)
	return combined, nil
}
