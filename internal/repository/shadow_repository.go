package repository

import (
	"context"
	"fmt"

	"github.com/wu-piyaphon/outbound-api/internal/model"
)

// ShadowRepository persists observations from the shadow strategy path.
// Writes are best-effort: callers should log errors but not abort the live path.
type ShadowRepository interface {
	// LogShadowSignal persists a signal with mode='shadow'. The signal's Mode
	// field must be set to model.SignalModeShadow before calling.
	LogShadowSignal(ctx context.Context, signal *model.Signal) error
	// LogShadowExitDecision records a non-hold exit evaluation outcome.
	// Hold actions are inferred by absence to keep the table compact.
	LogShadowExitDecision(ctx context.Context, dec *model.ShadowExitDecision) error
}

type shadowRepository struct {
	pool DBTX
}

// NewShadowRepository constructs a ShadowRepository backed by pool.
func NewShadowRepository(pool DBTX) ShadowRepository {
	return &shadowRepository{pool: pool}
}

func (r *shadowRepository) LogShadowSignal(ctx context.Context, signal *model.Signal) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx,
		"INSERT INTO signals (id, symbol, side, price_at_signal, indicators, is_executed, mode, reasoning) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		signal.ID, signal.Symbol, signal.Side, signal.PriceAtSignal, signal.Indicators, signal.IsExecuted, signal.Mode, signal.Reasoning)
	if err != nil {
		return fmt.Errorf("LogShadowSignal: %w", err)
	}
	return nil
}

func (r *shadowRepository) LogShadowExitDecision(ctx context.Context, dec *model.ShadowExitDecision) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx,
		`INSERT INTO shadow_exit_decisions (id, trade_id, bar_time, current_price, peak_price, current_stop, action, reasoning, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		dec.ID, dec.TradeID, dec.BarTime, dec.CurrentPrice, dec.PeakPrice, dec.CurrentStop, dec.Action, dec.Reasoning, dec.CreatedAt)
	if err != nil {
		return fmt.Errorf("LogShadowExitDecision: %w", err)
	}
	return nil
}
