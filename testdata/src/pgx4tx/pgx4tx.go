// Package pgx4tx covers outer-handle with pgx v4, whose callback openers are
// methods.
package pgx4tx

import (
	"context"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

func ConnFunc(ctx context.Context, conn *pgx.Conn) error {
	return conn.BeginFunc(ctx, func(tx pgx.Tx) error {
		_, err := conn.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^conn bypasses the transaction opened with BeginFunc; use tx instead \[outer-handle\]$`
		return err
	})
}

func ConnTxFunc(ctx context.Context, conn *pgx.Conn) error {
	return conn.BeginTxFunc(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := conn.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^conn bypasses the transaction opened with BeginTxFunc; use tx instead \[outer-handle\]$`
		return err
	})
}

func PoolFunc(ctx context.Context, pool *pgxpool.Pool) error {
	return pool.BeginFunc(ctx, func(tx pgx.Tx) error {
		_, err := pool.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^pool bypasses the transaction opened with BeginFunc; use tx instead \[outer-handle\]$`
		return err
	})
}

func PoolTxFunc(ctx context.Context, pool *pgxpool.Pool) error {
	return pool.BeginTxFunc(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := pool.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^pool bypasses the transaction opened with BeginTxFunc; use tx instead \[outer-handle\]$`
		return err
	})
}

func SavepointFunc(ctx context.Context, tx pgx.Tx) error {
	return tx.BeginFunc(ctx, func(sp pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^tx bypasses the transaction opened with BeginFunc; use sp instead \[outer-handle\]$`
		return err
	})
}

func Region(ctx context.Context, pool *pgxpool.Pool, conn *pgx.Conn) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	pool.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^pool bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	tx2, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	conn.Exec(ctx, "INSERT INTO ledger VALUES (1)") // want `^conn bypasses the transaction opened with Begin; use tx2 instead \[outer-handle\]$`
	return tx2.Rollback(ctx)
}
