// Package buntx covers outer-handle with bun's RunInTx.
package buntx

import (
	"context"
	"database/sql"

	"github.com/uptrace/bun"
)

type User struct{ Name string }

// ===== Reported =====

func OnDB(ctx context.Context, db *bun.DB, u *User) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(u).Exec(ctx); err != nil {
			return err
		}
		_, err := db.NewInsert().Model(u).Exec(ctx) // want `^db bypasses the transaction opened with RunInTx; use tx instead \[outer-handle\]$`
		return err
	})
}

func OnConn(ctx context.Context, conn bun.Conn, u *User) error {
	return conn.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// Not reported: the connection is the one the transaction runs on.
		_, err := conn.NewInsert().Model(u).Exec(ctx)
		return err
	})
}

// A savepoint is opened on a transaction.
func OnTx(ctx context.Context, tx bun.Tx, u *User) error {
	return tx.RunInTx(ctx, nil, func(ctx context.Context, sp bun.Tx) error {
		// Not reported: a savepoint runs on the transaction's connection.
		_, err := tx.NewInsert().Model(u).Exec(ctx)
		return err
	})
}

// The interface that DB, Conn and Tx share.
func OnIDB(ctx context.Context, idb bun.IDB, u *User) error {
	return idb.RunInTx(ctx, nil, func(ctx context.Context, _ bun.Tx) error {
		_, err := idb.NewInsert().Model(u).Exec(ctx) // want `^idb bypasses the transaction opened with RunInTx; use the callback's transaction instead \[outer-handle\]$`
		return err
	})
}

// The *sql.DB that bun.DB embeds is the same pool, and a field of the
// handle.
func Embedded(ctx context.Context, db *bun.DB) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := db.ExecContext(ctx, "DELETE FROM users") // want `^db bypasses the transaction opened with RunInTx; use tx instead \[outer-handle\]$`
		return err
	})
}

// BeginTx opens a transaction without a callback. It is open until it is
// committed or rolled back.
func Begin(ctx context.Context, db *bun.DB, u *User) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = db.NewInsert().Model(u).Exec(ctx) // want `^db bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Begin of DB, BeginTx of the IDB interface, and Begin of Tx, a savepoint.
func Begins(ctx context.Context, db *bun.DB, idb bun.IDB, u *User) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db.NewInsert().Model(u).Exec(ctx) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	if err := tx.Commit(); err != nil {
		return err
	}
	tx2, err := idb.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	idb.NewInsert().Model(u).Exec(ctx) // want `^idb bypasses the transaction opened with BeginTx; use tx2 instead \[outer-handle\]$`
	sp, err := tx2.Begin()
	if err != nil {
		return err
	}
	// Not reported: a savepoint runs on the transaction's connection.
	tx2.NewInsert().Model(u).Exec(ctx)
	if err := sp.Commit(); err != nil {
		return err
	}
	return tx2.Commit()
}

// ===== Not reported =====

func Clean(ctx context.Context, db *bun.DB, u *User) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewInsert().Model(u).Exec(ctx)
		return err
	})
}
