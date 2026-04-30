package service

import (
	"context"
	"testing"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockTradeRepo struct {
	created       []model.Trade
	updated       []model.Trade
	openTrades    []*model.Trade
	byAlpacaOrder map[string]*model.Trade
	hasOpenPos    bool
	createErr     error
}

func (m *mockTradeRepo) Create(_ context.Context, t model.Trade) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.created = append(m.created, t)
	return nil
}

func (m *mockTradeRepo) GetOpenBuyTradesBySymbol(_ context.Context, _ string) ([]*model.Trade, error) {
	return m.openTrades, nil
}

func (m *mockTradeRepo) GetByAlpacaOrderID(_ context.Context, id string) (*model.Trade, error) {
	t, ok := m.byAlpacaOrder[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (m *mockTradeRepo) Update(_ context.Context, t model.Trade) error {
	m.updated = append(m.updated, t)
	return nil
}

func (m *mockTradeRepo) HasOpenPosition(_ context.Context, _ string) (bool, error) {
	return m.hasOpenPos, nil
}

type mockAccountTransferRepo struct {
	decremented int
	incremented int
	budget      *model.AccountTransfer
}

func (m *mockAccountTransferRepo) Create(_ context.Context, _ *model.AccountTransfer) error {
	return nil
}

func (m *mockAccountTransferRepo) GetAvailableBudget(_ context.Context) (*model.AccountTransfer, error) {
	return m.budget, nil
}

func (m *mockAccountTransferRepo) DecrementRemainingTrades(_ context.Context, _ uuid.UUID) error {
	m.decremented++
	return nil
}

func (m *mockAccountTransferRepo) IncrementRemainingTrades(_ context.Context, _ uuid.UUID) error {
	m.incremented++
	return nil
}

type mockSignalRepo struct{}

func (m *mockSignalRepo) GetAll(_ context.Context) ([]model.Signal, error) { return nil, nil }
func (m *mockSignalRepo) Create(_ context.Context, _ *model.Signal) error  { return nil }

type mockTransactor struct{}

func (m *mockTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// mockOrderPlacer records every PlaceOrder call so tests can assert on them.
type mockOrderPlacer struct {
	placedOrders []alpaca.PlaceOrderRequest
	returnErr    error
}

func (m *mockOrderPlacer) PlaceOrder(req alpaca.PlaceOrderRequest) (*alpaca.Order, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	m.placedOrders = append(m.placedOrders, req)
	id := uuid.New().String()
	return &alpaca.Order{ID: id}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestTradeService(tradeRepo repository.TradeRepository, atRepo repository.AccountTransferRepository) TradeService {
	return newTestTradeServiceWith(tradeRepo, atRepo, &mockOrderPlacer{})
}

func newTestTradeServiceWith(tradeRepo repository.TradeRepository, atRepo repository.AccountTransferRepository, placer OrderPlacer) TradeService {
	return NewTradeService(
		tradeRepo,
		atRepo,
		&mockSignalRepo{},
		&mockTransactor{},
		placer,
		decimal.NewFromFloat(0.01),  // riskPerTradePct
		decimal.NewFromFloat(2.0),   // atrRiskMultiplier
		decimal.NewFromFloat(3.0),   // takeProfitMultiplier
	)
}

// ---------------------------------------------------------------------------
// Guard / validation tests
// ---------------------------------------------------------------------------

func TestExecuteBuyTrade_NilAccount(t *testing.T) {
	svc := newTestTradeService(&mockTradeRepo{}, &mockAccountTransferRepo{})
	signal := &model.Signal{
		ID:            uuid.New(),
		Symbol:        "AAPL",
		PriceAtSignal: decimal.NewFromFloat(150),
		Indicators:    model.SignalIndicators{ATR: decimal.NewFromFloat(2)},
		CreatedAt:     time.Now().UTC(),
	}

	_, err := svc.ExecuteBuyTrade(context.Background(), signal, nil)
	if err == nil {
		t.Fatal("expected error when account is nil, got nil")
	}
}

func TestExecuteBuyTrade_ZeroRemainingTrades(t *testing.T) {
	remaining := 0
	account := &model.AccountTransfer{
		ID:              uuid.New(),
		AmountUSD:       decimal.NewFromFloat(1000),
		TargetTrades:    5,
		RemainingTrades: &remaining,
	}
	svc := newTestTradeService(&mockTradeRepo{}, &mockAccountTransferRepo{})
	signal := &model.Signal{
		ID:            uuid.New(),
		Symbol:        "AAPL",
		PriceAtSignal: decimal.NewFromFloat(150),
		Indicators:    model.SignalIndicators{ATR: decimal.NewFromFloat(2)},
		CreatedAt:     time.Now().UTC(),
	}

	trade, err := svc.ExecuteBuyTrade(context.Background(), signal, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trade != nil {
		t.Fatal("expected nil trade when no remaining trades")
	}
}

func TestExecuteSellTrade_NilAccountTransferID(t *testing.T) {
	svc := newTestTradeService(&mockTradeRepo{}, &mockAccountTransferRepo{})

	parentTrade := &model.Trade{
		ID:                uuid.New(),
		AccountTransferID: nil,
		Symbol:            "AAPL",
		Quantity:          decimal.NewFromFloat(10),
	}
	sellSignal := &model.Signal{
		ID:            uuid.New(),
		Symbol:        "AAPL",
		PriceAtSignal: decimal.NewFromFloat(160),
		CreatedAt:     time.Now().UTC(),
	}

	_, err := svc.ExecuteSellTrade(context.Background(), sellSignal, parentTrade)
	if err == nil {
		t.Fatal("expected error when parent trade has nil account_transfer_id, got nil")
	}
}

// ---------------------------------------------------------------------------
// Dynamic position sizing — table-driven, calls the real service
// ---------------------------------------------------------------------------

func TestDynamicPositionSizing(t *testing.T) {
	tests := []struct {
		name    string
		atr     decimal.Decimal
		price   decimal.Decimal
		budget  decimal.Decimal
		targets int
		wantQty decimal.Decimal
	}{
		{
			// riskAmount = 1000 × 0.01 = 10
			// atrStop    = 4 × 2.0    = 8
			// atrQty     = 10 / 8     = 1.25
			// maxQty     = 1000/5/150 ≈ 1.333  →  1.25 < 1.333, ATR qty wins
			name:    "ATR-based uncapped",
			atr:     decimal.NewFromFloat(4),
			price:   decimal.NewFromFloat(150),
			budget:  decimal.NewFromFloat(1000),
			targets: 5,
			wantQty: decimal.NewFromFloat(1.25),
		},
		{
			// riskAmount = 10, atrStop = 0.1×2 = 0.2, rawQty = 50
			// maxQty     = 1000/5/100 = 2  →  50 > 2, capped at 2
			name:    "ATR-based capped by slot max",
			atr:     decimal.NewFromFloat(0.1),
			price:   decimal.NewFromFloat(100),
			budget:  decimal.NewFromFloat(1000),
			targets: 5,
			wantQty: decimal.NewFromInt(2),
		},
		{
			// ATR = 0 → fallback equal-weight: 1000/5/100 = 2
			name:    "zero ATR falls back to fixed sizing",
			atr:     decimal.NewFromInt(0),
			price:   decimal.NewFromFloat(100),
			budget:  decimal.NewFromFloat(1000),
			targets: 5,
			wantQty: decimal.NewFromInt(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining := tt.targets
			account := &model.AccountTransfer{
				ID:              uuid.New(),
				AmountUSD:       tt.budget,
				TargetTrades:    tt.targets,
				RemainingTrades: &remaining,
			}
			signal := &model.Signal{
				ID:            uuid.New(),
				Symbol:        "AAPL",
				PriceAtSignal: tt.price,
				Indicators:    model.SignalIndicators{ATR: tt.atr},
				CreatedAt:     time.Now().UTC(),
			}

			placer := &mockOrderPlacer{}
			svc := newTestTradeServiceWith(&mockTradeRepo{}, &mockAccountTransferRepo{}, placer)

			trade, err := svc.ExecuteBuyTrade(context.Background(), signal, account)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if trade == nil {
				t.Fatal("expected trade, got nil")
			}
			if !trade.Quantity.Equal(tt.wantQty) {
				t.Errorf("trade.Quantity = %s, want %s", trade.Quantity, tt.wantQty)
			}

			// Verify the broker received the same quantity.
			if len(placer.placedOrders) != 1 {
				t.Fatalf("expected 1 placed order, got %d", len(placer.placedOrders))
			}
			if placer.placedOrders[0].Qty == nil {
				t.Fatal("placed order has nil Qty")
			}
			if !(*placer.placedOrders[0].Qty).Equal(tt.wantQty) {
				t.Errorf("broker order Qty = %s, want %s", *placer.placedOrders[0].Qty, tt.wantQty)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ApplyTradeUpdates
// ---------------------------------------------------------------------------

func TestApplyTradeUpdates_TerminalStatusIgnored(t *testing.T) {
	tradeID := uuid.New()
	alpacaOrderID := "order-123"
	existing := &model.Trade{
		ID:            tradeID,
		AlpacaOrderID: &alpacaOrderID,
		Status:        model.StatusFilled,
	}

	tradeRepo := &mockTradeRepo{
		byAlpacaOrder: map[string]*model.Trade{alpacaOrderID: existing},
	}
	svc := newTestTradeService(tradeRepo, &mockAccountTransferRepo{})

	update := alpaca.TradeUpdate{
		Event: "fill",
		Order: alpaca.Order{ID: alpacaOrderID},
	}

	err := svc.ApplyTradeUpdates(context.Background(), update, model.StatusFilled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tradeRepo.updated) != 0 {
		t.Fatalf("expected 0 updates for terminal trade, got %d", len(tradeRepo.updated))
	}
}
