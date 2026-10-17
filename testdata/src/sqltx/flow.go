package sqltx

import (
	"context"
	"database/sql"
	"log"

	"txhelper"
)

// ===== Values the transaction moves through =====

// A copy of the transaction is the transaction.
func Alias(db *sql.DB) {
	tx, _ := db.Begin()
	t := tx
	t.Commit()
	db.Exec("after commit via a copy")
}

// A pointer to the transaction's variable reaches the transaction.
func Pointer(db *sql.DB) {
	tx, _ := db.Begin()
	p := &tx
	(*p).Commit()
	db.Exec("after commit via a pointer")
}

// A function literal called where it is written runs there.
func Closure(db *sql.DB) {
	tx, _ := db.Begin()
	func() { tx.Commit() }()
	db.Exec("after commit in a closure")
}

// A function literal held in a variable runs where it is called.
func ClosureLater(db *sql.DB) {
	tx, _ := db.Begin()
	commit := func() error { return tx.Commit() }
	db.Exec("before the call") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	commit()
	db.Exec("after the call")
}

// A helper of this package that ends the transaction.
func Helper(db *sql.DB) {
	tx, err := db.Begin()
	_ = finish(tx, err)
	db.Exec("after a helper")
}

func finish(tx *sql.Tx, err error) error { // want finish:"closes\\[0\\]"
	if err != nil {
		return tx.Rollback()
	}
	return tx.Commit()
}

// A helper of another package, through the fact it exports.
func ForeignHelper(db *sql.DB) {
	tx, err := db.Begin()
	_ = txhelper.Finish(tx, err)
	db.Exec("after a helper of another package")
	tx2, _ := db.Begin()
	txhelper.Close(tx2)
	db.Exec("after a helper that calls a helper")
	tx3, _ := db.Begin()
	txhelper.Later(tx3)
	db.Exec("after a helper that defers the rollback")
}

// A helper that keeps the transaction open does not end it.
func KeepingHelper(db *sql.DB) {
	tx, _ := db.Begin()
	tx = txhelper.Keep(tx)
	db.Exec("still inside") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	tx.Commit()
}

// A method of this package that ends the transaction it is given.
type unit struct{}

func (unit) done(tx *sql.Tx) { tx.Rollback() } // want done:"closes\\[1\\]"

func Method(db *sql.DB) {
	tx, _ := db.Begin()
	unit{}.done(tx)
	db.Exec("after a method")
}

// A helper that calls itself, and ends the transaction on one path.
func recurse(tx *sql.Tx, n int) { // want recurse:"closes\\[0\\]"
	if n == 0 {
		tx.Commit()
		return
	}
	recurse(tx, n-1)
}

func Recursive(db *sql.DB) {
	tx, _ := db.Begin()
	recurse(tx, 3)
	db.Exec("after a recursive helper")
}

// ===== Where the transaction goes, it may end =====

// Stored into a field, or sent away, the transaction may be ended by code
// this analysis does not read.
type holder struct{ tx *sql.Tx }

func Stored(db *sql.DB, h *holder) {
	tx, _ := db.Begin()
	h.tx = tx
	db.Exec("after the transaction escaped")
}

func Sent(db *sql.DB, ch chan *sql.Tx) {
	tx, _ := db.Begin()
	ch <- tx
	db.Exec("after the transaction escaped")
}

// A function literal that ends the transaction, given away to be run later.
func Given(db *sql.DB, run func(func() error)) {
	tx, _ := db.Begin()
	run(func() error { return tx.Commit() })
	db.Exec("after the closure was given away")
}

// A function value whose body is unknown is taken to keep it open.
func Unknown(db *sql.DB, f func(*sql.Tx)) {
	tx, _ := db.Begin()
	f(tx)
	db.Exec("after an unknown function") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	tx.Commit()
}

// ===== Paths that end =====

// A path that ends in log.Fatal does not come back.
func Fatal(db *sql.DB) {
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tx.Exec("UPDATE accounts SET balance = 0"); err != nil {
		tx.Rollback()
		log.Fatal(err)
	}
	db.Exec("still inside") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	tx.Commit()
}

// A path that panics does not come back either.
func Panics(db *sql.DB) {
	tx, _ := db.Begin()
	if _, err := tx.Exec("UPDATE accounts SET balance = 0"); err != nil {
		tx.Rollback()
		panic(err)
	}
	db.Exec("still inside") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	tx.Commit()
}

// ===== The begin's error =====

// The error is a named result that a deferred literal reads, so it lives
// in memory. Its check right after the begin is still the begin's.
func NamedErr(db *sql.DB) (err error) {
	var tx *sql.Tx
	tx, err = db.Begin()
	defer func() {
		if err != nil && tx != nil {
			tx.Rollback()
		}
	}()
	if err != nil {
		db.Exec("INSERT INTO failures VALUES (1)")
		return err
	}
	db.Exec("inside") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// The begin's own error, checked later, is still its own.
func LaterOwnCheck(db *sql.DB) error {
	tx, err := db.Begin()
	n := 1
	if err != nil {
		db.Exec("INSERT INTO failures VALUES (1)")
		return err
	}
	db.Exec("inside", n) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// ===== Copies of the handle =====

// A copy of the handle is the handle.
func CopiedHandle(ctx context.Context, db *sql.DB) error {
	d := db
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	d.ExecContext(ctx, "INSERT INTO ledger VALUES (1)") // want `^d bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// ===== More ways a transaction ends =====

// A generic helper is read through its origin.
func endAny[T any](tx *sql.Tx, _ T) { tx.Commit() } // want endAny:"closes\\[0\\]"

func Generic(db *sql.DB) {
	tx, _ := db.Begin()
	endAny(tx, 1)
	db.Exec("after a generic helper")
}

// A method value of the transaction ends it when it is called.
func BoundCommit(db *sql.DB) {
	tx, _ := db.Begin()
	commit := tx.Commit
	db.Exec("before the call") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	commit()
	db.Exec("after the call")
}

// A literal that ends the transaction, kept in a field to run later.
type deferred struct{ fn func() error }

func Kept(db *sql.DB, d *deferred) {
	tx, _ := db.Begin()
	d.fn = func() error { return tx.Commit() }
	db.Exec("after the literal was kept")
}

// The variable's address, given away.
type pointing struct{ tx **sql.Tx }

func Address(db *sql.DB, p *pointing) {
	tx, _ := db.Begin()
	p.tx = &tx
	db.Exec("after the address was given away")
}

// A transaction kept in a variable that a literal captures and that is
// assigned again still ends where it is committed.
func ReassignedCaptured(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { tx = nil }()
	tx.Commit()
	_, err = db.Exec("after the commit")
	return err
}

// A map insert gives the transaction away, as a field store does.
type sessions struct{ txs map[string]*sql.Tx }

func (s *sessions) Open(db *sql.DB, id string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	s.txs[id] = tx
	_, err = db.Exec("after the transaction went away")
	return err
}

// A literal that ends the transaction but is never made ends nothing: it
// never runs.
func NeverMade(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	_ = func() { tx.Commit() }
	db.Exec("still inside") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}
