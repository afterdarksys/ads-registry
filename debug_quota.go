package main

import (
	"context"
	"fmt"
	"github.com/ryan/ads-registry/internal/config"
	"github.com/ryan/ads-registry/internal/db/sqlite"
)

func main() {
	cfg := config.DatabaseConfig{
		DSN: "data/registry.db",
	}
	db, err := sqlite.New(cfg)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	q, err := db.CheckQuota(context.Background(), "limited-user")
	fmt.Printf("Quota for limited-user: %+v, error: %v\n", q, err)
}
