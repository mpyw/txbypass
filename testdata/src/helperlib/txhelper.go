// Package helperlib holds helpers that end a transaction, for the fixtures of
// another package.
package helperlib

import (
	"context"
	"database/sql"
	"errors"
)

// Finish commits tx, or rolls it back when err is set.
func Finish(tx *sql.Tx, err error) error {
	if err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// Close ends tx through Finish.
func Close(tx *sql.Tx) error {
	return Finish(tx, nil)
}

// Later ends tx when the function returns.
func Later(tx *sql.Tx) {
	defer tx.Rollback()
}

// Keep does not end tx.
func Keep(tx *sql.Tx) *sql.Tx {
	return tx
}

// WithTx is a function, not a method, named like sqlc's.
func WithTx(tx *sql.Tx) *sql.Tx { return tx }

// RunInTx runs fn in a transaction begun on db.
func RunInTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// Begin begins a transaction on db.
func Begin(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	return db.BeginTx(ctx, nil)
}
