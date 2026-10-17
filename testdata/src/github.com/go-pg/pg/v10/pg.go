// Package pg is a stub of github.com/go-pg/pg/v10 for the analysistest fixtures.
//
// The signatures are those of github.com/go-pg/pg/v10 v10.14.0. Only what the fixtures use is
// kept, and the bodies do nothing. BSD 2-Clause License.
package pg

import (
	"context"
)

type baseDB struct{}

// DB is a handle.
type DB struct {
	*baseDB
}

// Tx is a transaction.
type Tx struct{}

func (db *baseDB) Exec(query interface{}, params ...interface{}) (interface{}, error) {
	return nil, nil
}

func (db *baseDB) Begin() (*Tx, error) { return &Tx{}, nil }

func (db *baseDB) BeginContext(ctx context.Context) (*Tx, error) { return &Tx{}, nil }

func (db *baseDB) RunInTransaction(ctx context.Context, fn func(*Tx) error) error { return nil }

func (tx *Tx) Exec(query interface{}, params ...interface{}) (interface{}, error) { return nil, nil }

// Begin returns current transaction. It does not start new transaction.
func (tx *Tx) Begin() (*Tx, error) { return tx, nil }

// RunInTransaction runs fn on the same transaction.
func (tx *Tx) RunInTransaction(ctx context.Context, fn func(*Tx) error) error { return fn(tx) }

func (tx *Tx) Commit() error { return nil }

func (tx *Tx) CommitContext(ctx context.Context) error { return nil }

func (tx *Tx) Rollback() error { return nil }
