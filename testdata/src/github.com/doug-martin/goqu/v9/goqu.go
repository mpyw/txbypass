// Package goqu is a stub of github.com/doug-martin/goqu/v9 for the analysistest fixtures.
//
// The signatures are those of github.com/doug-martin/goqu/v9 v9.19.0. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package goqu

import (
	"context"
	"database/sql"
)

// Database is a handle.
type Database struct{}

// TxDatabase is a transaction.
type TxDatabase struct{}

// InsertDataset is an INSERT statement.
type InsertDataset struct{}

func (d *Database) Insert(table interface{}) *InsertDataset { return &InsertDataset{} }

func (d *Database) Begin() (*TxDatabase, error) { return &TxDatabase{}, nil }

func (d *Database) BeginTx(ctx context.Context, opts *sql.TxOptions) (*TxDatabase, error) {
	return &TxDatabase{}, nil
}

func (d *Database) WithTx(fn func(*TxDatabase) error) error { return nil }

func (td *TxDatabase) Insert(table interface{}) *InsertDataset { return &InsertDataset{} }

func (td *TxDatabase) Commit() error { return nil }

func (td *TxDatabase) Rollback() error { return nil }
