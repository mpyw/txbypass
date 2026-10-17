// Package pgxpool is a stub of github.com/jackc/pgx/v4/pgxpool for the
// analysistest fixtures.
//
// The signatures are those of github.com/jackc/pgx/v4 v4.18.3. Only what the
// fixtures use is kept, and the bodies do nothing.
//
// Copyright (c) 2013-2021 Jack Christensen. MIT License:
// https://github.com/jackc/pgx/blob/v4.18.3/LICENSE
package pgxpool

import (
	"context"

	"github.com/jackc/pgx/v4"
)

// Pool is a pool of connections.
type Pool struct{}

func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }

func (p *Pool) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) { return nil, nil }

func (p *Pool) BeginFunc(ctx context.Context, f func(pgx.Tx) error) error { return nil }

func (p *Pool) BeginTxFunc(ctx context.Context, txOptions pgx.TxOptions, f func(pgx.Tx) error) error {
	return nil
}

func (p *Pool) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgx.CommandTag, error) {
	return nil, nil
}

// Conn is a connection acquired from a Pool.
type Conn struct{}

func (c *Conn) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }

func (c *Conn) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) { return nil, nil }
