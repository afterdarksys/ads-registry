-- Migration 013: Ownership Model and Multi-Layered Security Scanning
-- This migration adds ownership tracking, groups, permissions, and comprehensive security scanning

-- ============================================================================
-- OWNERSHIP MODEL
-- ============================================================================

-- Add owner tracking to repositories
ALTER TABLE repositories
ADD COLUMN owner_id INTEGER REFERENCES users(id),
ADD COLUMN owner_group_id INTEGER,
ADD COLUMN created_by_id INTEGER REFERENCES users(id),
ADD COLUMN visibility VARCHAR(20) DEFAULT 'private' CHECK (visibility IN ('public', 'private', 'internal'));

-- Create groups table (can sync from LDAP/AD)
CREATE TABLE IF NOT EXISTS groups (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    ldap_dn VARCHAR(500),              -- LDAP distinguished name
    ad_sid VARCHAR(255),                -- Active Directory SID
    external_id VARCHAR(255),           -- External system ID
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
    role VARCHAR(50) NOT NULL DEFAULT 'member',  -- owner, admin, contributor, member, reader
    added_at TIMESTAMP DEFAULT NOW(),
    added_by_id INTEGER REFERENCES users(id),
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX idx_group_members_user ON group_members(user_id);
CREATE INDEX idx_group_members_group ON group_members(group_id);

-- Repository permissions (user or group based)
CREATE TABLE IF NOT EXISTS repository_permissions (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
    permission VARCHAR(50) NOT NULL,  -- pull, push, delete, admin
    granted_at TIMESTAMP DEFAULT NOW(),
    granted_by_id INTEGER REFERENCES users(id),
    CHECK (user_id IS NOT NULL OR group_id IS NOT NULL),
    CHECK (user_id IS NULL OR group_id IS NULL)  -- exclusive: user OR group, not both
);

CREATE INDEX idx_repo_perms_repo ON repository_permissions(repository_id);
CREATE INDEX idx_repo_perms_user ON repository_permissions(user_id);
CREATE INDEX idx_repo_perms_group ON repository_permissions(group_id);

-- Add foreign key for owner_group_id (had to wait until groups table created)
ALTER TABLE repositories
ADD CONSTRAINT fk_repositories_owner_group
FOREIGN KEY (owner_group_id) REFERENCES groups(id);

-- ============================================================================
-- SECURITY SCANNING ARCHITECTURE
-- ============================================================================

-- Security scan configurations (per-scanner settings)
CREATE TABLE IF NOT EXISTS security_scanners (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,  -- 'trivy', 'clamav', 'semgrep', 'falco'
    scanner_type VARCHAR(50) NOT NULL,   -- 'cve', 'malware', 'static', 'behavioral'
    enabled BOOLEAN DEFAULT true,
    priority INTEGER DEFAULT 100,        -- Lower = higher priority
    timeout_seconds INTEGER DEFAULT 300,
    config JSONB,                        -- Scanner-specific configuration
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_scanners_type ON security_scanners(scanner_type);
CREATE INDEX idx_scanners_enabled ON security_scanners(enabled);

-- Insert default scanners
INSERT INTO security_scanners (name, scanner_type, priority, config) VALUES
('trivy-cve', 'cve', 10, '{"severity": ["CRITICAL", "HIGH", "MEDIUM", "LOW"], "scan_os": true, "scan_app": true}'),
('trivy-secrets', 'static', 20, '{"detect_secrets": true}'),
('clamav', 'malware', 30, '{"scan_archives": true, "max_file_size": 104857600}'),
('yara', 'malware', 40, '{"rules_path": "/etc/yara/rules"}'),
('semgrep', 'static', 50, '{"rulesets": ["security", "owasp-top-10"]}'),
('docker-bench', 'static', 60, '{"check_dockerfile": true}'),
('falco', 'behavioral', 70, '{"runtime_monitoring": true}')
ON CONFLICT (name) DO NOTHING;

-- Security scan jobs (tracks individual scan executions)
CREATE TABLE IF NOT EXISTS security_scan_jobs (
    id SERIAL PRIMARY KEY,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    scanner_id INTEGER REFERENCES security_scanners(id),
    status VARCHAR(50) DEFAULT 'pending',  -- pending, running, completed, failed, skipped
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_ms INTEGER,
    error_message TEXT,
    river_job_id BIGINT,                   -- Link to River job queue
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_scan_jobs_manifest ON security_scan_jobs(manifest_id);
CREATE INDEX idx_scan_jobs_scanner ON security_scan_jobs(scanner_id);
CREATE INDEX idx_scan_jobs_status ON security_scan_jobs(status);
CREATE INDEX idx_scan_jobs_river ON security_scan_jobs(river_job_id);

-- ============================================================================
-- LAYER 1: CVE SCANNING (Already have Trivy)
-- ============================================================================

-- Enhanced vulnerability findings (extends existing Trivy integration)
CREATE TABLE IF NOT EXISTS vulnerability_findings (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,

    -- Vulnerability details
    cve_id VARCHAR(50),
    package_name VARCHAR(255),
    package_version VARCHAR(100),
    fixed_version VARCHAR(100),
    severity VARCHAR(20),  -- CRITICAL, HIGH, MEDIUM, LOW
    cvss_score DECIMAL(3,1),

    -- Metadata
    title TEXT,
    description TEXT,
    references TEXT[],
    published_date TIMESTAMP,

    -- Status tracking
    status VARCHAR(50) DEFAULT 'open',  -- open, acknowledged, false_positive, fixed, wontfix
    acknowledged_by_id INTEGER REFERENCES users(id),
    acknowledged_at TIMESTAMP,
    notes TEXT,

    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_vuln_findings_scan ON vulnerability_findings(scan_job_id);
CREATE INDEX idx_vuln_findings_manifest ON vulnerability_findings(manifest_id);
CREATE INDEX idx_vuln_findings_cve ON vulnerability_findings(cve_id);
CREATE INDEX idx_vuln_findings_severity ON vulnerability_findings(severity);
CREATE INDEX idx_vuln_findings_status ON vulnerability_findings(status);

-- ============================================================================
-- LAYER 2: MALWARE & BAD PACKAGE DETECTION
-- ============================================================================

CREATE TABLE IF NOT EXISTS malware_findings (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,

    -- Malware details
    detection_type VARCHAR(50),  -- virus, trojan, cryptominer, backdoor, typosquatting, suspicious_binary
    signature_name VARCHAR(255),
    file_path VARCHAR(1000),
    file_hash VARCHAR(128),
    threat_level VARCHAR(20),    -- critical, high, medium, low, info

    -- Details
    description TEXT,
    indicators JSONB,            -- IOCs, behavioral patterns, etc.

    -- Status
    status VARCHAR(50) DEFAULT 'detected',  -- detected, quarantined, false_positive, removed
    resolved_by_id INTEGER REFERENCES users(id),
    resolved_at TIMESTAMP,
    resolution_notes TEXT,

    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_malware_findings_scan ON malware_findings(scan_job_id);
CREATE INDEX idx_malware_findings_manifest ON malware_findings(manifest_id);
CREATE INDEX idx_malware_findings_type ON malware_findings(detection_type);
CREATE INDEX idx_malware_findings_threat ON malware_findings(threat_level);
CREATE INDEX idx_malware_findings_status ON malware_findings(status);

-- ============================================================================
-- LAYER 3: STATIC ANALYSIS
-- ============================================================================

CREATE TABLE IF NOT EXISTS static_analysis_findings (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,

    -- Finding details
    rule_id VARCHAR(255),
    rule_name VARCHAR(255),
    category VARCHAR(100),       -- secrets, code_quality, security_pattern, license, dockerfile
    severity VARCHAR(20),

    -- Location
    file_path VARCHAR(1000),
    line_number INTEGER,
    column_number INTEGER,
    code_snippet TEXT,

    -- Details
    message TEXT,
    recommendation TEXT,
    cwe_id VARCHAR(20),          -- Common Weakness Enumeration
    owasp_category VARCHAR(100),

    -- Secret-specific fields
    secret_type VARCHAR(100),    -- api_key, password, private_key, token, aws_access_key, etc.
    secret_entropy DECIMAL(5,2), -- Entropy score for secret detection confidence

    -- Status
    status VARCHAR(50) DEFAULT 'open',
    suppressed BOOLEAN DEFAULT false,
    suppressed_by_id INTEGER REFERENCES users(id),
    suppressed_reason TEXT,

    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_static_findings_scan ON static_analysis_findings(scan_job_id);
CREATE INDEX idx_static_findings_manifest ON static_analysis_findings(manifest_id);
CREATE INDEX idx_static_findings_category ON static_analysis_findings(category);
CREATE INDEX idx_static_findings_severity ON static_analysis_findings(severity);
CREATE INDEX idx_static_findings_status ON static_analysis_findings(status);

-- ============================================================================
-- LAYER 4: BEHAVIORAL ANALYSIS
-- ============================================================================

CREATE TABLE IF NOT EXISTS behavioral_profiles (
    id SERIAL PRIMARY KEY,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,

    -- Runtime characteristics
    baseline_cpu_usage DECIMAL(5,2),
    baseline_memory_mb INTEGER,
    baseline_network_kb_in INTEGER,
    baseline_network_kb_out INTEGER,
    baseline_disk_io_kb INTEGER,

    -- Behavioral patterns
    network_connections JSONB,   -- Expected outbound connections
    file_access_patterns JSONB,  -- Expected file access
    syscall_patterns JSONB,      -- Expected syscalls

    -- Metadata
    profiling_duration_seconds INTEGER,
    sample_count INTEGER,
    confidence_score DECIMAL(3,2),

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_behavioral_profiles_manifest ON behavioral_profiles(manifest_id);

CREATE TABLE IF NOT EXISTS behavioral_anomalies (
    id SERIAL PRIMARY KEY,
    scan_job_id INTEGER REFERENCES security_scan_jobs(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,
    profile_id INTEGER REFERENCES behavioral_profiles(id),

    -- Anomaly details
    anomaly_type VARCHAR(100),   -- unexpected_network, suspicious_syscall, resource_spike, etc.
    severity VARCHAR(20),

    -- Observed vs Expected
    observed_behavior JSONB,
    expected_behavior JSONB,
    deviation_score DECIMAL(5,2),

    -- Details
    description TEXT,
    indicators JSONB,

    -- Status
    status VARCHAR(50) DEFAULT 'detected',
    investigated_by_id INTEGER REFERENCES users(id),
    investigation_notes TEXT,

    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_behavioral_anomalies_scan ON behavioral_anomalies(scan_job_id);
CREATE INDEX idx_behavioral_anomalies_manifest ON behavioral_anomalies(manifest_id);
CREATE INDEX idx_behavioral_anomalies_type ON behavioral_anomalies(anomaly_type);
CREATE INDEX idx_behavioral_anomalies_severity ON behavioral_anomalies(severity);

-- ============================================================================
-- GITHUB SECURITY INTEGRATION
-- ============================================================================

CREATE TABLE IF NOT EXISTS github_security_alerts (
    id SERIAL PRIMARY KEY,
    repository_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
    manifest_id INTEGER REFERENCES manifests(id) ON DELETE CASCADE,

    -- GitHub alert details
    github_alert_id VARCHAR(255),
    github_repo VARCHAR(255),
    alert_type VARCHAR(100),     -- dependabot, code_scanning, secret_scanning
    severity VARCHAR(20),
    state VARCHAR(50),           -- open, dismissed, fixed

    -- Alert content
    title TEXT,
    description TEXT,
    cve_ids TEXT[],
    ghsa_id VARCHAR(50),         -- GitHub Security Advisory ID

    -- Package info (for Dependabot)
    package_ecosystem VARCHAR(100),
    package_name VARCHAR(255),
    affected_version VARCHAR(100),
    patched_version VARCHAR(100),

    -- Timestamps
    github_created_at TIMESTAMP,
    github_updated_at TIMESTAMP,
    dismissed_at TIMESTAMP,
    dismissed_reason TEXT,

    -- Metadata
    raw_payload JSONB,

    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_github_alerts_repo ON github_security_alerts(repository_id);
CREATE INDEX idx_github_alerts_manifest ON github_security_alerts(manifest_id);
CREATE INDEX idx_github_alerts_github_id ON github_security_alerts(github_alert_id);
CREATE INDEX idx_github_alerts_type ON github_security_alerts(alert_type);
CREATE INDEX idx_github_alerts_state ON github_security_alerts(state);

-- ============================================================================
-- AGGREGATED SECURITY POSTURE
-- ============================================================================

-- Per-image security summary (materialized view for dashboard)
CREATE TABLE IF NOT EXISTS security_posture_summary (
    manifest_id INTEGER PRIMARY KEY REFERENCES manifests(id) ON DELETE CASCADE,

    -- Owner info
    owner_id INTEGER REFERENCES users(id),
    repository_id INTEGER REFERENCES repositories(id),

    -- Scan status
    last_scanned_at TIMESTAMP,
    scan_status VARCHAR(50),     -- complete, partial, failed, pending

    -- CVE counts
    cve_critical INTEGER DEFAULT 0,
    cve_high INTEGER DEFAULT 0,
    cve_medium INTEGER DEFAULT 0,
    cve_low INTEGER DEFAULT 0,

    -- Malware counts
    malware_critical INTEGER DEFAULT 0,
    malware_high INTEGER DEFAULT 0,
    malware_detected INTEGER DEFAULT 0,

    -- Static analysis counts
    static_critical INTEGER DEFAULT 0,
    static_high INTEGER DEFAULT 0,
    secrets_found INTEGER DEFAULT 0,

    -- Behavioral
    anomalies_detected INTEGER DEFAULT 0,
    anomalies_critical INTEGER DEFAULT 0,

    -- GitHub
    github_alerts_open INTEGER DEFAULT 0,

    -- Overall risk score (0-100)
    risk_score INTEGER DEFAULT 0,
    risk_level VARCHAR(20),      -- critical, high, medium, low, clean

    -- Compliance
    compliance_status JSONB,     -- PCI-DSS, HIPAA, SOC2, etc.

    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_security_posture_owner ON security_posture_summary(owner_id);
CREATE INDEX idx_security_posture_repo ON security_posture_summary(repository_id);
CREATE INDEX idx_security_posture_risk ON security_posture_summary(risk_level);
CREATE INDEX idx_security_posture_scan_status ON security_posture_summary(scan_status);

-- ============================================================================
-- NOTIFICATION PREFERENCES
-- ============================================================================

CREATE TABLE IF NOT EXISTS security_notification_preferences (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,

    -- Notification channels
    email_enabled BOOLEAN DEFAULT true,
    webhook_enabled BOOLEAN DEFAULT false,
    webhook_url VARCHAR(500),
    slack_enabled BOOLEAN DEFAULT false,
    slack_webhook VARCHAR(500),

    -- Severity thresholds (only notify above this level)
    cve_threshold VARCHAR(20) DEFAULT 'HIGH',
    malware_threshold VARCHAR(20) DEFAULT 'HIGH',
    static_threshold VARCHAR(20) DEFAULT 'CRITICAL',
    behavioral_threshold VARCHAR(20) DEFAULT 'HIGH',

    -- Notification frequency
    immediate_notification BOOLEAN DEFAULT true,
    daily_digest BOOLEAN DEFAULT false,
    weekly_digest BOOLEAN DEFAULT false,

    -- Filters
    only_my_images BOOLEAN DEFAULT true,
    include_group_images BOOLEAN DEFAULT false,

    updated_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- AUDIT LOG
-- ============================================================================

CREATE TABLE IF NOT EXISTS security_audit_log (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    action VARCHAR(100),         -- acknowledge_vuln, suppress_finding, dismiss_alert, etc.
    target_type VARCHAR(50),     -- vulnerability, malware, static_finding, anomaly
    target_id INTEGER,
    before_state VARCHAR(50),
    after_state VARCHAR(50),
    reason TEXT,
    metadata JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_security_audit_user ON security_audit_log(user_id);
CREATE INDEX idx_security_audit_action ON security_audit_log(action);
CREATE INDEX idx_security_audit_created ON security_audit_log(created_at);

-- ============================================================================
-- FUNCTIONS & TRIGGERS
-- ============================================================================

-- Update security_posture_summary when findings change
CREATE OR REPLACE FUNCTION update_security_posture()
RETURNS TRIGGER AS $$
BEGIN
    -- Recalculate counts for the affected manifest
    INSERT INTO security_posture_summary (
        manifest_id,
        owner_id,
        repository_id,
        last_scanned_at,
        cve_critical,
        cve_high,
        cve_medium,
        cve_low,
        malware_critical,
        malware_high,
        malware_detected,
        static_critical,
        static_high,
        secrets_found,
        anomalies_detected,
        anomalies_critical,
        github_alerts_open
    )
    SELECT
        m.id,
        r.owner_id,
        m.repository_id,
        NOW(),
        COUNT(CASE WHEN vf.severity = 'CRITICAL' THEN 1 END),
        COUNT(CASE WHEN vf.severity = 'HIGH' THEN 1 END),
        COUNT(CASE WHEN vf.severity = 'MEDIUM' THEN 1 END),
        COUNT(CASE WHEN vf.severity = 'LOW' THEN 1 END),
        COUNT(CASE WHEN mf.threat_level = 'critical' THEN 1 END),
        COUNT(CASE WHEN mf.threat_level = 'high' THEN 1 END),
        COUNT(DISTINCT mf.id),
        COUNT(CASE WHEN saf.severity = 'CRITICAL' THEN 1 END),
        COUNT(CASE WHEN saf.severity = 'HIGH' THEN 1 END),
        COUNT(CASE WHEN saf.category = 'secrets' THEN 1 END),
        COUNT(DISTINCT ba.id),
        COUNT(CASE WHEN ba.severity = 'CRITICAL' OR ba.severity = 'HIGH' THEN 1 END),
        COUNT(CASE WHEN gsa.state = 'open' THEN 1 END)
    FROM manifests m
    LEFT JOIN repositories r ON m.repository_id = r.id
    LEFT JOIN vulnerability_findings vf ON vf.manifest_id = m.id AND vf.status = 'open'
    LEFT JOIN malware_findings mf ON mf.manifest_id = m.id AND mf.status = 'detected'
    LEFT JOIN static_analysis_findings saf ON saf.manifest_id = m.id AND saf.status = 'open'
    LEFT JOIN behavioral_anomalies ba ON ba.manifest_id = m.id AND ba.status = 'detected'
    LEFT JOIN github_security_alerts gsa ON gsa.manifest_id = m.id AND gsa.state = 'open'
    WHERE m.id = COALESCE(NEW.manifest_id, OLD.manifest_id)
    GROUP BY m.id, r.owner_id, m.repository_id
    ON CONFLICT (manifest_id) DO UPDATE SET
        owner_id = EXCLUDED.owner_id,
        repository_id = EXCLUDED.repository_id,
        last_scanned_at = EXCLUDED.last_scanned_at,
        cve_critical = EXCLUDED.cve_critical,
        cve_high = EXCLUDED.cve_high,
        cve_medium = EXCLUDED.cve_medium,
        cve_low = EXCLUDED.cve_low,
        malware_critical = EXCLUDED.malware_critical,
        malware_high = EXCLUDED.malware_high,
        malware_detected = EXCLUDED.malware_detected,
        static_critical = EXCLUDED.static_critical,
        static_high = EXCLUDED.static_high,
        secrets_found = EXCLUDED.secrets_found,
        anomalies_detected = EXCLUDED.anomalies_detected,
        anomalies_critical = EXCLUDED.anomalies_critical,
        github_alerts_open = EXCLUDED.github_alerts_open,
        updated_at = NOW();

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

-- Triggers to update security posture
CREATE TRIGGER trigger_update_posture_on_vuln
AFTER INSERT OR UPDATE OR DELETE ON vulnerability_findings
FOR EACH ROW EXECUTE FUNCTION update_security_posture();

CREATE TRIGGER trigger_update_posture_on_malware
AFTER INSERT OR UPDATE OR DELETE ON malware_findings
FOR EACH ROW EXECUTE FUNCTION update_security_posture();

CREATE TRIGGER trigger_update_posture_on_static
AFTER INSERT OR UPDATE OR DELETE ON static_analysis_findings
FOR EACH ROW EXECUTE FUNCTION update_security_posture();

CREATE TRIGGER trigger_update_posture_on_behavioral
AFTER INSERT OR UPDATE OR DELETE ON behavioral_anomalies
FOR EACH ROW EXECUTE FUNCTION update_security_posture();

CREATE TRIGGER trigger_update_posture_on_github
AFTER INSERT OR UPDATE OR DELETE ON github_security_alerts
FOR EACH ROW EXECUTE FUNCTION update_security_posture();

-- Auto-set repository owner on creation
CREATE OR REPLACE FUNCTION set_repository_owner()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.owner_id IS NULL THEN
        NEW.owner_id := NEW.created_by_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_set_repo_owner
BEFORE INSERT ON repositories
FOR EACH ROW EXECUTE FUNCTION set_repository_owner();
