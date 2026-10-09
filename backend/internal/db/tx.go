package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InTx runs fn inside a transaction, committing when it returns nil and
// rolling back otherwise.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *Queries) error) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return fn(New(tx))
	})
}

// Savepoint runs fn in a nested transaction: if fn fails, only its own
// changes are undone and the outer transaction carries on. q must itself be
// running in a transaction.
func (q *Queries) Savepoint(ctx context.Context, fn func(q *Queries) error) error {
	return pgx.BeginFunc(ctx, q.db.(pgx.Tx), func(tx pgx.Tx) error {
		return fn(New(tx))
	})
}

// Tx runs fn in a transaction of q's own: a new one when q runs on the
// pool, a nested one when q is already inside a transaction.
func (q *Queries) Tx(ctx context.Context, fn func(q *Queries) error) error {
	return pgx.BeginFunc(ctx, q.db.(interface {
		Begin(context.Context) (pgx.Tx, error)
	}), func(tx pgx.Tx) error {
		return fn(New(tx))
	})
}
