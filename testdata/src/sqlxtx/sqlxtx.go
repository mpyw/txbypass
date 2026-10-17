// Package sqlxtx covers outer-handle with sqlx's begins.
package sqlxtx

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type User struct{ Name string }

func Beginx(db *sqlx.DB) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	db.MustExec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Beginx; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

func MustBegin(db *sqlx.DB) error {
	tx := db.MustBegin()
	var u User
	db.Get(&u, "SELECT * FROM users") // want `^db bypasses the transaction opened with MustBegin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

func BeginTxx(ctx context.Context, db *sqlx.DB) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	db.MustExec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with BeginTxx; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

func MustBeginTx(ctx context.Context, db *sqlx.DB) error {
	tx := db.MustBeginTx(ctx, nil)
	db.MustExec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with MustBeginTx; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

func OnConn(ctx context.Context, conn *sqlx.Conn) error {
	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	conn.ExecContext(ctx, "INSERT INTO ledger VALUES (1)") // want `^conn bypasses the transaction opened with BeginTxx; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// Begin is database/sql's, promoted from the embedded *sql.DB. sqlx's own
// methods are on the same handle.
func Promoted(db *sqlx.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db.MustExec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	db.Exec("INSERT INTO ledger VALUES (2)")     // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}
