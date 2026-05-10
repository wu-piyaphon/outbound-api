// Package repository contains the persistence layer. All repositories accept a
// DBTX so the same query code runs against either *pgxpool.Pool or pgx.Tx,
// with the active transaction resolved via GetDB.
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

// DBTX is the minimal pgx surface (*pgxpool.Pool and pgx.Tx both satisfy it)
// used by repositories so the same query code runs inside or outside a
// transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txKey struct{}

// Transactor runs fn inside a database transaction. Implementations inject the
// active pgx.Tx into the context so repositories called by fn execute against
// it via GetDB.
type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// GetDB returns the pgx.Tx stored on ctx if present, otherwise fallback.
// Repositories should always call this rather than the pool directly so the
// same method works inside and outside WithinTransaction.
func GetDB(ctx context.Context, fallback DBTX) DBTX {
	tx, ok := ctx.Value(txKey{}).(DBTX)
	if ok {
		return tx
	}
	return fallback
}

// InjectTx returns a context carrying tx. Used by Transactor implementations;
// repositories retrieve the value via GetDB.
func InjectTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}
