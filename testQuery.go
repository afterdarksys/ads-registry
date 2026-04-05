package main

import (
	"context"
	"fmt"
	"log"
	
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db/sqlite"
)

func main() {
	cfg := config.DatabaseConfig{
		Driver: "sqlite3",
		DSN:    "data/registry.db?_journal_mode=WAL&_busy_timeout=5000",
	}
	db, err := sqlite.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("DB connected")
	
	mt, d, p, err := db.GetManifest(context.Background(), "alpine", "sha256:16ee56ecca54b715d51fe825bb28ec72918c38b0ad85a763fd8310426336dbd7")
	if err != nil {
		fmt.Printf("GetManifest Error: %v\n", err)
	} else {
		fmt.Printf("Found: %s %s len=%d\n", mt, d, len(p))
	}
}
