// Package declaredlib declares helpers that another package calls.
package declaredlib

import (
	"context"
	"database/sql"
)

// Querier runs statements, on a pool or on a transaction.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Store is given by dependency injection, so no body of it is read.
type Store interface {
	Querier
	//txbypass:invokes // runs fn in a transaction
	WithTx(ctx context.Context, fn func(q Querier) error) error // want WithTx:`^invokes\[p0, p2\(0\)\]$`
}
