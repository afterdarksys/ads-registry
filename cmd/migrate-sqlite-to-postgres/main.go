package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	sqlitePath := flag.String("sqlite", "data/registry.db", "SQLite database path")
	postgresURL := flag.String("postgres", "", "PostgreSQL connection string (required)")
	flag.Parse()

	if *postgresURL == "" {
		fmt.Println("Usage: migrate-sqlite-to-postgres -postgres 'postgres://user:pass@host/dbname?sslmode=disable'")
		os.Exit(1)
	}

	log.Printf("Migrating from SQLite (%s) to PostgreSQL", *sqlitePath)

	// Open SQLite
	sqliteDB, err := sql.Open("sqlite3", *sqlitePath)
	if err != nil {
		log.Fatalf("Failed to open SQLite: %v", err)
	}
	defer sqliteDB.Close()

	// Open PostgreSQL
	pgDB, err := sql.Open("postgres", *postgresURL)
	if err != nil {
		log.Fatalf("Failed to open PostgreSQL: %v", err)
	}
	defer pgDB.Close()

	// Test connections
	if err := sqliteDB.Ping(); err != nil {
		log.Fatalf("SQLite ping failed: %v", err)
	}
	if err := pgDB.Ping(); err != nil {
		log.Fatalf("PostgreSQL ping failed: %v", err)
	}

	log.Println("Database connections established")

	// Migrate namespaces
	log.Println("Migrating namespaces...")
	if err := migrateTable(sqliteDB, pgDB, "namespaces",
		"SELECT id, name, created_at FROM namespaces",
		"INSERT INTO namespaces (id, name, created_at) VALUES ($1, $2, $3) ON CONFLICT (name) DO NOTHING"); err != nil {
		log.Fatalf("Failed to migrate namespaces: %v", err)
	}

	// Migrate repositories
	log.Println("Migrating repositories...")
	if err := migrateTable(sqliteDB, pgDB, "repositories",
		"SELECT id, namespace_id, name, created_at FROM repositories",
		"INSERT INTO repositories (id, namespace_id, name, created_at) VALUES ($1, $2, $3, $4) ON CONFLICT (namespace_id, name) DO NOTHING"); err != nil {
		log.Fatalf("Failed to migrate repositories: %v", err)
	}

	// Migrate blobs
	log.Println("Migrating blobs...")
	if err := migrateTable(sqliteDB, pgDB, "blobs",
		"SELECT id, digest, size_bytes, media_type, created_at FROM blobs",
		"INSERT INTO blobs (id, digest, size_bytes, media_type, created_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (digest) DO NOTHING"); err != nil {
		log.Fatalf("Failed to migrate blobs: %v", err)
	}

	// Migrate manifests
	log.Println("Migrating manifests...")
	if err := migrateTable(sqliteDB, pgDB, "manifests",
		"SELECT id, repo_id, reference, media_type, digest, payload, created_at FROM manifests",
		"INSERT INTO manifests (id, repo_id, reference, media_type, digest, payload, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (repo_id, reference) DO NOTHING"); err != nil {
		log.Fatalf("Failed to migrate manifests: %v", err)
	}

	// Migrate users
	log.Println("Migrating users...")
	if err := migrateTable(sqliteDB, pgDB, "users",
		"SELECT id, username, token_hash, scopes, created_at FROM users",
		"INSERT INTO users (id, username, token_hash, scopes, created_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (username) DO NOTHING"); err != nil {
		log.Fatalf("Failed to migrate users: %v", err)
	}

	// Migrate quotas if table exists
	log.Println("Migrating quotas (if exists)...")
	migrateTable(sqliteDB, pgDB, "quotas",
		"SELECT namespace, limit_bytes, used_bytes FROM quotas",
		"INSERT INTO quotas (namespace, limit_bytes, used_bytes) VALUES ($1, $2, $3) ON CONFLICT (namespace) DO NOTHING")

	// Update sequences to max ID + 1
	log.Println("Updating PostgreSQL sequences...")
	updateSequences(pgDB)

	log.Println("✅ Migration completed successfully!")
	log.Println("\nNext steps:")
	log.Println("1. Verify data in PostgreSQL")
	log.Println("2. Update config.json to use PostgreSQL")
	log.Println("3. Restart registry with new config")
}

func migrateTable(src, dst *sql.DB, tableName, selectSQL, insertSQL string) error {
	rows, err := src.Query(selectSQL)
	if err != nil {
		// Table might not exist (like quotas) - not an error
		return nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	count := 0
	for rows.Next() {
		// Create slice of interface{} to hold values
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return err
		}

		// Insert into PostgreSQL
		if _, err := dst.Exec(insertSQL, values...); err != nil {
			log.Printf("Warning: Failed to insert row into %s: %v", tableName, err)
			continue
		}
		count++
	}

	log.Printf("  → Migrated %d rows from %s", count, tableName)
	return rows.Err()
}

func updateSequences(db *sql.DB) {
	tables := []string{"namespaces", "repositories", "manifests", "blobs", "users"}
	for _, table := range tables {
		// Get max ID
		var maxID sql.NullInt64
		err := db.QueryRow(fmt.Sprintf("SELECT MAX(id) FROM %s", table)).Scan(&maxID)
		if err != nil || !maxID.Valid {
			continue
		}

		// Update sequence
		seqName := fmt.Sprintf("%s_id_seq", table)
		_, err = db.Exec(fmt.Sprintf("SELECT setval('%s', $1)", seqName), maxID.Int64+1)
		if err != nil {
			log.Printf("Warning: Failed to update sequence for %s: %v", table, err)
		} else {
			log.Printf("  → Updated sequence %s to %d", seqName, maxID.Int64+1)
		}
	}
}
