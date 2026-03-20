-- Access Tokens for Docker Registry Authentication
-- This allows users who login via OAuth2/SSO to generate tokens for Docker CLI

CREATE TABLE IF NOT EXISTS access_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL, -- User-friendly name/slug (e.g., "ci-pipeline", "laptop", "prod-server")
    token_hash TEXT NOT NULL UNIQUE, -- bcrypt hash of the actual token
    scopes TEXT NOT NULL, -- Inherited from user or subset (comma-separated)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE, -- Optional expiry
    UNIQUE(user_id, name) -- One token per name per user
);

CREATE INDEX IF NOT EXISTS idx_access_tokens_user_id ON access_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_access_tokens_token_hash ON access_tokens(token_hash);
