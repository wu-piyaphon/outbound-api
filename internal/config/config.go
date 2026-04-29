package config

import (
	"fmt"
	"os"

	"github.com/shopspring/decimal"
)

type Config struct {
	AlpacaAPIKey      string
	AlpacaAPISecret   string
	AlpacaBaseURL     string
	SupabaseURL       string
	SupabaseKey       string
	DatabaseURL       string
	Port              string
	BotAutoStart      bool
	RiskPerTradePct   decimal.Decimal
	ATRRiskMultiplier decimal.Decimal
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
		BotAutoStart:    os.Getenv("BOT_AUTOSTART") != "false",
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	riskPercentage := os.Getenv("RISK_PER_TRADE_PCT")
	if riskPercentage == "" {
		riskPercentage = "0.01"
	}
	var err error
	cfg.RiskPerTradePct, err = decimal.NewFromString(riskPercentage)
	if err != nil {
		return nil, fmt.Errorf("config: invalid RISK_PER_TRADE_PCT: %w", err)
	}

	atrMultiplier := os.Getenv("ATR_RISK_MULTIPLIER")
	if atrMultiplier == "" {
		atrMultiplier = "2.0"
	}
	cfg.ATRRiskMultiplier, err = decimal.NewFromString(atrMultiplier)
	if err != nil {
		return nil, fmt.Errorf("config: invalid ATR_RISK_MULTIPLIER: %w", err)
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
