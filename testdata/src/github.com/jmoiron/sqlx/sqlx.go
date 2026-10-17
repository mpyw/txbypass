// Package sqlx is a stub of github.com/jmoiron/sqlx for the analysistest fixtures.
//
// The signatures are those of github.com/jmoiron/sqlx v1.4.0. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package sqlx

import (
	"context"
	"database/sql"
)

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

func (db *DB) Get(dest interface{}, query string, args ...interface{}) error { return nil }

func (db *DB) MustExec(query string, args ...interface{}) sql.Result { return nil }

func (db *DB) Beginx() (*Tx, error) { return &Tx{}, nil }

func (db *DB) MustBegin() *Tx { return &Tx{} }

func (db *DB) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) { return &Tx{}, nil }

func (db *DB) MustBeginTx(ctx context.Context, opts *sql.TxOptions) *Tx { return &Tx{} }

func (c *Conn) BeginTxx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) { return &Tx{}, nil }

func (tx *Tx) Get(dest interface{}, query string, args ...interface{}) error { return nil }

func (tx *Tx) MustExec(query string, args ...interface{}) sql.Result { return nil }
