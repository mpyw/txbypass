// Package gopgtx covers outer-handle with go-pg, whose methods come from an
// embedded field.
package gopgtx

import (
	"context"

	"github.com/go-pg/pg/v10"
)

func Callback(ctx context.Context, db *pg.DB) error {
	return db.RunInTransaction(ctx, func(tx *pg.Tx) error {
		_, err := db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with RunInTransaction; use tx instead \[outer-handle\]$`
		return err
	})
}

func Savepoint(ctx context.Context, tx *pg.Tx) error {
	return tx.RunInTransaction(ctx, func(inner *pg.Tx) error {
		_, err := tx.Exec("INSERT INTO ledger VALUES (1)") // want `^tx bypasses the transaction opened with RunInTransaction; use inner instead \[outer-handle\]$`
		return err
	})
}

func Region(ctx context.Context, db *pg.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	if err := tx.Commit(); err != nil {
		return err
	}
	tx2, _ := db.BeginContext(ctx)
	db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with BeginContext; use tx2 instead \[outer-handle\]$`
	if err := tx2.CommitContext(ctx); err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO ledger VALUES (2)")
	return err
}
