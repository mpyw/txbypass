// Package flaglib is a library whose helpers are declared by flags.
package flaglib

import (
	"context"
	"database/sql"
)

// Manager runs callbacks in transactions, through code that is not read.
type Manager struct {
	DB  *sql.DB
	run func(context.Context, func(*sql.Tx) error) error
}

// InTx runs fn in a transaction on m.DB.
func (m *Manager) InTx(ctx context.Context, fn func(*sql.Tx) error) error {
	return m.run(ctx, fn)
}

// Begin begins a transaction on m.DB.
func (m *Manager) Begin(ctx context.Context) (*Tx, error) { return nil, nil }

// Tx is a transaction of the library.
type Tx struct{ tx *sql.Tx }

// Finish ends the transaction.
func (t *Tx) Finish() error { return nil }
