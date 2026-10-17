package pgxtx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func PoolBegin(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	pool.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^pool bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit(ctx)
}

func PoolBeginTx(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	pool.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^pool bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
	return tx.Commit(ctx)
}

func ConnBegin(ctx context.Context, conn *pgx.Conn) error {
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	// Not reported: the connection is the one the transaction runs on.
	conn.Exec(ctx, "INSERT INTO ledger VALUES (1)")
	return tx.Commit(ctx)
}

func Acquired(ctx context.Context, pool *pgxpool.Pool) error {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	tx, err := c.Begin(ctx)
	if err != nil {
		return err
	}
	// Not reported: the connection is the one the transaction runs on.
	c.Exec(ctx, "INSERT INTO ledger VALUES (1)")
	return tx.Commit(ctx)
}

// A savepoint begun on a transaction.
func Savepoint(ctx context.Context, tx pgx.Tx) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	tx.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^tx bypasses the transaction opened with Begin; use sp instead \[outer-handle\]$`
	return sp.Commit(ctx)
}

// Queries is what sqlc generates for pgx.
type Queries struct{ db pgx.Tx }

func (q *Queries) WithTx(tx pgx.Tx) *Queries { return &Queries{db: tx} }

func (q *Queries) CreateUser(ctx context.Context, name string) error { return nil }

// sqlc's WithTx in a callback.
func Sqlc(ctx context.Context, pool *pgxpool.Pool, q *Queries) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qtx := q.WithTx(tx)
		if err := qtx.CreateUser(ctx, "a"); err != nil {
			return err
		}
		return q.CreateUser(ctx, "b") // want `^q bypasses the transaction opened with pgx\.BeginFunc; use qtx instead \[outer-handle\]$`
	})
}

// A callback with no name for its transaction rebinds nothing.
func SqlcUnnamed(ctx context.Context, pool *pgxpool.Pool, q *Queries) error {
	return pgx.BeginFunc(ctx, pool, func(pgx.Tx) error {
		return q.CreateUser(ctx, "b")
	})
}
