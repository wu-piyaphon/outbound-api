package config

import (
	"fmt"
	"os"

	"github.com/shopspring/decimal"
)

// StrategyMode selects which coordinator runs bar events.
type StrategyMode string

const (
	// StrategyV1 runs only the live strategy path (default).
	StrategyV1 StrategyMode = "v1"
	// StrategyV2 dual-executes: live path (v1) plus a shadow path that writes
	// to shadow_exit_decisions and signals (mode='shadow') for comparison.
	StrategyV2 StrategyMode = "v2"
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

	// Strategy selects the coordinator mode. Defaults to "v1" (live path only).
	// Set to "v2" to enable dual-execution with shadow logging.
	Strategy StrategyMode

	// Sentiment LLM settings — required when Strategy=v2.
	//
	// SentimentAPIBaseURL is the OpenAI-compatible base URL (no trailing slash).
	// Default: https://api.deepseek.com. Set via SENTIMENT_API_BASE_URL.
	SentimentAPIBaseURL string
	// SentimentAPIKey is the bearer token for the sentiment API.
	// Set via SENTIMENT_API_KEY.
	SentimentAPIKey string
	// SentimentModel is the model name passed in the chat completions request.
	// Default: deepseek-v4-flash. Set via SENTIMENT_MODEL.
	// Note: deepseek-chat and deepseek-reasoner are deprecated 2026-07-24.
	SentimentModel string
	// SentimentMinArticles is the minimum number of news articles required to
	// call the LLM; below this threshold a neutral pass-through is returned.
	// Default: 3. Set via SENTIMENT_MIN_ARTICLES.
	SentimentMinArticles int

	// Regime filter settings — used by the v2 shadow path.
	//
	// RegimeSymbol is the index ticker used for the market regime filter.
	// Default: SPY. Set via REGIME_SYMBOL.
	RegimeSymbol string
	// RegimeEMAPeriod is the EMA period for the regime filter.
	// The shadow buy gate fires only when RegimeSymbol close > EMA(period).
	// Default: 50. Set via REGIME_EMA_PERIOD.
	RegimeEMAPeriod int

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

	// CommissionFeePct is the fractional commission charged per filled trade
	// (applied to notional value). Default: 0.0005 (0.05%).
	// Set via COMMISSION_FEE_PCT env var.
	CommissionFeePct decimal.Decimal

	// FXFeePct is the fractional FX conversion fee amortised per filled trade
	// (applied to notional value). Default: 0.0001 (0.01%).
	// Set via FX_FEE_PCT env var.
	FXFeePct decimal.Decimal

	// Adaptive shadow exit (ATR-based trailing / break-even) — v2 comparison vs v1 static stops.
	// BREAK_EVEN_ATR_TRIGGER default 1.0 — profit in ATR units to lift stop to entry.
	BreakEvenATRTrigger decimal.Decimal
	// TRAIL_ATR_TRIGGER default 1.5 — profit in ATR units to enable trailing stop.
	TrailATRTrigger decimal.Decimal
	// TRAIL_ATR_DISTANCE default 2.0 — trail distance as multiple of entry ATR below peak.
	TrailATRDistance decimal.Decimal
}

func Load() (*Config, error) {
	strategy := StrategyMode(os.Getenv("STRATEGY"))
	if strategy == "" {
		strategy = StrategyV1
	}

	sentimentAPIBaseURL := os.Getenv("SENTIMENT_API_BASE_URL")
	if sentimentAPIBaseURL == "" {
		sentimentAPIBaseURL = "https://api.deepseek.com"
	}
	sentimentModel := os.Getenv("SENTIMENT_MODEL")
	if sentimentModel == "" {
		sentimentModel = "deepseek-v4-flash"
	}
	sentimentMinArticles := 3
	if raw := os.Getenv("SENTIMENT_MIN_ARTICLES"); raw != "" {
		n := 0
		if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n >= 0 {
			sentimentMinArticles = n
		}
	}

	regimeSymbol := os.Getenv("REGIME_SYMBOL")
	if regimeSymbol == "" {
		regimeSymbol = "SPY"
	}
	regimeEMAPeriod := 50
	if raw := os.Getenv("REGIME_EMA_PERIOD"); raw != "" {
		n := 0
		if _, err := fmt.Sscanf(raw, "%d", &n); err == nil && n > 0 {
			regimeEMAPeriod = n
		}
	}

	cfg := &Config{
		AlpacaAPIKey:         os.Getenv("ALPACA_API_KEY"),
		AlpacaAPISecret:      os.Getenv("ALPACA_API_SECRET"),
		AlpacaBaseURL:        os.Getenv("ALPACA_BASE_URL"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		Port:                 os.Getenv("PORT"),
		BotAutoStart:         os.Getenv("BOT_AUTOSTART") != "false",
		BotAPIKey:            os.Getenv("BOT_API_KEY"),
		Strategy:             strategy,
		SentimentAPIBaseURL:  sentimentAPIBaseURL,
		SentimentAPIKey:      os.Getenv("SENTIMENT_API_KEY"),
		SentimentModel:       sentimentModel,
		SentimentMinArticles: sentimentMinArticles,
		RegimeSymbol:         regimeSymbol,
		RegimeEMAPeriod:      regimeEMAPeriod,
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

	commissionFeePct := os.Getenv("COMMISSION_FEE_PCT")
	if commissionFeePct == "" {
		commissionFeePct = "0.0005"
	}
	cfg.CommissionFeePct, err = decimal.NewFromString(commissionFeePct)
	if err != nil {
		return nil, fmt.Errorf("config: invalid COMMISSION_FEE_PCT: %w", err)
	}

	fxFeePct := os.Getenv("FX_FEE_PCT")
	if fxFeePct == "" {
		fxFeePct = "0.0001"
	}
	cfg.FXFeePct, err = decimal.NewFromString(fxFeePct)
	if err != nil {
		return nil, fmt.Errorf("config: invalid FX_FEE_PCT: %w", err)
	}

	breakEvenTrig := os.Getenv("BREAK_EVEN_ATR_TRIGGER")
	if breakEvenTrig == "" {
		breakEvenTrig = "1.0"
	}
	cfg.BreakEvenATRTrigger, err = decimal.NewFromString(breakEvenTrig)
	if err != nil {
		return nil, fmt.Errorf("config: invalid BREAK_EVEN_ATR_TRIGGER: %w", err)
	}

	trailTrig := os.Getenv("TRAIL_ATR_TRIGGER")
	if trailTrig == "" {
		trailTrig = "1.5"
	}
	cfg.TrailATRTrigger, err = decimal.NewFromString(trailTrig)
	if err != nil {
		return nil, fmt.Errorf("config: invalid TRAIL_ATR_TRIGGER: %w", err)
	}

	trailDist := os.Getenv("TRAIL_ATR_DISTANCE")
	if trailDist == "" {
		trailDist = "2.0"
	}
	cfg.TrailATRDistance, err = decimal.NewFromString(trailDist)
	if err != nil {
		return nil, fmt.Errorf("config: invalid TRAIL_ATR_DISTANCE: %w", err)
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
	if c.Strategy != StrategyV1 && c.Strategy != StrategyV2 {
		return fmt.Errorf("STRATEGY must be 'v1' or 'v2', got %q", c.Strategy)
	}
	if c.Strategy == StrategyV2 && c.SentimentAPIKey == "" {
		return fmt.Errorf("SENTIMENT_API_KEY is required when STRATEGY=v2")
	}
	return nil
}
