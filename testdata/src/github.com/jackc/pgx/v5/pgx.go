// Package pgx is a stub of github.com/jackc/pgx/v5 for the analysistest
// fixtures.
//
// The signatures are those of github.com/jackc/pgx/v5 v5.11.0. Only what the
// fixtures use is kept, and the bodies do nothing.
//
// Copyright (c) 2013-2021 Jack Christensen. MIT License:
// https://github.com/jackc/pgx/blob/v5.11.0/LICENSE
package pgx

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

// Tx is a transaction.
type Tx interface {
	Begin(ctx context.Context) (Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (commandTag pgconn.CommandTag, err error)
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

func (c *Conn) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func BeginFunc(
	ctx context.Context,
	db interface {
		Begin(ctx context.Context) (Tx, error)
	},
	fn func(Tx) error,
) (err error) {
	return nil
}

func BeginTxFunc(
	ctx context.Context,
	db interface {
		BeginTx(ctx context.Context, txOptions TxOptions) (Tx, error)
	},
	txOptions TxOptions,
	fn func(Tx) error,
) (err error) {
	return nil
}
