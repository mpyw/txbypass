// Package pop is a stub of github.com/gobuffalo/pop/v6 for the analysistest fixtures.
//
// The signatures are those of github.com/gobuffalo/pop/v6 v6.1.1. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package pop

import (
	"context"
	"database/sql"
)

// Tx is the transaction a Connection holds.
type Tx struct{}

func (tx *Tx) Commit() error { return nil }

func (tx *Tx) Rollback() error { return nil }

// Connection is a handle, or a transaction when TX is set.
type Connection struct {
	TX *Tx
}

func (c *Connection) Create(model interface{}, excludeColumns ...string) error { return nil }

func (c *Connection) Transaction(fn func(tx *Connection) error) error { return nil }

func (c *Connection) Rollback(fn func(tx *Connection)) error { return nil }

func (c *Connection) NewTransaction() (*Connection, error) { return c, nil }

func (c *Connection) NewTransactionContext(ctx context.Context) (*Connection, error) { return c, nil }

func (c *Connection) NewTransactionContextOptions(ctx context.Context, options *sql.TxOptions) (*Connection, error) {
	return c, nil
}
