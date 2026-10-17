// Package pgxtx covers outer-handle with pgx's BeginFunc and BeginTxFunc.
package pgxtx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ===== Reported =====

// The handle is the second argument.
func Begin(ctx context.Context, pool *pgxpool.Pool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE accounts SET balance = balance - 1"); err != nil {
			return err
		}
		_, err := pool.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^pool bypasses the transaction opened with pgx\.BeginFunc; use tx instead \[outer-handle\]$`
		return err
	})
}

func BeginTx(ctx context.Context, conn *pgx.Conn) error {
	return pgx.BeginTxFunc(ctx, conn, pgx.TxOptions{IsoLevel: "serializable"}, func(pgx.Tx) error {
		// Not reported: the connection is the one the transaction runs on.
		_, err := conn.Exec(ctx, "INSERT INTO ledger VALUES (1)")
		return err
	})
}

type Repo struct {
	pool *pgxpool.Pool
}

func (r *Repo) Transfer(ctx context.Context) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		_, err := r.pool.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^r\.pool bypasses the transaction opened with pgx\.BeginFunc; use tx instead \[outer-handle\]$`
		return err
	})
}

// ===== Not reported =====

func Clean(ctx context.Context, pool *pgxpool.Pool) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO ledger VALUES (1)")
		return err
	})
}

// A nil handle is no handle.
func Nil(ctx context.Context) error {
	return pgx.BeginFunc(ctx, nil, func(tx pgx.Tx) error {
		return nil
	})
}

// A handle given by a call is not named from a variable.
func Fresh(ctx context.Context) error {
	return pgx.BeginFunc(ctx, connect(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO ledger VALUES (1)")
		return err
	})
}

func connect() *pgxpool.Pool { return &pgxpool.Pool{} }

// A callback held in a variable has no body here to read.
func Value(ctx context.Context, pool *pgxpool.Pool) error {
	fn := func(tx pgx.Tx) error {
		_, err := pool.Exec(ctx, "INSERT INTO ledger VALUES (1)")
		return err
	}
	return pgx.BeginFunc(ctx, pool, fn)
}

// A method of an interface with no name opens nothing.
func Anonymous(ctx context.Context, db interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}) error {
	_, err := db.Begin(ctx)
	return err
}
