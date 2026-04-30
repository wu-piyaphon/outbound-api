package service

import (
	"context"
	"fmt"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/indicator"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
	"github.com/wu-piyaphon/outbound-api/internal/sentiment"
)

type SignalService interface {
	GetAllSignals(ctx context.Context) ([]model.Signal, error)
	CreateSellSignal(ctx context.Context, symbol string, priceAtSignal decimal.Decimal, reasoning string) (*model.Signal, error)
	EvaluateBuySignal(ctx context.Context, symbol string) (*model.Signal, error)
}

type signalService struct {
	signalRepo        repository.SignalRepository
	tradeRepo         repository.TradeRepository
	marketData        *marketdata.Client
	sentimentProvider sentiment.Provider
}

func NewSignalService(
	signalRepo repository.SignalRepository,
	tradeRepo repository.TradeRepository,
	marketData *marketdata.Client,
	sentimentProvider sentiment.Provider,
) SignalService {
	return &signalService{
		signalRepo:        signalRepo,
		tradeRepo:         tradeRepo,
		marketData:        marketData,
		sentimentProvider: sentimentProvider,
	}
}

func (s *signalService) GetAllSignals(ctx context.Context) ([]model.Signal, error) {
	rows, err := s.signalRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

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

func (s *signalService) EvaluateBuySignal(ctx context.Context, symbol string) (*model.Signal, error) {
	hasPosition, err := s.tradeRepo.HasOpenPosition(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("EvaluateBuySignal: checking open position: %w", err)
	}
	if hasPosition {
		return nil, nil
	}

	bars, err := s.marketData.GetBars(symbol, marketdata.GetBarsRequest{
		TimeFrame: marketdata.OneDay,
		Start:     time.Now().AddDate(-1, -2, 0),
		End:       time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("GetBars: %w", err)
	}

	prices := make([]decimal.Decimal, len(bars))
	convertedBars := make([]indicator.Bar, len(bars))

	for i, bar := range bars {
		prices[i] = decimal.NewFromFloat(bar.Close)
		convertedBars[i] = indicator.Bar{
			High:  decimal.NewFromFloat(bar.High),
			Low:   decimal.NewFromFloat(bar.Low),
			Close: decimal.NewFromFloat(bar.Close),
		}
	}

	// Layer 1: Trend — price must be above EMA 200.
	ema, err := indicator.CalculateEMA(prices, 200)
	if err != nil {
		return nil, fmt.Errorf("CalculateEMA: %w", err)
	}

	// Layer 2: Momentum — RSI(14) must be below 35 (oversold).
	rsi, err := indicator.CalculateRSI(prices, 14)
	if err != nil {
		return nil, fmt.Errorf("CalculateRSI: %w", err)
	}

	atr, err := indicator.CalculateATR(convertedBars, 14)
	if err != nil {
		return nil, fmt.Errorf("CalculateATR: %w", err)
	}

	currentPrice := prices[len(prices)-1]

	if !currentPrice.GreaterThan(ema) || !rsi.LessThan(decimal.NewFromInt(35)) {
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
		currentPrice, ema, rsi, atr, sentimentResult.Reasoning,
	)

	signal := &model.Signal{
		ID:            uuid.New(),
		Symbol:        symbol,
		Side:          model.SideBuy,
		PriceAtSignal: currentPrice,
		Indicators:    model.SignalIndicators{EMA: ema, RSI: rsi, ATR: atr},
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
