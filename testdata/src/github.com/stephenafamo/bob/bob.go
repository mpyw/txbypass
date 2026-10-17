// Package bob is a stub of github.com/stephenafamo/bob for the analysistest fixtures.
//
// The signatures are those of github.com/stephenafamo/bob v0.42.0. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package bob

import (
	"context"
	"database/sql"
)

// Executor runs statements.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Transaction is an Executor that commits or rolls back.
type Transaction interface {
	Executor
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// DB wraps a *sql.DB.
type DB struct {
	*sql.DB
}

// Conn wraps a *sql.Conn.
type Conn struct {
	*sql.Conn
}

// Tx wraps a *sql.Tx.
type Tx struct {
	*sql.Tx
}

func (d DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) { return Tx{}, nil }

func (d DB) RunInTx(ctx context.Context, txOptions *sql.TxOptions, fn func(context.Context, Transaction) error) error {
	return nil
}

func (d Conn) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) { return Tx{}, nil }

func (t Tx) Commit(_ context.Context) error { return nil }

func (t Tx) Rollback(_ context.Context) error { return nil }
