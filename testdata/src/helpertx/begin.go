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

func (s *Store) NotBegun(ctx context.Context) error {
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
