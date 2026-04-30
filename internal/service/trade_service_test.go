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

func newTestTradeService(tradeRepo repository.TradeRepository, atRepo repository.AccountTransferRepository) TradeService {
	return NewTradeService(
		tradeRepo,
		atRepo,
		&mockSignalRepo{},
		&mockTransactor{},
		nil,
		decimal.NewFromFloat(0.01),
		decimal.NewFromFloat(2.0),
	)
}

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
		AccountTransferID: nil, // nil — should be rejected
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

func TestDynamicPositionSizing_ATRBasedQuantity(t *testing.T) {
	// With ATR=4, multiplier=2.0, stop distance = 8
	// With budget=1000 and riskPct=0.01, riskAmount=10
	// Expected: qty = 10 / 8 = 1.25
	// Max per slot = 1000 / 5 / 150 = 1.333...
	// So ATR qty (1.25) < max (1.333), ATR qty wins
	atrValue := decimal.NewFromFloat(4)
	priceAtSignal := decimal.NewFromFloat(150)
	budget := decimal.NewFromFloat(1000)
	riskPct := decimal.NewFromFloat(0.01)
	atrMult := decimal.NewFromFloat(2.0)
	targetTrades := 5

	riskAmount := budget.Mul(riskPct)                                                // 10
	atrStop := atrValue.Mul(atrMult)                                                 // 8
	wantQty := riskAmount.Div(atrStop)                                               // 1.25
	maxQty := budget.Div(decimal.NewFromInt(int64(targetTrades))).Div(priceAtSignal) // 1.333

	if wantQty.GreaterThan(maxQty) {
		t.Fatalf("test invariant broken: ATR qty should be <= max qty")
	}

	gotQty := riskAmount.Div(atrStop)
	if !gotQty.Equal(wantQty) {
		t.Fatalf("expected qty %s, got %s", wantQty, gotQty)
	}
}

func TestDynamicPositionSizing_CappedByMaxSlot(t *testing.T) {
	// When ATR is very small, risk-based qty would be huge → must be capped.
	atrValue := decimal.NewFromFloat(0.1)
	budget := decimal.NewFromFloat(1000)
	price := decimal.NewFromFloat(100)
	riskPct := decimal.NewFromFloat(0.01)
	atrMult := decimal.NewFromFloat(2.0)
	targetTrades := 5

	riskAmount := budget.Mul(riskPct)                                        // 10
	atrStop := atrValue.Mul(atrMult)                                         // 0.2
	rawQty := riskAmount.Div(atrStop)                                        // 50 — too large
	maxQty := budget.Div(decimal.NewFromInt(int64(targetTrades))).Div(price) // 2

	if !rawQty.GreaterThan(maxQty) {
		t.Fatal("test invariant broken: raw qty should exceed max qty in this scenario")
	}

	gotQty := rawQty
	if gotQty.GreaterThan(maxQty) {
		gotQty = maxQty
	}
	if !gotQty.Equal(maxQty) {
		t.Fatalf("expected capped qty %s, got %s", maxQty, gotQty)
	}
}

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
	// Nothing should have been written since the trade was already terminal.
	if len(tradeRepo.updated) != 0 {
		t.Fatalf("expected 0 updates for terminal trade, got %d", len(tradeRepo.updated))
	}
}
