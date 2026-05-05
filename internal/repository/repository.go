package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNoRemainingSlots is returned by DecrementRemainingTrades when the
// account transfer has no slots left. Callers should treat this as a normal
// "nothing to do" condition, not an infrastructure failure.
var ErrNoRemainingSlots = errors.New("account transfer has no remaining trade slots")

// ErrPositionAlreadyOpen is returned when HasOpenPosition finds an active
// (pending, open, or filled) buy trade for the symbol inside the buy
// transaction. Callers should treat this as a normal "nothing to do" condition.
var ErrPositionAlreadyOpen = errors.New("an open position already exists for this symbol")

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

func GetDB(ctx context.Context, fallback DBTX) DBTX {
	tx, ok := ctx.Value(txKey{}).(DBTX)
	if ok {
		return tx
	}
	return fallback
}

func InjectTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}
