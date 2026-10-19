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

// Session is a transaction with a name txbypass does not know.
type Session struct{}

// Exec runs a statement on the session.
func (*Session) Exec(ctx context.Context, query string) error { return nil }

// End ends the session.
//
//txbypass:closes
func (*Session) End() error { return nil } // want End:`^closes\[0\]$`

// Pool begins sessions.
type Pool struct{}

// Exec runs a statement on the pool.
func (*Pool) Exec(ctx context.Context, query string) error { return nil }

// Open begins a session.
//
//txbypass:begins
func (*Pool) Open(ctx context.Context) (*Session, error) { return nil, nil } // want Open:`^begins\[p0\]$`

// EndSession ends s through its declared end, so it ends s too.
func EndSession(s *Session) error { return s.End() } // want EndSession:`^closes\[0\]$`
