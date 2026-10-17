package sqltx

import (
	"database/sql"
	"log"
)

// ===== Code no path reaches =====

// Not reported, and no crash: the begin is after a return, so it never runs.
func Disabled(db *sql.DB) error {
	return nil
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db.Exec("SELECT 1")
	return tx.Commit()
}

// The same after a call that never returns.
func FatalFirst(db *sql.DB) {
	log.Fatal("not yet")
	tx, _ := db.Begin()
	db.Exec("SELECT 1")
	tx.Commit()
}

// A literal assigned to the blank identifier is never made, so it never
// runs.
func BlankLiteral(db *sql.DB) {
	tx, _ := db.Begin()
	_ = func() { db.Exec("SELECT 1") }
	tx.Commit()
}

// Shared is a handle the package holds.
var Shared *sql.DB

// A goroutine with no free variables starts where it is written, while the
// transaction is open.
func SharedGoroutine() {
	tx, _ := Shared.Begin()
	go func() {
		Shared.Exec("SELECT 1") // want `^Shared bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	}()
	tx.Commit()
}
