package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

type transactor struct {
	pool *pgxpool.Pool
}

func NewTransactor(pool *pgxpool.Pool) repository.Transactor {
	return &transactor{pool: pool}
}

func (t *transactor) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	txCtx := repository.InjectTx(ctx, tx)

	err = fn(txCtx)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
