package sqltx

import (
	"context"
	"database/sql"
	"errors"
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

// A deferred call inside a literal runs when the literal returns, while
// the transaction is open.
func DeferInLiteral(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	func() {
		defer db.Exec("inside the literal") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		tx.Exec("x")
	}()
	return tx.Commit()
}

// A captured handle assigned again inside the region holds another handle
// after it, so nothing about it is read in the region.
func ReassignedInRegion(db, other *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db = other
	db.Exec("on the other handle")
	defer func() { _ = db }()
	return tx.Commit()
}

// Helpers that call each other end the transaction through either one,
// whichever is read first.
func cycleA(tx *sql.Tx, n int) error { // want cycleA:"closes\\[0\\]"
	if n > 0 {
		return cycleB(tx, n-1)
	}
	return nil
}

func cycleB(tx *sql.Tx, n int) error { // want cycleB:"closes\\[0\\]"
	if n > 0 {
		return cycleA(tx, n-1)
	}
	return tx.Commit()
}

func UseCycle(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := cycleA(tx, 3); err != nil {
		return err
	}
	_, err = db.Exec("after the helpers")
	return err
}

// The same pair, with the helper that commits written first, so that it is
// read first.
func cycleCommits(tx *sql.Tx, n int) error { // want cycleCommits:"closes\\[0\\]"
	if n > 0 {
		return cyclePasses(tx, n-1)
	}
	return tx.Commit()
}

func cyclePasses(tx *sql.Tx, n int) error { // want cyclePasses:"closes\\[0\\]"
	if n > 0 {
		return cycleCommits(tx, n-1)
	}
	return nil
}

func UseCyclePasses(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := cyclePasses(tx, 3); err != nil {
		return err
	}
	_, err = db.Exec("after the helpers")
	return err
}

// The begin's error is a named result read by a deferred literal, and a
// branch sits between the begin and its check. The check is still the
// begin's.
func NamedErrBranch(ctx context.Context, db *sql.DB, debug bool) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if debug {
		log.Println("begin")
	}
	if err != nil {
		db.ExecContext(ctx, "INSERT INTO failures VALUES (1)")
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	db.ExecContext(ctx, "inside") // want `^db bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
	_, err = tx.ExecContext(ctx, "UPDATE accounts SET n = n + 1")
	return err
}

// When another error is stored on one path, the check is not the begin's.
func NamedErrOther(ctx context.Context, db *sql.DB, check func() error, strict bool) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if strict {
		err = check()
	}
	defer func() { _ = err }()
	if err != nil {
		db.ExecContext(ctx, "maybe inside") // want `^db bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
		return err
	}
	return tx.Commit()
}

// The check of a named error inside a loop is still the begin's, when the
// loop stores no other error.
func NamedErrLoop(db *sql.DB, n int) (err error) {
	tx, err := db.Begin()
	defer func() { _ = err }()
	for range n {
		if err != nil {
			db.Exec("INSERT INTO failures VALUES (1)")
			return err
		}
	}
	return tx.Commit()
}

// A variable in memory stored on one path only, compared with nil while the
// transaction is open, is not the begin's error.
func StoredOnOnePath(db *sql.DB, retry bool) (err error) {
	var last error
	defer func() { _ = last }()
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	if retry {
		last = errors.New("retry")
	}
	if last != nil {
		db.Exec("y") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	}
	return tx.Commit()
}
