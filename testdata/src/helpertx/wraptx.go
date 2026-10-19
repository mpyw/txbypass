// Package helpertx holds helpers of a project that open a transaction and run
// a function they are given inside it.
package helpertx

import (
	"context"
	"database/sql"
	"errors"

	"gorm.io/gorm"

	"txhelper"
)

type Store struct {
	db    *sql.DB
	audit *sql.DB
	hooks []func(*sql.Tx) error
}

// ===== Helpers =====

// withTx begins a transaction on s.db, and runs fn inside it.
func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error { // want withTx:`^invokes\[p0\.db, p2\(0\)\]$`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

// retry runs fn through withTx, so it opens on s.db too.
func (s *Store) retry(ctx context.Context, fn func(*sql.Tx) error) error { // want retry:`^invokes\[p0\.db, p2\(0\)\]$`
	var err error
	for range 3 {
		err = s.withTx(ctx, func(tx *sql.Tx) error {
			return fn(tx)
		})
		if err == nil {
			return nil
		}
	}
	return err
}

// RunInTx takes the handle as an argument.
func RunInTx(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error { // want RunInTx:`^invokes\[p1, p2\(1\)\]$`
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// guarded checks fn against nil first. A comparison hands fn nowhere.
func (s *Store) guarded(ctx context.Context, fn func(*sql.Tx) error) error { // want guarded:`^invokes\[p0\.db, p2\(0\)\]$`
	if fn == nil {
		return errors.New("no function")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		return fn(tx)
	})
}

type Repo struct {
	db *gorm.DB
}

// InTx runs fn in the callback of a known invoker.
func (r *Repo) InTx(fn func(tx *gorm.DB) error) error { // want InTx:`^invokes\[p0\.db, p1\(0\)\]$`
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// ===== Reported =====

func (s *Store) Place(ctx context.Context) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "insert"); err != nil {
			return err
		}
		_, err := s.db.ExecContext(ctx, "audit") // want `^s\.db bypasses the transaction opened with withTx; use tx instead \[outer-handle\]$`
		return err
	})
}

func (s *Store) Retried(ctx context.Context) error {
	return s.retry(ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "audit") // want `^s\.db bypasses the transaction opened with retry; use tx instead \[outer-handle\]$`
		return err
	})
}

func Plain(ctx context.Context, db *sql.DB) error {
	return RunInTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := db.ExecContext(ctx, "audit") // want `^db bypasses the transaction opened with helpertx\.RunInTx; use tx instead \[outer-handle\]$`
		return err
	})
}

func Imported(ctx context.Context, db *sql.DB) error {
	return txhelper.RunInTx(ctx, db, func(tx *sql.Tx) error {
		_, err := db.ExecContext(ctx, "audit") // want `^db bypasses the transaction opened with txhelper\.RunInTx; use tx instead \[outer-handle\]$`
		return err
	})
}

func (r *Repo) Save(v any) error {
	return r.InTx(func(tx *gorm.DB) error {
		return r.db.Create(v).Error // want `^r\.db bypasses the transaction opened with InTx; use tx instead \[outer-handle\]$`
	})
}

func (s *Store) Guarded(ctx context.Context) error {
	return s.guarded(ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "audit") // want `^s\.db bypasses the transaction opened with guarded; use tx instead \[outer-handle\]$`
		return err
	})
}

// App reaches withTx through the Store it embeds.
type App struct {
	*Store
}

func (a *App) Run(ctx context.Context) error {
	return a.withTx(ctx, func(tx *sql.Tx) error {
		_, err := a.db.ExecContext(ctx, "audit") // want `^a\.db bypasses the transaction opened with withTx; use tx instead \[outer-handle\]$`
		return err
	})
}

// ===== Not reported =====

// Another field of the receiver is not the handle the helper opens on.
func (s *Store) Audit(ctx context.Context) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		_, err := s.audit.ExecContext(ctx, "audit")
		return err
	})
}

// reuse runs fn in a transaction it is given, when there is one. fn runs
// outside the transaction it begins on that path, so it is no helper.
func (s *Store) reuse(ctx context.Context, cur *sql.Tx, fn func(*sql.Tx) error) error {
	if cur != nil {
		return fn(cur)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func (s *Store) Reused(ctx context.Context) error {
	return s.reuse(ctx, nil, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "audit")
		return err
	})
}

// after runs fn once the transaction has committed.
func (s *Store) after(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return fn(tx)
}

func (s *Store) After(ctx context.Context) error {
	return s.after(ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "audit")
		return err
	})
}

// keep stores fn, which may run anywhere.
func (s *Store) keep(ctx context.Context, fn func(*sql.Tx) error) error {
	s.hooks = append(s.hooks, fn)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func (s *Store) Kept(ctx context.Context) error {
	return s.keep(ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "audit")
		return err
	})
}

// background runs fn in a goroutine, which may outlive the transaction.
func (s *Store) background(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	go fn(tx)
	return tx.Commit()
}

func (s *Store) Background(ctx context.Context) error {
	return s.background(ctx, func(tx *sql.Tx) error {
		_, err := s.db.ExecContext(ctx, "audit")
		return err
	})
}

var global *sql.DB

// withGlobal opens on a package-level variable, which no parameter names.
func withGlobal(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := global.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func Global(ctx context.Context) error {
	return withGlobal(ctx, func(tx *sql.Tx) error {
		_, err := global.ExecContext(ctx, "audit")
		return err
	})
}

// The callback is not a function literal, so its body is not read here.
func (s *Store) Named(ctx context.Context) error {
	return s.withTx(ctx, s.write)
}

func (s *Store) write(tx *sql.Tx) error {
	_, err := s.db.Exec("audit")
	return err
}

// gormTx runs fn in gorm's Transaction, which opens a savepoint when db is
// a transaction.
func gormTx(db *gorm.DB, fn func(*gorm.DB) error) error { // want gormTx:`^invokes\[p0, p1\(0\)\]$`
	return db.Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// A savepoint opened through a helper runs on the outer transaction's
// connection, as one opened by gorm's Transaction does.
func Savepoint(db *gorm.DB, v any) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return gormTx(tx, func(sp *gorm.DB) error {
			return tx.Create(v).Error
		})
	})
}

// run runs fn in a transaction on db, and onErr on db when it fails.
func run(db *gorm.DB, fn func(*gorm.DB) error, onErr func(*gorm.DB)) error { // want run:`^invokes\[p0, p1\(0\)\]$`
	if err := db.Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	}); err != nil {
		onErr(db)
		return err
	}
	return nil
}

// onErr is given the handle, not a transaction, so a transaction opened on
// it is no savepoint.
func OnErr(db *gorm.DB, v any) error {
	return run(db, func(tx *gorm.DB) error {
		return tx.Create(v).Error
	}, func(other *gorm.DB) {
		other.Transaction(func(sp *gorm.DB) error {
			return other.Create(v).Error // want `^other bypasses the transaction opened with Transaction; use sp instead \[outer-handle\]$`
		})
	})
}
