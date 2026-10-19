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

// Using binds the pool to s, as a repository's Using(tx) does: a session
// is a transaction, by its declared end.
func (p *Pool) Using(s *Session) *Pool { return p }

// EndSession ends s through its declared end, so it ends s too.
func EndSession(s *Session) error { return s.End() } // want EndSession:`^closes\[0\]$`

// Conn begins a transaction that is a Conn too, as gorm's DB does.
type Conn struct{}

// Conn gives a connection of the pool.
func (*Pool) Conn() *Conn { return nil }

// Exec runs a statement on the connection.
func (*Conn) Exec(ctx context.Context, query string) error { return nil }

// Begin begins a transaction on the connection.
//
//txbypass:begins
func (c *Conn) Begin() *Conn { return c } // want Begin:`^begins\[p0\]$`

// End ends the transaction.
//
//txbypass:closes
func (*Conn) End() error { return nil } // want End:`^closes\[0\]$`

// Log is given to an end beside the session. It is no transaction.
type Log struct{}

// Book ends a session, and logs to l.
type Book struct{}

//txbypass:closes
func (*Book) End(s *Session, l *Log) error { return nil } // want End:`^closes\[0 1 2\]$`
