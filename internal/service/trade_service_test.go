package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
	mu            sync.Mutex
	created       []model.Trade
	updated       []model.Trade
	deleted       []uuid.UUID
	openTrades    []*model.Trade
	byAlpacaOrder map[string]*model.Trade
	hasOpenPos    bool
	createErr     error
	updateErr     error
	deleteErr     error
}

func (m *mockTradeRepo) Create(_ context.Context, t model.Trade) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updated = append(m.updated, t)
	return nil
}

func (m *mockTradeRepo) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deleted = append(m.deleted, id)
	return nil
}

func (m *mockTradeRepo) HasOpenPosition(_ context.Context, _ string) (bool, error) {
	return m.hasOpenPos, nil
}

type mockAccountTransferRepo struct {
	mu          sync.Mutex
	decremented int
	incremented int
	budget      *model.AccountTransfer
	errDecrement error
}

func (m *mockAccountTransferRepo) Create(_ context.Context, _ *model.AccountTransfer) error {
	return nil
}

func (m *mockAccountTransferRepo) GetAvailableBudget(_ context.Context) (*model.AccountTransfer, error) {
	return m.budget, nil
}

func (m *mockAccountTransferRepo) DecrementRemainingTrades(_ context.Context, _ uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errDecrement != nil {
		return m.errDecrement
	}
	m.decremented++
	return nil
}

func (m *mockAccountTransferRepo) IncrementRemainingTrades(_ context.Context, _ uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incremented++
	return nil
}

type mockSignalRepo struct{}

func (m *mockSignalRepo) GetAll(_ context.Context) ([]model.Signal, error)    { return nil, nil }
func (m *mockSignalRepo) Create(_ context.Context, _ *model.Signal) error     { return nil }
func (m *mockSignalRepo) Delete(_ context.Context, _ uuid.UUID) error         { return nil }
func (m *mockSignalRepo) MarkExecuted(_ context.Context, _ uuid.UUID) error   { return nil }

type mockTransactor struct{}

func (m *mockTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// mockOrderPlacer records every PlaceOrder call so tests can assert on them.
type mockOrderPlacer struct {
	mu           sync.Mutex
	placedOrders []alpaca.PlaceOrderRequest
	returnErr    error
}

func (m *mockOrderPlacer) PlaceOrder(req alpaca.PlaceOrderRequest) (*alpaca.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		decimal.NewFromFloat(0.01), // riskPerTradePct
		decimal.NewFromFloat(2.0),  // atrRiskMultiplier
		decimal.NewFromFloat(3.0),  // takeProfitMultiplier
	)
}

func makeSignal(symbol string, price, atr float64) *model.Signal {
	return &model.Signal{
		ID:            uuid.New(),
		Symbol:        symbol,
		PriceAtSignal: decimal.NewFromFloat(price),
		Indicators:    model.SignalIndicators{ATR: decimal.NewFromFloat(atr)},
		CreatedAt:     time.Now().UTC(),
	}
}

func makeAccount(amountUSD float64, targetTrades, remainingTrades int) *model.AccountTransfer {
	remaining := remainingTrades
	return &model.AccountTransfer{
		ID:              uuid.New(),
		AmountUSD:       decimal.NewFromFloat(amountUSD),
		TargetTrades:    targetTrades,
		RemainingTrades: &remaining,
	}
}

// ---------------------------------------------------------------------------
// Guard / validation tests
// ---------------------------------------------------------------------------

func TestExecuteBuyTrade_NilAccount(t *testing.T) {
	svc := newTestTradeService(&mockTradeRepo{}, &mockAccountTransferRepo{})
	_, err := svc.ExecuteBuyTrade(context.Background(), makeSignal("AAPL", 150, 2), nil)
	if err == nil {
		t.Fatal("expected error when account is nil, got nil")
	}
}

func TestExecuteBuyTrade_ZeroRemainingTrades(t *testing.T) {
	svc := newTestTradeService(&mockTradeRepo{}, &mockAccountTransferRepo{})
	trade, err := svc.ExecuteBuyTrade(context.Background(), makeSignal("AAPL", 150, 2), makeAccount(1000, 5, 0))
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
			signal := &model.Signal{
				ID:            uuid.New(),
				Symbol:        "AAPL",
				PriceAtSignal: tt.price,
				Indicators:    model.SignalIndicators{ATR: tt.atr},
				CreatedAt:     time.Now().UTC(),
			}

			placer := &mockOrderPlacer{}
			svc := newTestTradeServiceWith(&mockTradeRepo{}, &mockAccountTransferRepo{}, placer)

			trade, err := svc.ExecuteBuyTrade(context.Background(), signal, makeAccount(tt.budget.InexactFloat64(), tt.targets, tt.targets))
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
// Slot accounting correctness
// ---------------------------------------------------------------------------

// TestExecuteBuyTrade_SlotExhausted verifies that when DecrementRemainingTrades
// returns ErrNoRemainingSlots the service returns nil, nil (not an error) and
// no broker order is placed.
func TestExecuteBuyTrade_SlotExhausted(t *testing.T) {
	atRepo := &mockAccountTransferRepo{
		errDecrement: fmt.Errorf("DecrementRemainingTrades: %w", repository.ErrNoRemainingSlots),
	}
	placer := &mockOrderPlacer{}
	svc := newTestTradeServiceWith(&mockTradeRepo{}, atRepo, placer)

	trade, err := svc.ExecuteBuyTrade(context.Background(), makeSignal("AAPL", 100, 2), makeAccount(1000, 5, 1))
	if err != nil {
		t.Fatalf("expected nil error when slot exhausted, got: %v", err)
	}
	if trade != nil {
		t.Fatal("expected nil trade when slot exhausted")
	}
	if len(placer.placedOrders) != 0 {
		t.Fatalf("expected 0 broker orders when slot exhausted, got %d", len(placer.placedOrders))
	}
}

// slotCounter is shared state for serialisedSlotTransactor.
type slotCounter struct {
	mu          sync.Mutex
	slotsLeft   int
	decremented int
}

// serialisedSlotTransactor is a test-only Transactor that enforces a slot limit
// across concurrent calls, mimicking the DB WHERE remaining_trades > 0 guard.
type serialisedSlotTransactor struct {
	counter *slotCounter
}

func (s *serialisedSlotTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	s.counter.mu.Lock()
	defer s.counter.mu.Unlock()

	if s.counter.slotsLeft <= 0 {
		return fmt.Errorf("DecrementRemainingTrades: %w", repository.ErrNoRemainingSlots)
	}
	s.counter.slotsLeft--
	s.counter.decremented++
	return fn(ctx)
}

// TestExecuteBuyTrade_ConcurrentSlotRace runs multiple goroutines against a
// single remaining slot and verifies exactly one trade is persisted and one
// broker order is placed. This exercises the authoritative-decrement guarantee.
func TestExecuteBuyTrade_ConcurrentSlotRace(t *testing.T) {
	remaining := 1
	account := &model.AccountTransfer{
		ID:              uuid.New(),
		AmountUSD:       decimal.NewFromFloat(1000),
		TargetTrades:    5,
		RemainingTrades: &remaining,
	}

	ct := &slotCounter{slotsLeft: 1}
	atRepo := &mockAccountTransferRepo{}
	tradeRepo := &mockTradeRepo{}
	placer := &mockOrderPlacer{}

	svc := NewTradeService(
		tradeRepo,
		atRepo,
		&mockSignalRepo{},
		&serialisedSlotTransactor{counter: ct},
		placer,
		decimal.NewFromFloat(0.01),
		decimal.NewFromFloat(2.0),
		decimal.NewFromFloat(3.0),
	)

	const workers = 5
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.ExecuteBuyTrade(context.Background(), makeSignal("AAPL", 100, 2), account) //nolint:errcheck
		}()
	}
	wg.Wait()

	ct.mu.Lock()
	decremented := ct.decremented
	ct.mu.Unlock()

	if decremented != 1 {
		t.Errorf("expected exactly 1 slot consumed by concurrent workers, got %d", decremented)
	}
	if len(placer.placedOrders) != 1 {
		t.Errorf("expected exactly 1 broker order, got %d", len(placer.placedOrders))
	}
}

// TestExecuteBuyTrade_BrokerFailureRestoresSlot verifies that when PlaceOrder
// fails the pre-inserted trade is marked cancelled and the slot is restored.
func TestExecuteBuyTrade_BrokerFailureRestoresSlot(t *testing.T) {
	placer := &mockOrderPlacer{returnErr: errors.New("broker unavailable")}
	atRepo := &mockAccountTransferRepo{}
	tradeRepo := &mockTradeRepo{}
	svc := newTestTradeServiceWith(tradeRepo, atRepo, placer)

	trade, err := svc.ExecuteBuyTrade(context.Background(), makeSignal("AAPL", 100, 2), makeAccount(1000, 5, 1))
	if err == nil {
		t.Fatal("expected error when broker fails, got nil")
	}
	if trade != nil {
		t.Fatal("expected nil trade when broker fails")
	}

	// The slot must have been restored.
	if atRepo.incremented != 1 {
		t.Errorf("expected 1 slot restoration, got %d", atRepo.incremented)
	}

	// The pre-inserted trade must have been marked cancelled via Update.
	if len(tradeRepo.updated) == 0 {
		t.Fatal("expected trade to be updated to cancelled status after broker failure")
	}
	lastUpdate := tradeRepo.updated[len(tradeRepo.updated)-1]
	if lastUpdate.Status != model.StatusCancelled {
		t.Errorf("expected cancelled status, got %s", lastUpdate.Status)
	}
}

// TestExecuteBuyTrade_PreInsertBeforeBroker verifies the two-phase ordering:
// the trade record must be created in the DB before PlaceOrder is called.
func TestExecuteBuyTrade_PreInsertBeforeBroker(t *testing.T) {
	var dbWriteTime, brokerCallTime time.Time

	tradeRepo := &mockTradeRepo{}
	placer := &mockOrderPlacer{}

	// Wrap the real mocks with timing instrumentation.
	timedRepo := &timedTradeRepo{inner: tradeRepo, onCreate: func() { dbWriteTime = time.Now() }}
	timedPlacer := &timedOrderPlacer{inner: placer, onPlace: func() { brokerCallTime = time.Now() }}

	svc := newTestTradeServiceWith(timedRepo, &mockAccountTransferRepo{}, timedPlacer)
	_, err := svc.ExecuteBuyTrade(context.Background(), makeSignal("AAPL", 100, 2), makeAccount(1000, 5, 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !dbWriteTime.Before(brokerCallTime) {
		t.Error("trade record must be created in DB before the broker order is placed")
	}
}

// timedTradeRepo delegates to inner and fires a callback on Create.
type timedTradeRepo struct {
	inner    *mockTradeRepo
	onCreate func()
}

func (r *timedTradeRepo) Create(ctx context.Context, t model.Trade) error {
	err := r.inner.Create(ctx, t)
	if err == nil && r.onCreate != nil {
		r.onCreate()
	}
	return err
}
func (r *timedTradeRepo) GetOpenBuyTradesBySymbol(ctx context.Context, s string) ([]*model.Trade, error) {
	return r.inner.GetOpenBuyTradesBySymbol(ctx, s)
}
func (r *timedTradeRepo) GetByAlpacaOrderID(ctx context.Context, id string) (*model.Trade, error) {
	return r.inner.GetByAlpacaOrderID(ctx, id)
}
func (r *timedTradeRepo) Update(ctx context.Context, t model.Trade) error {
	return r.inner.Update(ctx, t)
}
func (r *timedTradeRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.inner.Delete(ctx, id)
}
func (r *timedTradeRepo) HasOpenPosition(ctx context.Context, s string) (bool, error) {
	return r.inner.HasOpenPosition(ctx, s)
}

type timedOrderPlacer struct {
	inner   *mockOrderPlacer
	onPlace func()
}

func (p *timedOrderPlacer) PlaceOrder(req alpaca.PlaceOrderRequest) (*alpaca.Order, error) {
	if p.onPlace != nil {
		p.onPlace()
	}
	return p.inner.PlaceOrder(req)
}

// ---------------------------------------------------------------------------
// ApplyTradeUpdates
// ---------------------------------------------------------------------------

func TestApplyTradeUpdates_TerminalStatusIgnored(t *testing.T) {
	alpacaOrderID := "order-123"
	existing := &model.Trade{
		ID:            uuid.New(),
		AlpacaOrderID: &alpacaOrderID,
		Status:        model.StatusFilled,
	}

	tradeRepo := &mockTradeRepo{
		byAlpacaOrder: map[string]*model.Trade{alpacaOrderID: existing},
	}
	svc := newTestTradeService(tradeRepo, &mockAccountTransferRepo{})

	err := svc.ApplyTradeUpdates(context.Background(), alpaca.TradeUpdate{
		Event: "fill",
		Order: alpaca.Order{ID: alpacaOrderID},
	}, model.StatusFilled)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tradeRepo.updated) != 0 {
		t.Fatalf("expected 0 updates for terminal trade, got %d", len(tradeRepo.updated))
	}
}

// TestApplyTradeUpdates_UnknownOrderID verifies that an update for an order the
// system does not know about is silently skipped (not returned as an error).
func TestApplyTradeUpdates_UnknownOrderID(t *testing.T) {
	svc := newTestTradeService(&mockTradeRepo{byAlpacaOrder: map[string]*model.Trade{}}, &mockAccountTransferRepo{})

	err := svc.ApplyTradeUpdates(context.Background(), alpaca.TradeUpdate{
		Event: "fill",
		Order: alpaca.Order{ID: "unknown-order"},
	}, model.StatusFilled)
	if err != nil {
		t.Fatalf("expected nil for unknown order ID, got: %v", err)
	}
}

// TestApplyTradeUpdates_BuyRejectedRestoresSlot verifies that a rejected buy
// order increments the account slot counter.
func TestApplyTradeUpdates_BuyRejectedRestoresSlot(t *testing.T) {
	alpacaOrderID := "order-456"
	transferID := uuid.New()
	existing := &model.Trade{
		ID:                uuid.New(),
		AlpacaOrderID:     &alpacaOrderID,
		Side:              string(model.SideBuy),
		AccountTransferID: &transferID,
		Status:            model.StatusPending,
	}

	tradeRepo := &mockTradeRepo{
		byAlpacaOrder: map[string]*model.Trade{alpacaOrderID: existing},
	}
	atRepo := &mockAccountTransferRepo{}
	svc := newTestTradeService(tradeRepo, atRepo)

	err := svc.ApplyTradeUpdates(context.Background(), alpaca.TradeUpdate{
		Event: "rejected",
		Order: alpaca.Order{ID: alpacaOrderID},
	}, model.StatusRejected)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atRepo.incremented != 1 {
		t.Errorf("expected slot restored after buy rejection, got %d increments", atRepo.incremented)
	}
}
