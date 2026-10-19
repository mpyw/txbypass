// Package declaredtx covers helpers declared with directives.
package declaredtx

import (
	"context"
	"database/sql"

	"declaredlib"
)

// ===== Invokers =====

type Repo interface {
	declaredlib.Querier
	// InTx runs fn in a transaction on the repository.
	//
	//txbypass:invokes
	InTx(ctx context.Context, fn func(ctx context.Context, q declaredlib.Querier) error) error // want InTx:`^invokes\[p0, p2\(1\)\]$`
}

// Manager carries the transaction in a context, as a transaction manager
// does.
type Manager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error //txbypass:invokes // want Do:`^invokes\[p1, p2\(0\), context\]$`
}

func save(ctx context.Context) error { return nil }

func Invoked(ctx context.Context, r Repo, s declaredlib.Store, m Manager) error {
	if err := r.InTx(ctx, func(ctx context.Context, q declaredlib.Querier) error {
		_, err := r.ExecContext(ctx, "q") // want `^r bypasses the transaction opened with InTx; use q instead \[outer-handle\]$`
		return err
	}); err != nil {
		return err
	}
	if err := s.WithTx(ctx, func(q declaredlib.Querier) error {
		_, err := s.ExecContext(ctx, "q") // want `^s bypasses the transaction opened with WithTx; use q instead \[outer-handle\]$`
		return err
	}); err != nil {
		return err
	}
	return m.Do(ctx, func(txCtx context.Context) error {
		return save(ctx) // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
	})
}

// ===== Begins and ends =====

// Tx ends with Done, a name txbypass does not know.
type Tx interface {
	declaredlib.Querier
	//txbypass:closes
	Done(err error) error // want Done:`^closes\[0 1\]$`
}

type DB interface {
	declaredlib.Querier
	//txbypass:begins
	Begin(ctx context.Context) (Tx, error) // want Begin:`^begins\[p0\]$`
}

func Begun(ctx context.Context, db DB) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	db.ExecContext(ctx, "q") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	if err := tx.Done(nil); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "after")
	return err
}

// Conn begins a transaction that is a Conn too, as gorm's DB does.
type Conn struct{}

func (*Conn) Exec(ctx context.Context, query string) error { return nil }

// Start begins a transaction. It returns its receiver's type, yet gives
// the transaction, not the receiver.
//
//txbypass:begins
func (c *Conn) Start() *Conn { return c } // want Start:`^begins\[p0\]$`

//txbypass:closes
func (*Conn) Finish() error { return nil } // want Finish:`^closes\[0\]$`

func Started(ctx context.Context, db *Conn, pool *declaredlib.Pool) error {
	tx := db.Start()
	tx.Exec(ctx, "in")
	db.Exec(ctx, "q") // want `^db bypasses the transaction opened with Start; use tx instead \[outer-handle\]$`
	if err := tx.Finish(); err != nil {
		return err
	}
	lib := pool.Conn()
	ltx := lib.Begin()
	ltx.Exec(ctx, "in")
	lib.Exec(ctx, "q") // want `^lib bypasses the transaction opened with Begin; use ltx instead \[outer-handle\]$`
	if err := ltx.End(); err != nil {
		return err
	}
	s, err := pool.Open(ctx)
	if err != nil {
		return err
	}
	// Using binds the pool to the session, so it is not the pool.
	pool.Using(s).Exec(ctx, "in")
	return s.End()
}

// ===== A declaration wins over the body =====

type Store struct {
	db    *sql.DB
	audit *sql.DB
	hooks []func(*sql.Tx) error
	run   func(context.Context, func(*sql.Tx) error) error
}

// keep stores fn, so its body does not read as a helper. The declaration
// says it is one.
//
//txbypass:invokes
func (s *Store) keep(ctx context.Context, fn func(*sql.Tx) error) error { // want keep:`^invokes\[p0\.db, p2\(0\)\]$`
	s.hooks = append(s.hooks, fn)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Kept(ctx context.Context) error {
	return s.keep(ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "q") // want `^s\.db bypasses the transaction opened with keep; use tx instead \[outer-handle\]$`
		return err
	})
}

// delegate opens nothing in its body, so the receiver is the handle.
//
//txbypass:invokes
func (s *Store) delegate(ctx context.Context, fn func(*sql.Tx) error) error { // want delegate:`^invokes\[p0, p2\(0\)\]$`
	return s.run(ctx, fn)
}

// either opens on two handles, so the receiver is the handle.
//
//txbypass:invokes
func (s *Store) either(ctx context.Context, audit bool, fn func(*sql.Tx) error) error { // want either:`^invokes\[p0, p3\(0\)\]$`
	var tx *sql.Tx
	var err error
	if audit {
		tx, err = s.audit.BeginTx(ctx, nil)
	} else {
		tx, err = s.db.BeginTx(ctx, nil)
	}
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

var global *sql.DB

// onGlobal opens on a package-level variable, so its parameter is the
// handle.
//
//txbypass:invokes
func onGlobal(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error { // want onGlobal:`^invokes\[p1, p2\(0\)\]$`
	tx, err := global.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// finish ends the transaction it is given, whatever its body says.
//
//txbypass:closes
func finish(tx *sql.Tx) {} // want finish:`^closes\[0\]$`

func Finished(ctx context.Context, db *sql.DB, other Tx, dbs DB) error { // want Finished:`^closes\[2\]$`
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	finish(tx)
	_, err = db.ExecContext(ctx, "after")
	begun, err := dbs.Begin(ctx)
	if err != nil {
		return err
	}
	// Another transaction ends, so this one stays open.
	other.Done(nil)
	dbs.ExecContext(ctx, "q") // want `^dbs bypasses the transaction opened with Begin; use begun instead \[outer-handle\]$`
	return begun.Done(nil)
}

// ===== Problems =====

//txbypass:invokes // no callback // want `^txbypass:invokes needs a parameter of a function type$`
func noCallback(db *sql.DB) error { return nil }

//txbypass:invokes // no handle // want `^txbypass:invokes needs a handle: a receiver, or a parameter that is neither a context nor a function$`
func noHandle(ctx context.Context, fn func(*sql.Tx) error) error { return nil }

//txbypass:invokes // no context for the callback's // want `^txbypass:invokes needs a context parameter, since the callback takes only a context$`
func noContext(db *sql.DB, fn func(context.Context) error) error { return nil }

//txbypass:invokes // the callback takes nothing // want `^txbypass:invokes needs a callback that takes a transaction or a context$`
func emptyCallback(db *sql.DB, fn func() error) error { return nil }

//txbypass:begins // no result // want `^txbypass:begins needs a transaction as its first result, and nothing but an error after it$`
func noResult(db *sql.DB) {}

//txbypass:begins // second result // want `^txbypass:begins needs a transaction as its first result, and nothing but an error after it$`
func notError(db *sql.DB) (*sql.Tx, int) { return nil, 0 }

//txbypass:begins // a number // want `^txbypass:begins needs a transaction as its first result, and nothing but an error after it$`
func openNumber(db *sql.DB) (int, error) { return 0, nil }

//txbypass:begins // no handle // want `^txbypass:begins needs a handle: a receiver, or a parameter that is neither a context nor a function$`
func beginNoHandle(ctx context.Context) (*sql.Tx, error) { return nil, nil }

//txbypass:closes // nothing to end // want `^txbypass:closes needs a receiver or a parameter$`
func closeNothing() error { return nil }

//txbypass:invokes extra // want `^txbypass:invokes takes no argument; write a reason after //$`
func withArgument(db *sql.DB, fn func(*sql.Tx) error) error { return nil }

func misplaced() {
	//txbypass:begins // not on a declaration // want `^txbypass:begins belongs in the doc comment of a function or an interface method$`
	_ = 0
}

// retry calls itself, so reading its body reaches its own declaration.
//
//txbypass:invokes
func (s *Store) retry(ctx context.Context, n int, fn func(*sql.Tx) error) error { // want retry:`^invokes\[p0\.db, p3\(0\)\]$`
	if n > 0 {
		return s.retry(ctx, n-1, fn)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func Ended(ctx context.Context, p *declaredlib.Pool) error {
	s, err := p.Open(ctx)
	if err != nil {
		return err
	}
	p.Exec(ctx, "q") // want `^p bypasses the transaction opened with Open; use s instead \[outer-handle\]$`
	if err := s.End(); err != nil {
		return err
	}
	return p.Exec(ctx, "after")
}

func Generated(ctx context.Context, g Gen) error {
	return g.InTx(ctx, func(tx *sql.Tx) error {
		_, err := g.ExecContext(ctx, "q") // want `^g bypasses the transaction opened with InTx; use tx instead \[outer-handle\]$`
		return err
	})
}

// finishTx ends tx through its declared end, so it ends tx too.
func finishTx(tx Tx) error { return tx.Done(nil) } // want finishTx:`^closes\[0\]$`

func FinishedThrough(ctx context.Context, db DB, p *declaredlib.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	if err := finishTx(tx); err != nil {
		return err
	}
	db.ExecContext(ctx, "after")
	s, err := p.Open(ctx)
	if err != nil {
		return err
	}
	if err := declaredlib.EndSession(s); err != nil {
		return err
	}
	return p.Exec(ctx, "after")
}

// Ender ends the transactions it is given. It holds none itself.
type Ender struct{}

// End ends tx.
//
//txbypass:closes
func (*Ender) End(tx *sql.Tx) error { return nil } // want End:`^closes\[0 1\]$`

// endWith ends tx, and not e, which is no transaction.
func endWith(e *Ender, tx *sql.Tx) error { return e.End(tx) } // want endWith:`^closes\[1\]$`

// TxEnder ends the declared transactions it is given. It holds none
// itself, though its end takes a Tx, which txbypass knows by a declaration
// only.
type TxEnder struct{}

// End ends tx.
//
//txbypass:closes
func (*TxEnder) End(tx Tx) error { return nil } // want End:`^closes\[0 1\]$`

// endTx ends tx, and not e, which is no transaction.
func endTx(e *TxEnder, tx Tx) error { return e.End(tx) } // want endTx:`^closes\[1\]$`

// A declared manager called inside its own callback joins the transaction
// the callback's context carries, as go-transaction-manager's Do does.
func Joined(ctx context.Context, m Manager) error {
	return m.Do(ctx, func(outer context.Context) error {
		return m.Do(outer, func(inner context.Context) error {
			return save(outer)
		})
	})
}
