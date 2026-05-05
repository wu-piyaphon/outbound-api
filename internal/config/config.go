package config

import (
	"fmt"
	"os"

	"github.com/shopspring/decimal"
)

type Config struct {
	AlpacaAPIKey    string
	AlpacaAPISecret string
	// AlpacaBaseURL must be set explicitly to either the paper or live endpoint.
	// Leaving it empty risks the SDK defaulting to an unintended environment.
	AlpacaBaseURL string
	DatabaseURL   string
	Port          string
	BotAutoStart  bool
	// BotAPIKey is required to call any bot-control endpoint
	// (/bot/start, /bot/pause, /bot/stop, /bot/status).
	// Set via BOT_API_KEY env var.
	BotAPIKey string

	// RiskPerTradePct is the fraction of available budget risked per trade.
	// Default: 0.01 (1%). Set via RISK_PER_TRADE_PCT env var.
	RiskPerTradePct decimal.Decimal

	// ATRRiskMultiplier drives both position sizing and stop-loss placement so
	// the two remain consistent: stopDistance = ATR × ATRRiskMultiplier.
	// Default: 2.0. Set via ATR_RISK_MULTIPLIER env var.
	ATRRiskMultiplier decimal.Decimal

	// TakeProfitMultiplier is the ATR multiplier for the take-profit level:
	// takeProfit = entryPrice + ATR × TakeProfitMultiplier.
	// Default: 3.0. Set via TAKE_PROFIT_MULTIPLIER env var.
	TakeProfitMultiplier decimal.Decimal
}

func Load() (*Config, error) {
	cfg := &Config{
		AlpacaAPIKey:    os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret: os.Getenv("ALPACA_API_SECRET"),
		AlpacaBaseURL:   os.Getenv("ALPACA_BASE_URL"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		Port:            os.Getenv("PORT"),
		BotAutoStart:    os.Getenv("BOT_AUTOSTART") != "false",
		BotAPIKey:       os.Getenv("BOT_API_KEY"),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	var err error

	riskPercentage := os.Getenv("RISK_PER_TRADE_PCT")
	if riskPercentage == "" {
		riskPercentage = "0.01"
	}
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

	takeProfitMultiplier := os.Getenv("TAKE_PROFIT_MULTIPLIER")
	if takeProfitMultiplier == "" {
		takeProfitMultiplier = "3.0"
	}
	cfg.TakeProfitMultiplier, err = decimal.NewFromString(takeProfitMultiplier)
	if err != nil {
		return nil, fmt.Errorf("config: invalid TAKE_PROFIT_MULTIPLIER: %w", err)
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
	if c.AlpacaBaseURL == "" {
		return fmt.Errorf("ALPACA_BASE_URL is required (set to paper or live endpoint to avoid SDK defaulting to the wrong environment)")
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.BotAPIKey == "" {
		return fmt.Errorf("BOT_API_KEY is required")
	}
	return nil
}
