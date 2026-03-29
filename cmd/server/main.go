package main

import (
	"context"
	"log"
	"time"

	"github.com/joho/godotenv"
	"github.com/wu-piyaphon/outbound-api/internal/config"
	"github.com/wu-piyaphon/outbound-api/internal/database"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to the database: %v", err)
	}
	defer pool.Close()

	log.Println("Successfully connected to the database")

	database.Migrate(cfg.DatabaseURL)
}
