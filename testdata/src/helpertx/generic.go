package helpertx

import (
	"context"
	"database/sql"
	"errors"
)

// Generic is a store of a generic type.
type Generic[T any] struct {
	db   *sql.DB
	last T
}

// withTx is a method of a generic type.
func (g *Generic[T]) withTx(ctx context.Context, fn func(*sql.Tx) error) error { // want withTx:`^invokes\[p0\.db, p2\(0\)\]$`
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// begin is a begin of a generic type.
func (g *Generic[T]) begin(ctx context.Context) (*sql.Tx, error) { // want begin:`^begins\[p0\.db\]$`
	return g.db.BeginTx(ctx, nil)
}

// Query is a generic function that returns what fn gives.
func Query[T any](ctx context.Context, db *sql.DB, fn func(*sql.Tx) (T, error)) (T, error) { // want Query:`^invokes\[p1, p2\(0\)\]$`
	var zero T
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	v, err := fn(tx)
	if err != nil {
		return zero, err
	}
	return v, tx.Commit()
}

func UseGeneric(ctx context.Context, g *Generic[int], db *sql.DB) error {
	if err := g.withTx(ctx, func(tx *sql.Tx) error {
		_, err := g.db.ExecContext(ctx, "q") // want `^g\.db bypasses the transaction opened with withTx; use tx instead \[outer-handle\]$`
		return err
	}); err != nil {
		return err
	}
	tx, err := g.begin(ctx)
	if err != nil {
		return err
	}
	g.db.ExecContext(ctx, "q") // want `^g\.db bypasses the transaction opened with begin; use tx instead \[outer-handle\]$`
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = Query(ctx, db, func(tx *sql.Tx) (int, error) {
		_, err := db.ExecContext(ctx, "q") // want `^db bypasses the transaction opened with helpertx\.Query; use tx instead \[outer-handle\]$`
		return 0, err
	})
	return err
}
