// Package sqltx covers outer-handle with database/sql's Begin and BeginTx,
// whose transaction is open until it is committed or rolled back.
package sqltx

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
)

func use(...any) {}

// ===== Reported =====

// The handle is used between the begin and the commit.
func Transfer(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE accounts SET balance = balance - 1"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO ledger VALUES (1)"); err != nil { // want `^db bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
		return err
	}
	return tx.Commit()
}

// Begin, with no context.
func Plain(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// A connection begins one too.
func OnConn(ctx context.Context, conn *sql.Conn) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// Not reported: the connection is the one the transaction runs on.
	conn.ExecContext(ctx, "INSERT INTO ledger VALUES (1)")
	return tx.Commit()
}

// A later error check is not the begin's: the transaction is open in it,
// until it is rolled back.
func LaterCheck(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE accounts SET balance = 0"); err != nil {
		db.Exec("INSERT INTO failures VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		tx.Rollback()
		db.Exec("INSERT INTO failures VALUES (2)")
		return err
	}
	return tx.Commit()
}

// A variable declaration begins one too.
func Declared(db *sql.DB) error {
	var tx, err = db.Begin()
	if err != nil {
		return err
	}
	db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// The begin in an if's init, with its check.
func InIf(db *sql.DB) error {
	if tx, err := db.Begin(); err == nil {
		db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		return tx.Commit()
	} else {
		db.Exec("INSERT INTO failures VALUES (1)")
		return err
	}
}

// A goroutine started while the transaction is open runs while it is open.
func Goroutine(db *sql.DB) error {
	tx, _ := db.Begin()
	go func() {
		db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	}()
	return tx.Commit()
}

// A helper that takes the handle, as a query builder's Exec(db) does.
func Passed(db *sql.DB) error {
	tx, _ := db.Begin()
	use(db) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// A begin in a function literal belongs to the literal.
func Literal(db *sql.DB) {
	func() {
		tx, _ := db.Begin()
		db.Exec("INSERT INTO ledger VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		tx.Commit()
	}()
}

// The transaction stays open through a loop that does not end it.
func Loop(db *sql.DB, ids []int) error {
	tx, _ := db.Begin()
	for _, id := range ids {
		db.Exec("DELETE FROM users WHERE id = ?", id) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	}
	return tx.Commit()
}

// ===== Not reported =====

// The branch on the begin's own error has no transaction.
func Failed(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		db.Exec("INSERT INTO failures VALUES (1)")
		return err
	}
	return tx.Commit()
}

// After the commit, or a rollback, the handle is the only one left.
func After(db *sql.DB) error {
	tx, _ := db.Begin()
	tx.Exec("UPDATE accounts SET balance = 0")
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err := db.Exec("INSERT INTO ledger VALUES (1)")
	return err
}

// A deferred call runs when the function returns, after the commit.
func Deferred(db *sql.DB) error {
	tx, _ := db.Begin()
	defer db.Exec("INSERT INTO audit VALUES (1)")
	return tx.Commit()
}

// A branch that might have committed leaves the code after it outside.
func Maybe(db *sql.DB, done bool) error {
	tx, _ := db.Begin()
	if done {
		tx.Commit()
	}
	db.Exec("INSERT INTO ledger VALUES (1)")
	return nil
}

// A loop that ends each transaction it begins.
func PerRow(db *sql.DB, ids []int) {
	for _, id := range ids {
		db.Exec("SELECT 1")
		tx, _ := db.Begin()
		tx.Exec("DELETE FROM users WHERE id = ?", id)
		tx.Commit()
	}
}

// A new assignment to the variable leaves the transaction it held open. It
// is never committed, so a statement on the handle still bypasses it.
func Reassigned(db *sql.DB, other *sql.Tx) {
	tx, _ := db.Begin()
	tx = other
	db.Exec("SELECT 1") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	use(tx)
}

// A function literal that commits does not run where it is written.
func LiteralCommit(db *sql.DB) error {
	tx, _ := db.Begin()
	commit := func() error { return tx.Commit() }
	tx.Exec("UPDATE accounts SET balance = 0")
	return commit()
}

// Before the begin, nothing is open.
func Before(db *sql.DB) error {
	db.Exec("SELECT 1")
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Code no path reaches is not read.
func Unreachable(db *sql.DB) error {
	tx, _ := db.Begin()
	return tx.Commit()
	db.Exec("SELECT 1")
	return nil
}

// A check right after the begin that is not of its error leaves the
// transaction open in both branches.
func OtherCheck(db *sql.DB, done bool) error {
	tx, err := db.Begin()
	if done {
		db.Exec("SELECT 1") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// A blank transaction is not kept, so there is nothing to bypass.
func Blank(db *sql.DB) {
	_, _ = db.Begin()
	db.Exec("SELECT 1")
}

// A begin assigned to a field is not followed.
type Unit struct{ tx *sql.Tx }

func (u *Unit) Start(db *sql.DB) {
	u.tx, _ = db.Begin()
	db.Exec("SELECT 1")
}

// A call that is not a begin, assigned the same way.
func NotBegin(db *sql.DB) error {
	conn, err := db.Conn(context.Background())
	db.Exec("SELECT 1")
	use(conn)
	return errors.Join(err)
}

// A function's results are not a begin.
func NotMethod(db *sql.DB) error {
	n, err := strconv.Atoi("1")
	db.Exec("SELECT 1")
	use(n)
	return err
}

// A variable declared without a value begins nothing.
func Zero(db *sql.DB) {
	var tx *sql.Tx
	db.Exec("SELECT 1")
	use(tx)
}

// Not reported: the pool's own settings and statistics. Exec is reported.
func PoolMetadata(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	_ = db.Stats()
	db.SetMaxOpenConns(10)
	db.Exec("SELECT 1") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}
