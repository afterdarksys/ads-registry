#!/bin/bash
set -e

# Setup data directories with secure permissions
mkdir -p data/blobs
chown -R registry:registry data
chmod -R 750 data

# Check if PostgreSQL is enabled
if [ "$USE_POSTGRES" = "true" ] || [ "$USE_POSTGRES" = "1" ]; then
    echo "=> Setting up built-in PostgreSQL..."
    
    # Setup PostgreSQL data directory
    mkdir -p data/pgdata
    chown -R postgres:postgres data/pgdata
    chmod 700 data/pgdata
    
    mkdir -p /var/run/postgresql
    chown -R postgres:postgres /var/run/postgresql
    
    # Determine postgres binary path dynamically based on installed version on Debian
    PG_BIN=$(ls -d /usr/lib/postgresql/*/bin | head -n 1)
    
    # Initialize DB if empty
    if [ -z "$(ls -A data/pgdata)" ]; then
        echo "=> Creating new PostgreSQL database in data/pgdata..."
        su - postgres -c "$PG_BIN/initdb -D /app/data/pgdata"
        
        # Start temporarily to configure users
        su - postgres -c "$PG_BIN/pg_ctl -w -D /app/data/pgdata start"
        
        # Create user and db default configs
        su - postgres -c "psql -c \"CREATE USER registry WITH PASSWORD 'registry';\""
        su - postgres -c "psql -c \"CREATE DATABASE registry OWNER registry;\""
        su - postgres -c "psql -c \"GRANT ALL PRIVILEGES ON DATABASE registry TO registry;\""
        
        # Stop after setup
        su - postgres -c "$PG_BIN/pg_ctl -w -D /app/data/pgdata stop"
    fi
    
    echo "=> Starting PostgreSQL..."
    su - postgres -c "$PG_BIN/pg_ctl -w -D /app/data/pgdata start"
    
    # Automatically update config.json if jq is available
    if command -v jq >/dev/null 2>&1; then
        echo "=> Updating config.json to use postgres driver..."
        jq '.database.driver = "postgres" | .database.dsn = "postgres://registry:registry@127.0.0.1:5432/registry?sslmode=disable"' config.json > config.json.tmp && mv config.json.tmp config.json
    fi
else
    echo "=> Using SQLite (default). Set USE_POSTGRES=true to use PostgreSQL."
fi

echo "=> Starting ads-registry as user 'registry'..."
# Drop privileges and run as registry user
exec gosu registry "$@"
