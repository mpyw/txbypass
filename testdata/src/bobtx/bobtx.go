// Package bobtx covers outer-handle with bob.
package bobtx

import (
	"context"

	"github.com/stephenafamo/bob"
)

func Callback(ctx context.Context, db bob.DB) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bob.Transaction) error {
		_, err := db.ExecContext(ctx, "INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with RunInTx; use tx instead \[outer-handle\]$`
		return err
	})
}

func Region(ctx context.Context, db bob.DB, conn bob.Conn) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	db.ExecContext(ctx, "INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	ctx2, _ := conn.BeginTx(ctx, nil)
	conn.ExecContext(ctx, "INSERT INTO ledger VALUES (1)") // want `^conn bypasses the transaction opened with BeginTx; use ctx2 instead \[outer-handle\]$`
	return ctx2.Rollback(ctx)
}

// Begin is BeginTx with no options.
func Begin(ctx context.Context, db bob.DB, conn bob.Conn) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	db.ExecContext(ctx, "INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	tx2, _ := conn.Begin(ctx)
	conn.ExecContext(ctx, "INSERT INTO ledger VALUES (1)") // want `^conn bypasses the transaction opened with Begin; use tx2 instead \[outer-handle\]$`
	return tx2.Rollback(ctx)
}
