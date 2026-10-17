// Package goqutx covers outer-handle with goqu.
package goqutx

import (
	"context"

	"github.com/doug-martin/goqu/v9"
)

func Callback(db *goqu.Database) error {
	return db.WithTx(func(tx *goqu.TxDatabase) error {
		db.Insert("ledger") // want `^db bypasses the transaction opened with WithTx; use tx instead \[outer-handle\]$`
		return nil
	})
}

func Region(ctx context.Context, db *goqu.Database) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db.Insert("ledger") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	if err := tx.Commit(); err != nil {
		return err
	}
	tx2, _ := db.BeginTx(ctx, nil)
	db.Insert("ledger") // want `^db bypasses the transaction opened with BeginTx; use tx2 instead \[outer-handle\]$`
	return tx2.Rollback()
}
