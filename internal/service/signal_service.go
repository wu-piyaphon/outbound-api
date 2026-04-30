package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/indicator"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
	"github.com/wu-piyaphon/outbound-api/internal/sentiment"
)

// SignalService evaluates market conditions against the five-layer trading
// strategy and persists the resulting buy or sell signals.
type SignalService interface {
	GetAllSignals(ctx context.Context) ([]model.Signal, error)
	CreateSellSignal(ctx context.Context, symbol string, priceAtSignal decimal.Decimal, reasoning string) (*model.Signal, error)
	EvaluateBuySignal(ctx context.Context, symbol string, currentPrice decimal.Decimal) (*model.Signal, error)
}

type signalService struct {
	signalRepo        repository.SignalRepository
	tradeRepo         repository.TradeRepository
	indicators        indicator.IndicatorReader
	sentimentProvider sentiment.Provider
}

// NewSignalService constructs a SignalService backed by the supplied
// repositories, indicator cache, and sentiment provider.
func NewSignalService(
	signalRepo repository.SignalRepository,
	tradeRepo repository.TradeRepository,
	indicators indicator.IndicatorReader,
	sentimentProvider sentiment.Provider,
) SignalService {
	return &signalService{
		signalRepo:        signalRepo,
		tradeRepo:         tradeRepo,
		indicators:        indicators,
		sentimentProvider: sentimentProvider,
	}
}

// GetAllSignals returns every signal record in the database, ordered by the
// repository's default (typically creation time).
func (s *signalService) GetAllSignals(ctx context.Context) ([]model.Signal, error) {
	rows, err := s.signalRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// CreateSellSignal persists a manually triggered sell signal for symbol at the
// given price with the supplied reasoning string. Unlike EvaluateBuySignal, it
// bypasses the five-layer checks and is used for externally driven exits.
func (s *signalService) CreateSellSignal(ctx context.Context, symbol string, priceAtSignal decimal.Decimal, reasoning string) (*model.Signal, error) {
	signal := &model.Signal{
		ID:            uuid.New(),
		Symbol:        symbol,
		Side:          model.SideSell,
		PriceAtSignal: priceAtSignal,
		IsExecuted:    false,
		Indicators:    model.SignalIndicators{},
		Reasoning:     &reasoning,
		CreatedAt:     time.Now().UTC(),
	}

	err := s.signalRepo.Create(ctx, signal)
	if err != nil {
		return nil, fmt.Errorf("Create: %w", err)
	}

	return signal, nil
}

// EvaluateBuySignal runs the five-layer strategy check for symbol at
// currentPrice. It returns a persisted Signal when all layers pass, or nil,
// nil when any layer fails or a position is already open. Errors are returned
// only for infrastructure failures, not for normal negative evaluations.
func (s *signalService) EvaluateBuySignal(ctx context.Context, symbol string, currentPrice decimal.Decimal) (*model.Signal, error) {
	hasPosition, err := s.tradeRepo.HasOpenPosition(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("EvaluateBuySignal: checking open position: %w", err)
	}
	if hasPosition {
		return nil, nil
	}

	// Read pre-computed daily indicators from cache — zero network I/O.
	state, ready := s.indicators.Get(symbol)
	if !ready {
		// Symbol not yet seeded; skip until next daily warm.
		return nil, nil
	}

	// Layer 1: Trend — live price must be above EMA(200).
	if !currentPrice.GreaterThan(state.EMA) {
		return nil, nil
	}

	// Layer 2: Momentum — RSI(14) must be below 35 (oversold).
	if !state.RSI.LessThan(decimal.NewFromInt(35)) {
		return nil, nil
	}

	// Layer 3: Sentiment — news must not be strongly negative.
	sentimentResult, err := s.sentimentProvider.Analyze(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("EvaluateBuySignal: sentiment: %w", err)
	}
	if !sentimentResult.Positive {
		return nil, nil
	}

	reasoning := fmt.Sprintf(
		"Trend+Momentum+Sentiment confirmed. Price: %v, EMA200: %v, RSI14: %v, ATR14: %v. Sentiment: %s",
		currentPrice, state.EMA, state.RSI, state.ATR, sentimentResult.Reasoning,
	)

	signal := &model.Signal{
		ID:            uuid.New(),
		Symbol:        symbol,
		Side:          model.SideBuy,
		PriceAtSignal: currentPrice,
		Indicators:    model.SignalIndicators{EMA: state.EMA, RSI: state.RSI, ATR: state.ATR},
		IsExecuted:    false,
		Reasoning:     &reasoning,
		CreatedAt:     time.Now().UTC(),
	}

	err = s.signalRepo.Create(ctx, signal)
	if err != nil {
		return nil, fmt.Errorf("Create: %w", err)
	}

	return signal, nil
}
