package helpertx

import (
	"context"
	"database/sql"
	"errors"
)

// twice runs fn in two places, in one transaction on s.db.
func (s *Store) twice(ctx context.Context, fn func(*sql.Tx) error) error { // want twice:`^invokes\[p0\.db, p2\(0\)\]$`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// split runs fn on s.db, or on s.audit. No one handle is the outer one.
func (s *Store) split(ctx context.Context, audit bool, fn func(*sql.Tx) error) error {
	db := s.db
	if audit {
		tx, err := s.audit.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		return fn(tx)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return fn(tx)
}

// on takes the handle as an argument of a method.
func (s *Store) on(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error { // want on:`^invokes\[p2, p3\(0\)\]$`
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) On(ctx context.Context) error {
	return s.on(ctx, s.audit, func(tx *sql.Tx) error {
		_, err := s.audit.ExecContext(ctx, "q") // want `^s\.audit bypasses the transaction opened with on; use tx instead \[outer-handle\]$`
		return err
	})
}

// TxFunc is a callback that runs itself in a transaction.
type TxFunc func(*sql.Tx) error

func (fn TxFunc) Run(ctx context.Context, db *sql.DB) error { // want Run:`^invokes\[p2, p0\(0\)\]$`
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ===== Not read =====

// The callback is the receiver, so the selector names no handle.
func Receiver(ctx context.Context, db *sql.DB) error {
	return TxFunc(func(tx *sql.Tx) error {
		_, err := db.ExecContext(ctx, "q")
		return err
	}).Run(ctx, db)
}

// A method expression takes its receiver as an argument.
func Expression(ctx context.Context, s *Store) error {
	return (*Store).withTx(s, ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "q")
		return err
	})
}

func triple(db *sql.DB) (context.Context, *sql.DB, func(context.Context, *sql.Tx) error) {
	return context.Background(), db, nil
}

func pair() (context.Context, func(*sql.Tx) error) {
	return context.Background(), nil
}

// The arguments are one call with several results.
func Results(db *sql.DB, s *Store) error {
	if err := RunInTx(triple(db)); err != nil {
		return err
	}
	return s.withTx(pair())
}

// viaNil opens on nil, which is no handle.
func viaNil(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return RunInTx(ctx, nil, func(ctx context.Context, tx *sql.Tx) error {
		return fn(ctx, tx)
	})
}

// dead begins only in code that no path reaches.
func (s *Store) dead(ctx context.Context, fn func(*sql.Tx) error) error {
	return fn(nil)
	tx, _ := s.db.BeginTx(ctx, nil)
	return fn(tx)
}
