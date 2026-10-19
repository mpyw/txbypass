package pgxtx

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Beginner begins a transaction, or a savepoint when it is a transaction.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// inPgx runs fn in pgx's BeginFunc on db.
func inPgx(ctx context.Context, db Beginner, fn func(pgx.Tx) error) error { // want inPgx:`^invokes\[p1, p2\(0\)\]$`
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		return fn(tx)
	})
}

// Not reported: given a transaction, the helper opens a savepoint on its
// connection.
func HelperSavepoint(ctx context.Context, tx pgx.Tx) error {
	return inPgx(ctx, tx, func(sp pgx.Tx) error {
		_, err := tx.Exec(ctx, "select 1")
		return err
	})
}
