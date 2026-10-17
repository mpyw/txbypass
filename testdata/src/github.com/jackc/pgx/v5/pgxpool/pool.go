// Package pgxpool is a stub of github.com/jackc/pgx/v5/pgxpool for the
// analysistest fixtures.
//
// The signatures are those of github.com/jackc/pgx/v5 v5.11.0. Only what the
// fixtures use is kept, and the bodies do nothing.
//
// Copyright (c) 2013-2021 Jack Christensen. MIT License:
// https://github.com/jackc/pgx/blob/v5.11.0/LICENSE
package pgxpool

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Pool is a pool of connections.
type Pool struct{}

func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }

func (p *Pool) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	return nil, nil
}

func (p *Pool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
