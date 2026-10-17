// Package pgx is a stub of github.com/jackc/pgx/v4 for the analysistest
// fixtures.
//
// The signatures are those of github.com/jackc/pgx/v4 v4.18.3. Only what the
// fixtures use is kept, and the bodies do nothing.
//
// Copyright (c) 2013-2021 Jack Christensen. MIT License:
// https://github.com/jackc/pgx/blob/v4.18.3/LICENSE
package pgx

import "context"

// CommandTag is the result of a statement.
type CommandTag []byte

// Tx is a transaction.
type Tx interface {
	Begin(ctx context.Context) (Tx, error)
	BeginFunc(ctx context.Context, f func(Tx) error) (err error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (commandTag CommandTag, err error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// TxOptions are the options of a transaction.
type TxOptions struct {
	IsoLevel string
}

// Conn is one connection.
type Conn struct{}

func (c *Conn) Begin(ctx context.Context) (Tx, error) { return nil, nil }

func (c *Conn) BeginTx(ctx context.Context, txOptions TxOptions) (Tx, error) { return nil, nil }

func (c *Conn) BeginFunc(ctx context.Context, f func(Tx) error) (err error) { return nil }

func (c *Conn) BeginTxFunc(ctx context.Context, txOptions TxOptions, f func(Tx) error) (err error) {
	return nil
}

func (c *Conn) Exec(ctx context.Context, sql string, arguments ...interface{}) (CommandTag, error) {
	return nil, nil
}
