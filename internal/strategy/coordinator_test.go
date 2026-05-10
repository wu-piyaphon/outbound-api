package strategy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	alpacaSDK "github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

// mockRegimeReader satisfies indicator.RegimeReader for tests.
type mockRegimeReader struct{ riskOn bool }

func (m *mockRegimeReader) IsRiskOn() bool { return m.riskOn }

// noopTransactor runs the callback without an outer DB transaction (tests).
type noopTransactor struct{}

func (noopTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func testAdaptiveParams() AdaptiveExitParams {
	return AdaptiveExitParams{
		BreakEvenATRTrigger: decimal.NewFromFloat(1.0),
		TrailATRTrigger:     decimal.NewFromFloat(1.5),
		TrailATRDistance:    decimal.NewFromFloat(2.0),
	}
}
// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockSignalService struct {
	evaluateResult  *model.Signal
	evaluateErr     error
	previewResult   *model.Signal
	previewErr      error
	evaluateCalls   int
	previewCalls    int
}

func (m *mockSignalService) GetAllSignals(_ context.Context) ([]model.Signal, error) {
	return nil, nil
}

func (m *mockSignalService) CreateSellSignal(_ context.Context, _ string, _ decimal.Decimal, _ string) (*model.Signal, error) {
	return nil, nil
}

func (m *mockSignalService) EvaluateBuySignal(_ context.Context, _ string, _ decimal.Decimal) (*model.Signal, error) {
	m.evaluateCalls++
	return m.evaluateResult, m.evaluateErr
}

func (m *mockSignalService) PreviewBuySignal(_ context.Context, _ string, _ decimal.Decimal) (*model.Signal, error) {
	m.previewCalls++
	return m.previewResult, m.previewErr
}

type mockTradeService struct {
	exitErr     error
	buyResult   *model.Trade
	buyErr      error
	exitCalls   int
	buyCalls    int
}

func (m *mockTradeService) ExecuteBuyTrade(_ context.Context, _ *model.Signal, _ *model.AccountTransfer) (*model.Trade, error) {
	m.buyCalls++
	return m.buyResult, m.buyErr
}

func (m *mockTradeService) ExecuteSellTrade(_ context.Context, _ *model.Signal, _ *model.Trade) (*model.Trade, error) {
	return nil, nil
}

func (m *mockTradeService) EvaluateAndExecuteExits(_ context.Context, _ string, _ decimal.Decimal) error {
	m.exitCalls++
	return m.exitErr
}

func (m *mockTradeService) ApplyTradeUpdates(_ context.Context, _ alpacaSDK.TradeUpdate, _ model.Status) error {
	return nil
}

type mockAccountTransferService struct {
	budget    *model.AccountTransfer
	budgetErr error
}

func (m *mockAccountTransferService) CreateAccountTransfer(_ context.Context, _ *model.AccountTransfer) error {
	return nil
}

func (m *mockAccountTransferService) GetAvailableBudget(_ context.Context) (*model.AccountTransfer, error) {
	return m.budget, m.budgetErr
}

func (m *mockAccountTransferService) DecrementRemainingTrades(_ context.Context, _ uuid.UUID) error {
	return nil
}

type mockTradeRepo struct {
	openTrades []*model.Trade
	openErr    error
}

func (m *mockTradeRepo) Create(_ context.Context, _ model.Trade) error        { return nil }
func (m *mockTradeRepo) Update(_ context.Context, _ model.Trade) error        { return nil }
func (m *mockTradeRepo) Delete(_ context.Context, _ uuid.UUID) error          { return nil }
func (m *mockTradeRepo) HasOpenPosition(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockTradeRepo) GetByAlpacaOrderID(_ context.Context, _ string) (*model.Trade, error) {
	return nil, nil
}
func (m *mockTradeRepo) GetOpenBuyTradesBySymbol(_ context.Context, _ string) ([]*model.Trade, error) {
	return m.openTrades, m.openErr
}

type mockShadowRepo struct {
	signalLogs  []*model.Signal
	exitLogs    []*model.ShadowExitDecision
	signalErr   error
	exitErr     error
}

func (m *mockShadowRepo) LogShadowSignal(_ context.Context, sig *model.Signal) error {
	if m.signalErr != nil {
		return m.signalErr
	}
	m.signalLogs = append(m.signalLogs, sig)
	return nil
}

func (m *mockShadowRepo) LogShadowExitDecision(_ context.Context, dec *model.ShadowExitDecision) error {
	if m.exitErr != nil {
		return m.exitErr
	}
	m.exitLogs = append(m.exitLogs, dec)
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var testEvent = BarEvent{
	Symbol:  "AAPL",
	Price:   decimal.NewFromInt(150),
	BarTime: time.Now().UTC(),
}

func newBudget() *model.AccountTransfer {
	remaining := 3
	return &model.AccountTransfer{
		ID:              uuid.New(),
		AmountUSD:       decimal.NewFromInt(10000),
		TargetTrades:    5,
		RemainingTrades: &remaining,
	}
}

func newSignal() *model.Signal {
	return &model.Signal{
		ID:            uuid.New(),
		Symbol:        "AAPL",
		Side:          model.SideBuy,
		PriceAtSignal: decimal.NewFromInt(150),
		Mode:          model.SignalModeLive,
	}
}

// ---------------------------------------------------------------------------
// V1Coordinator tests
// ---------------------------------------------------------------------------

func TestV1_EvaluateBar_NoSignal(t *testing.T) {
	sigSvc := &mockSignalService{evaluateResult: nil}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)

	if err := v1.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sigSvc.evaluateCalls != 1 {
		t.Errorf("expected EvaluateBuySignal called once, got %d", sigSvc.evaluateCalls)
	}
	if tradeSvc.exitCalls != 1 {
		t.Errorf("expected EvaluateAndExecuteExits called once, got %d", tradeSvc.exitCalls)
	}
	if tradeSvc.buyCalls != 0 {
		t.Errorf("expected ExecuteBuyTrade not called, got %d", tradeSvc.buyCalls)
	}
}

func TestV1_EvaluateBar_SignalFires_ExecutesBuy(t *testing.T) {
	sig := newSignal()
	sigSvc := &mockSignalService{evaluateResult: sig}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{budget: newBudget()}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)

	if err := v1.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tradeSvc.buyCalls != 1 {
		t.Errorf("expected ExecuteBuyTrade called once, got %d", tradeSvc.buyCalls)
	}
}

func TestV1_EvaluateBar_NoBudget_SkipsBuy(t *testing.T) {
	sig := newSignal()
	sigSvc := &mockSignalService{evaluateResult: sig}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{budget: nil}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)

	if err := v1.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tradeSvc.buyCalls != 0 {
		t.Errorf("expected ExecuteBuyTrade not called when no budget, got %d", tradeSvc.buyCalls)
	}
}

func TestV1_EvaluateBar_ExitError_ReturnsError(t *testing.T) {
	sigSvc := &mockSignalService{}
	tradeSvc := &mockTradeService{exitErr: errors.New("db error")}
	atSvc := &mockAccountTransferService{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)

	err := v1.EvaluateBar(context.Background(), testEvent)
	if err == nil {
		t.Fatal("expected error from exit evaluation, got nil")
	}
	// Buy signal evaluation must NOT have run after exit failure.
	if sigSvc.evaluateCalls != 0 {
		t.Errorf("EvaluateBuySignal should not run after exit error, got %d calls", sigSvc.evaluateCalls)
	}
}

// ---------------------------------------------------------------------------
// V2Coordinator tests
// ---------------------------------------------------------------------------

func TestV2_EvaluateBar_V1PathUnchanged(t *testing.T) {
	sig := newSignal()
	sigSvc := &mockSignalService{evaluateResult: sig}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{budget: newBudget()}
	tradeRepo := &mockTradeRepo{}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// v1 live path must have run.
	if tradeSvc.exitCalls != 1 {
		t.Errorf("v1 exit evaluation should run once, got %d", tradeSvc.exitCalls)
	}
	if tradeSvc.buyCalls != 1 {
		t.Errorf("v1 buy execution should run once, got %d", tradeSvc.buyCalls)
	}
}

func TestV2_EvaluateBar_ShadowBuySignalLogged(t *testing.T) {
	shadowSig := &model.Signal{
		ID:     uuid.New(),
		Symbol: "AAPL",
		Mode:   model.SignalModeShadow,
	}
	sigSvc := &mockSignalService{
		// v1 live path returns nil (no live buy); shadow preview returns a signal.
		evaluateResult: nil,
		previewResult:  shadowSig,
	}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sigSvc.previewCalls != 1 {
		t.Errorf("expected PreviewBuySignal called once, got %d", sigSvc.previewCalls)
	}
	if len(shadowRepo.signalLogs) != 1 {
		t.Errorf("expected 1 shadow signal logged, got %d", len(shadowRepo.signalLogs))
	}
	if shadowRepo.signalLogs[0].Mode != model.SignalModeShadow {
		t.Errorf("expected shadow mode on logged signal, got %q", shadowRepo.signalLogs[0].Mode)
	}
}

func TestV2_EvaluateBar_ShadowExitDecisionLogged(t *testing.T) {
	stopLoss := decimal.NewFromInt(140)
	openTrade := &model.Trade{
		ID:       uuid.New(),
		Symbol:   "AAPL",
		StopLoss: &stopLoss,
	}

	sigSvc := &mockSignalService{}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{openTrades: []*model.Trade{openTrade}}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	// Price 150 is above stop (140) — no exit triggered.
	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(shadowRepo.exitLogs) != 0 {
		t.Errorf("expected no shadow exit when price above stop, got %d", len(shadowRepo.exitLogs))
	}

	// Price below stop — exit should be logged.
	belowStopEvent := BarEvent{Symbol: "AAPL", Price: decimal.NewFromInt(130), BarTime: time.Now().UTC()}
	if err := v2.EvaluateBar(context.Background(), belowStopEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(shadowRepo.exitLogs) != 1 {
		t.Errorf("expected 1 shadow exit decision, got %d", len(shadowRepo.exitLogs))
	}
	if shadowRepo.exitLogs[0].Action != "exit_stop" {
		t.Errorf("expected action 'exit_stop', got %q", shadowRepo.exitLogs[0].Action)
	}
}

func TestV2_EvaluateBar_ShadowTakeProfitLogged(t *testing.T) {
	takeProfit := decimal.NewFromInt(145)
	openTrade := &model.Trade{
		ID:         uuid.New(),
		Symbol:     "AAPL",
		TakeProfit: &takeProfit,
	}

	sigSvc := &mockSignalService{}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{openTrades: []*model.Trade{openTrade}}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	// Price 150 >= take-profit 145 — exit_take_profit should be logged.
	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(shadowRepo.exitLogs) != 1 {
		t.Fatalf("expected 1 shadow exit decision, got %d", len(shadowRepo.exitLogs))
	}
	if shadowRepo.exitLogs[0].Action != "exit_take_profit" {
		t.Errorf("expected action 'exit_take_profit', got %q", shadowRepo.exitLogs[0].Action)
	}
}

func TestV2_EvaluateBar_ShadowFailures_DoNotBlockLivePath(t *testing.T) {
	sig := newSignal()
	sigSvc := &mockSignalService{
		evaluateResult: sig,
		previewErr:     errors.New("sentiment timeout"),
	}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{budget: newBudget()}
	tradeRepo := &mockTradeRepo{openErr: errors.New("db error")}
	shadowRepo := &mockShadowRepo{signalErr: errors.New("insert failed")}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	// All shadow paths fail — live path must still succeed.
	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("live path should not fail on shadow errors, got: %v", err)
	}

	if tradeSvc.exitCalls != 1 {
		t.Errorf("v1 exit calls expected 1, got %d", tradeSvc.exitCalls)
	}
	if tradeSvc.buyCalls != 1 {
		t.Errorf("v1 buy calls expected 1, got %d", tradeSvc.buyCalls)
	}
}

func TestV2_EvaluateBar_HoldNotLogged(t *testing.T) {
	// Trade has stop at 100, take-profit at 200. Price 150 is between — hold.
	stopLoss := decimal.NewFromInt(100)
	takeProfit := decimal.NewFromInt(200)
	openTrade := &model.Trade{
		ID:         uuid.New(),
		Symbol:     "AAPL",
		StopLoss:   &stopLoss,
		TakeProfit: &takeProfit,
	}

	sigSvc := &mockSignalService{}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{openTrades: []*model.Trade{openTrade}}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(shadowRepo.exitLogs) != 0 {
		t.Errorf("hold decisions must not be logged; got %d rows", len(shadowRepo.exitLogs))
	}
}

// ---------------------------------------------------------------------------
// Regime gate tests
// ---------------------------------------------------------------------------

func TestV2_RegimeOff_SkipsPreviewAndLogsRegimeOff(t *testing.T) {
	sigSvc := &mockSignalService{previewResult: newSignal()}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	// regime OFF
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: false}, noopTransactor{}, testAdaptiveParams())

	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PreviewBuySignal must NOT be called when regime is off.
	if sigSvc.previewCalls != 0 {
		t.Errorf("PreviewBuySignal must not run when regime is off, got %d calls", sigSvc.previewCalls)
	}

	// A regime_off shadow signal must be logged.
	if len(shadowRepo.signalLogs) != 1 {
		t.Fatalf("expected 1 regime_off shadow signal, got %d", len(shadowRepo.signalLogs))
	}
	logged := shadowRepo.signalLogs[0]
	if logged.Reasoning == nil || *logged.Reasoning != "regime_off" {
		t.Errorf("expected reasoning='regime_off', got %v", logged.Reasoning)
	}
	if logged.Mode != model.SignalModeShadow {
		t.Errorf("expected mode='shadow', got %q", logged.Mode)
	}
	if logged.Side != model.SideBuy {
		t.Errorf("expected side='buy', got %q", logged.Side)
	}
}

func TestV2_RegimeOff_LiveV1PathUnaffected(t *testing.T) {
	sig := newSignal()
	sigSvc := &mockSignalService{evaluateResult: sig}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{budget: newBudget()}
	tradeRepo := &mockTradeRepo{}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: false}, noopTransactor{}, testAdaptiveParams())

	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// v1 must still run its full path regardless of regime state.
	if tradeSvc.exitCalls != 1 {
		t.Errorf("v1 exit evaluation must run even when regime is off, got %d calls", tradeSvc.exitCalls)
	}
	if tradeSvc.buyCalls != 1 {
		t.Errorf("v1 buy execution must run even when regime is off, got %d calls", tradeSvc.buyCalls)
	}
}

func TestV2_RegimeOn_CallsPreviewNormally(t *testing.T) {
	shadowSig := &model.Signal{
		ID:     uuid.New(),
		Symbol: "AAPL",
		Mode:   model.SignalModeShadow,
	}
	sigSvc := &mockSignalService{
		evaluateResult: nil,
		previewResult:  shadowSig,
	}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	// regime ON
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sigSvc.previewCalls != 1 {
		t.Errorf("expected PreviewBuySignal called once when regime is on, got %d", sigSvc.previewCalls)
	}
	if len(shadowRepo.signalLogs) != 1 {
		t.Errorf("expected 1 shadow signal logged, got %d", len(shadowRepo.signalLogs))
	}
	// Must NOT be a regime_off marker.
	if shadowRepo.signalLogs[0].Reasoning != nil && *shadowRepo.signalLogs[0].Reasoning == "regime_off" {
		t.Error("shadow signal must not have reasoning='regime_off' when regime is on")
	}
}

func TestV2_RegimeOff_ShadowLogFailure_DoesNotBlockLivePath(t *testing.T) {
	sig := newSignal()
	sigSvc := &mockSignalService{evaluateResult: sig}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{budget: newBudget()}
	tradeRepo := &mockTradeRepo{}
	shadowRepo := &mockShadowRepo{signalErr: errors.New("db full")}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: false}, noopTransactor{}, testAdaptiveParams())

	// Shadow log fails, but live path must still complete without error.
	if err := v2.EvaluateBar(context.Background(), testEvent); err != nil {
		t.Fatalf("live path must not fail on shadow log error, got: %v", err)
	}
	if tradeSvc.buyCalls != 1 {
		t.Errorf("v1 buy must still run despite shadow log error, got %d calls", tradeSvc.buyCalls)
	}
}

func TestV2_AdaptiveStopMovedWhileV1Holds(t *testing.T) {
	entry := decimal.NewFromInt(100)
	stop := decimal.NewFromInt(95)
	atr := decimal.NewFromInt(2)
	tp := decimal.NewFromInt(200)
	openTrade := &model.Trade{
		ID:           uuid.New(),
		Symbol:       "AAPL",
		Side:         string(model.SideBuy),
		Quantity:     decimal.NewFromInt(10),
		AvgFillPrice: &entry,
		StopLoss:     &stop,
		TakeProfit:   &tp,
		EntryATR:     &atr,
	}

	sigSvc := &mockSignalService{}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{openTrades: []*model.Trade{openTrade}}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	ev := BarEvent{Symbol: "AAPL", Price: decimal.NewFromInt(103), BarTime: time.Now().UTC()}
	if err := v2.EvaluateBar(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(shadowRepo.exitLogs) != 1 {
		t.Fatalf("expected 1 shadow exit log, got %d", len(shadowRepo.exitLogs))
	}
	if shadowRepo.exitLogs[0].Action != "stop_moved" {
		t.Errorf("want stop_moved, got %q", shadowRepo.exitLogs[0].Action)
	}
}

func TestV2_AdaptiveExitWhileV1Holds(t *testing.T) {
	entry := decimal.NewFromInt(100)
	stop := decimal.NewFromInt(95)
	atr := decimal.NewFromInt(2)
	tp := decimal.NewFromInt(200)
	peak := decimal.NewFromInt(103)
	openTrade := &model.Trade{
		ID:           uuid.New(),
		Symbol:       "AAPL",
		Side:         string(model.SideBuy),
		Quantity:     decimal.NewFromInt(10),
		AvgFillPrice: &entry,
		StopLoss:     &stop,
		TakeProfit:   &tp,
		EntryATR:     &atr,
		PeakPrice:    &peak,
	}

	sigSvc := &mockSignalService{}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{openTrades: []*model.Trade{openTrade}}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	ev := BarEvent{Symbol: "AAPL", Price: decimal.NewFromInt(98), BarTime: time.Now().UTC()}
	if err := v2.EvaluateBar(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(shadowRepo.exitLogs) != 1 {
		t.Fatalf("expected 1 shadow exit log, got %d", len(shadowRepo.exitLogs))
	}
	if shadowRepo.exitLogs[0].Action != "exit_stop" {
		t.Errorf("want exit_stop, got %q", shadowRepo.exitLogs[0].Action)
	}
	if shadowRepo.exitLogs[0].Reasoning == nil || *shadowRepo.exitLogs[0].Reasoning != "adaptive_exit" {
		t.Errorf("want adaptive_exit reasoning, got %v", shadowRepo.exitLogs[0].Reasoning)
	}
}

func TestV2_AdaptiveNoShadowComparisonWhenV1StopHit(t *testing.T) {
	entry := decimal.NewFromInt(100)
	stop := decimal.NewFromInt(95)
	atr := decimal.NewFromInt(2)
	tp := decimal.NewFromInt(200)
	openTrade := &model.Trade{
		ID:           uuid.New(),
		Symbol:       "AAPL",
		Side:         string(model.SideBuy),
		Quantity:     decimal.NewFromInt(10),
		AvgFillPrice: &entry,
		StopLoss:     &stop,
		TakeProfit:   &tp,
		EntryATR:     &atr,
	}

	sigSvc := &mockSignalService{}
	tradeSvc := &mockTradeService{}
	atSvc := &mockAccountTransferService{}
	tradeRepo := &mockTradeRepo{openTrades: []*model.Trade{openTrade}}
	shadowRepo := &mockShadowRepo{}

	v1 := NewV1Coordinator(sigSvc, tradeSvc, atSvc)
	v2 := NewV2Coordinator(v1, sigSvc, tradeRepo, shadowRepo, &mockRegimeReader{riskOn: true}, noopTransactor{}, testAdaptiveParams())

	ev := BarEvent{Symbol: "AAPL", Price: decimal.NewFromInt(94), BarTime: time.Now().UTC()}
	if err := v2.EvaluateBar(context.Background(), ev); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(shadowRepo.exitLogs) != 0 {
		t.Fatalf("expected no adaptive comparison log when v1 stop already hit, got %d logs", len(shadowRepo.exitLogs))
	}
}
