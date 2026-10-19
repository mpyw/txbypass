package helpertx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"txhelper"
)

// ===== Begins =====

// begin returns a transaction it begins on s.db.
func (s *Store) begin(ctx context.Context) (*sql.Tx, error) { // want begin:`^begins\[p0\.db\]$`
	return s.db.BeginTx(ctx, nil)
}

// beginWrapped wraps the error, and returns nil with it.
func (s *Store) beginWrapped(ctx context.Context) (*sql.Tx, error) { // want beginWrapped:`^begins\[p0\.db\]$`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	return tx, nil
}

// beginAgain begins through begin.
func (s *Store) beginAgain(ctx context.Context) (*sql.Tx, error) { // want beginAgain:`^begins\[p0\.db\]$`
	return s.begin(ctx)
}

// BeginOn takes the handle as an argument.
func BeginOn(ctx context.Context, db *sql.DB) (*sql.Tx, error) { // want BeginOn:`^begins\[p1\]$`
	return db.BeginTx(ctx, nil)
}

// beginGorm returns gorm's transaction, which carries its own error.
func (r *Repo) beginGorm() *gorm.DB { // want beginGorm:`^begins\[p0\.db\]$`
	return r.db.Begin()
}

// inTx is an invoker built on a begin of the project.
func (s *Store) inTx(ctx context.Context, fn func(*sql.Tx) error) error { // want inTx:`^invokes\[p0\.db, p2\(0\)\]$`
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// ===== Reported =====

func (s *Store) Begun(ctx context.Context) error {
	tx, err := s.beginWrapped(ctx)
	if err != nil {
		_, err := s.db.ExecContext(ctx, "log")
		return err
	}
	defer tx.Rollback()
	if _, err := s.db.ExecContext(ctx, "audit"); err != nil { // want `^s\.db bypasses the transaction opened with beginWrapped; use tx instead \[outer-handle\]$`
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "after")
	return err
}

func BegunOn(ctx context.Context, db *sql.DB) error {
	tx, err := BeginOn(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	db.ExecContext(ctx, "audit") // want `^db bypasses the transaction opened with helpertx\.BeginOn; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

func BegunImported(ctx context.Context, db *sql.DB) error {
	tx, err := txhelper.Begin(ctx, db)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	db.ExecContext(ctx, "audit") // want `^db bypasses the transaction opened with txhelper\.Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

func (r *Repo) BegunGorm(v any) error {
	tx := r.beginGorm()
	if tx.Error != nil {
		return tx.Error
	}
	r.db.Create(v) // want `^r\.db bypasses the transaction opened with beginGorm; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}

func (s *Store) InTx(ctx context.Context) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "audit") // want `^s\.db bypasses the transaction opened with inTx; use tx instead \[outer-handle\]$`
		return err
	})
}

// ===== Not begins =====

// committed returns a transaction it has committed.
func (s *Store) committed(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return tx, tx.Commit()
}

// current returns a transaction it is given, when there is one.
func (s *Store) current(ctx context.Context, cur *sql.Tx) (*sql.Tx, error) {
	if cur != nil {
		return cur, nil
	}
	return s.db.BeginTx(ctx, nil)
}

// either begins on s.db or on s.audit.
func (s *Store) either(ctx context.Context, audit bool) (*sql.Tx, error) {
	if audit {
		return s.audit.BeginTx(ctx, nil)
	}
	return s.db.BeginTx(ctx, nil)
}

// beginGlobal begins on a package-level variable.
func beginGlobal(ctx context.Context) (*sql.Tx, error) {
	return global.BeginTx(ctx, nil)
}

// Tx wraps a transaction of database/sql.
type Tx struct {
	*sql.Tx
}

// beginTx returns the transaction wrapped in a struct.
func (s *Store) beginTx(ctx context.Context) (*Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	return &Tx{tx}, err
}

// three returns more than a transaction and an error.
func (s *Store) three(ctx context.Context) (*sql.Tx, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	return tx, 0, err
}

// second returns a transaction second.
func (s *Store) second(ctx context.Context) (int, *sql.Tx) {
	tx, _ := s.db.BeginTx(ctx, nil)
	return 0, tx
}

// noError returns something else than an error second.
func (s *Store) noError(ctx context.Context) (*sql.Tx, int) {
	tx, _ := s.db.BeginTx(ctx, nil)
	return tx, 0
}

// setUp returns the transaction it began with an error of its own, which
// leaves the transaction open where the caller reads a failed begin.
func (s *Store) setUp(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "set"); err != nil {
		return tx, errors.New("set up")
	}
	return tx, nil
}

// beginWith takes the handle first, so that one call with several results
// fills it.
func beginWith(db *sql.DB, ctx context.Context) (*sql.Tx, error) { // want beginWith:`^begins\[p0\]$`
	return db.BeginTx(ctx, nil)
}

func withCtx(db *sql.DB) (*sql.DB, context.Context) {
	return db, context.Background()
}

// beginOn takes the handle after the receiver.
func (s *Store) beginOn(ctx context.Context, db *sql.DB) (*sql.Tx, error) { // want beginOn:`^begins\[p2\]$`
	return db.BeginTx(ctx, nil)
}

func (s *Store) NotBegun(ctx context.Context) error {
	if setUp, err := s.setUp(ctx); err == nil {
		s.db.ExecContext(ctx, "q")
		setUp.Commit()
	}
	// The arguments are one call with several results.
	several, _ := beginWith(withCtx(s.db))
	s.db.ExecContext(ctx, "q")
	several.Commit()
	// A method expression takes its receiver as an argument, and its other
	// arguments one place off.
	onExpr, _ := (*Store).beginOn(s, ctx, s.db)
	s.db.ExecContext(ctx, "q")
	onExpr.Commit()
	committed, _ := s.committed(ctx)
	s.db.ExecContext(ctx, "q")
	committed.Commit()
	either, _ := s.either(ctx, false)
	s.db.ExecContext(ctx, "q")
	either.Commit()
	tx, _ := s.current(ctx, nil)
	s.db.ExecContext(ctx, "q")
	tx.Commit()
	// A method expression takes its receiver as an argument.
	viaExpr, _ := (*Store).begin(s, ctx)
	s.db.ExecContext(ctx, "q")
	viaExpr.Commit()
	wrapped, _ := s.beginTx(ctx)
	s.db.ExecContext(ctx, "q")
	return wrapped.Commit()
}

// viaNilBegin begins on nil, which is no handle.
func viaNilBegin(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := BeginOn(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return fn(tx)
}

// ===== Savepoints on a helper's transaction =====

// A savepoint on the transaction a helper gives its callback runs on that
// transaction's connection.
func (r *Repo) SavepointInHelper(v any) error {
	return r.InTx(func(tx *gorm.DB) error {
		return tx.Transaction(func(sp *gorm.DB) error {
			return tx.Create(v).Error
		})
	})
}

// So does one on the transaction a helper begins.
func (r *Repo) SavepointOnBegun(v any) error {
	tx := r.beginGorm()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Transaction(func(sp *gorm.DB) error {
		return tx.Create(v).Error
	}); err != nil {
		return err
	}
	return tx.Commit().Error
}

// rolledBack rolls the transaction back when it returns, so the caller
// gets one that has ended.
func (s *Store) rolledBack(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return tx, nil
}

func (s *Store) RolledBack(ctx context.Context) error {
	tx, err := s.rolledBack(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "q")
	return errors.Join(err, tx.Commit())
}

// beginNamed returns through named results, which a deferred literal reads.
func (s *Store) beginNamed(ctx context.Context) (tx *sql.Tx, err error) { // want beginNamed:`^begins\[p0\.db\]$`
	defer func() {
		if err != nil {
			err = fmt.Errorf("begin: %w", err)
		}
	}()
	tx, err = s.db.BeginTx(ctx, nil)
	return tx, err
}

// beginBare returns through named results, with a bare return.
func (s *Store) beginBare(ctx context.Context) (tx *sql.Tx, err error) { // want beginBare:`^begins\[p0\.db\]$`
	tx, err = s.db.BeginTx(ctx, nil)
	return
}

func (s *Store) NamedResults(ctx context.Context) error {
	tx, err := s.beginNamed(ctx)
	if err != nil {
		return err
	}
	s.db.ExecContext(ctx, "q") // want `^s\.db bypasses the transaction opened with beginNamed; use tx instead \[outer-handle\]$`
	if err := tx.Commit(); err != nil {
		return err
	}
	bare, err := s.beginBare(ctx)
	if err != nil {
		return err
	}
	s.db.ExecContext(ctx, "q") // want `^s\.db bypasses the transaction opened with beginBare; use bare instead \[outer-handle\]$`
	return bare.Commit()
}

// beginLoop copies its error around a loop before it returns it.
func (s *Store) beginLoop(ctx context.Context, n int) (tx *sql.Tx, err error) { // want beginLoop:`^begins\[p0\.db\]$`
	defer func() {
		if err != nil {
			err = fmt.Errorf("begin: %w", err)
		}
	}()
	tx, err = s.db.BeginTx(ctx, nil)
	for range n {
		e := err
		err = e
	}
	return tx, err
}

// beginGormErr returns gorm's transaction beside its own error.
func (r *Repo) beginGormErr() (*gorm.DB, error) { // want beginGormErr:`^begins\[p0\.db\]$`
	tx := r.db.Begin()
	return tx, tx.Error
}

var errSetup = errors.New("setup")

// beginGormOther returns gorm's open transaction beside another error, so
// a caller that stops on the error leaves it open where it reads none.
func (r *Repo) beginGormOther() (*gorm.DB, error) {
	tx := r.db.Begin()
	return tx, errSetup
}

func (r *Repo) GormErr(v any) error {
	tx, err := r.beginGormErr()
	if err != nil {
		r.db.Create(v)
		return err
	}
	r.db.Create(v) // want `^r\.db bypasses the transaction opened with beginGormErr; use tx instead \[outer-handle\]$`
	if err := tx.Commit().Error; err != nil {
		return err
	}
	other, err := r.beginGormOther()
	if err != nil {
		return err
	}
	r.db.Create(v)
	return other.Commit().Error
}

// maybeBegin may begin nothing, with no error.
func (s *Store) maybeBegin(ctx context.Context, on bool) (*sql.Tx, error) {
	if !on {
		return nil, nil
	}
	return s.db.BeginTx(ctx, nil)
}

// beginOverride may set its error after the begin succeeded.
func (s *Store) beginOverride(ctx context.Context, fail bool) (tx *sql.Tx, err error) {
	defer func() {
		if fail {
			err = errSetup
		}
	}()
	tx, err = s.db.BeginTx(ctx, nil)
	return tx, err
}

// beginReset hands its error to a function that may set it.
func (s *Store) beginReset(ctx context.Context) (tx *sql.Tx, err error) {
	defer func() {
		reset(&err)
	}()
	tx, err = s.db.BeginTx(ctx, nil)
	return tx, err
}

func reset(err *error) { *err = nil }

func (s *Store) MaybeBegun(ctx context.Context) error {
	tx, err := s.maybeBegin(ctx, false)
	if err != nil {
		return err
	}
	s.db.ExecContext(ctx, "q")
	if tx != nil {
		return tx.Commit()
	}
	over, err := s.beginOverride(ctx, true)
	if err != nil {
		return err
	}
	s.db.ExecContext(ctx, "q")
	if err := over.Commit(); err != nil {
		return err
	}
	reset, err := s.beginReset(ctx)
	if err != nil {
		return err
	}
	s.db.ExecContext(ctx, "q")
	return reset.Commit()
}

// beginNilFirst wraps its error under nil != err.
func (s *Store) beginNilFirst(ctx context.Context) (tx *sql.Tx, err error) { // want beginNilFirst:`^begins\[p0\.db\]$`
	defer func() {
		if nil != err {
			err = fmt.Errorf("begin: %w", err)
		}
	}()
	tx, err = s.db.BeginTx(ctx, nil)
	return tx, err
}
