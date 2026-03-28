package config

import (
	"fmt"
	"os"
)

type Config struct {
	AlpacaAPIKey    string
	AlpacaAPISecret string
	AlpacaBaseURL   string
	SupabaseURL     string
	SupabaseKey     string
	DatabaseURL     string
	Port            string
}

func Load() (*Config, error) {
	cfg := &Config{
		AlpacaAPIKey:    os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret: os.Getenv("ALPACA_API_SECRET"),
		AlpacaBaseURL:   os.Getenv("ALPACA_BASE_URL"),
		SupabaseURL:     os.Getenv("SUPABASE_URL"),
		SupabaseKey:     os.Getenv("SUPABASE_KEY"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		Port:            os.Getenv("PORT"),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.AlpacaAPIKey == "" {
		return fmt.Errorf("ALPACA_API_KEY is required")
	}
	if c.AlpacaAPISecret == "" {
		return fmt.Errorf("ALPACA_API_SECRET is required")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}
