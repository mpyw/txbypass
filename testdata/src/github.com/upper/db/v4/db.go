// Package db is a stub of github.com/upper/db/v4 for the analysistest fixtures.
//
// The signatures are those of github.com/upper/db/v4 v4.10.0. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package db

import (
	"context"
	"database/sql"
)

// Collection is a table.
type Collection interface {
	Insert(interface{}) (interface{}, error)
}

// Session is a handle, or a transaction.
type Session interface {
	Collection(name string) Collection
	Tx(fn func(sess Session) error) error
	TxContext(ctx context.Context, fn func(sess Session) error, opts *sql.TxOptions) error
}
