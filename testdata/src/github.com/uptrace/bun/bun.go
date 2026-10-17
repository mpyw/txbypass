// Package bun is a stub of github.com/uptrace/bun for the analysistest
// fixtures.
//
// The signatures are those of github.com/uptrace/bun v1.3.0. Only what the
// fixtures use is kept, and the bodies do nothing.
//
// Copyright (c) 2021 Vladimir Mihailenco. BSD 2-Clause License:
// https://github.com/uptrace/bun/blob/v1.3.0/LICENSE
package bun

import (
	"context"
	"database/sql"
)

// IDB is the interface DB, Conn and Tx share.
type IDB interface {
	NewInsert() *InsertQuery
	RunInTx(ctx context.Context, opts *sql.TxOptions, f func(ctx context.Context, tx Tx) error) error
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)
}

// DB is a pool of connections.
type DB struct {
	*sql.DB
}

// Conn is one connection.
type Conn struct {
	*sql.Conn
}

// Tx is a transaction, or a savepoint inside one.
type Tx struct {
	*sql.Tx
}

// InsertQuery is an INSERT statement.
type InsertQuery struct{}

func (q *InsertQuery) Model(model interface{}) *InsertQuery { return q }

func (q *InsertQuery) Exec(ctx context.Context, dest ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (db *DB) NewInsert() *InsertQuery { return &InsertQuery{} }

func (c Conn) NewInsert() *InsertQuery { return &InsertQuery{} }

func (tx Tx) NewInsert() *InsertQuery { return &InsertQuery{} }

func (db *DB) RunInTx(
	ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context, tx Tx) error,
) error {
	return fn(ctx, Tx{})
}

func (c Conn) RunInTx(
	ctx context.Context, opts *sql.TxOptions, fn func(ctx context.Context, tx Tx) error,
) error {
	return fn(ctx, Tx{})
}

func (tx Tx) RunInTx(
	ctx context.Context, _ *sql.TxOptions, fn func(ctx context.Context, tx Tx) error,
) error {
	return fn(ctx, tx)
}

func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) { return Tx{}, nil }

func (c Conn) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) { return Tx{}, nil }

func (tx Tx) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) { return tx, nil }

func (db *DB) Begin() (Tx, error) { return db.BeginTx(context.Background(), nil) }

func (tx Tx) Begin() (Tx, error) { return tx.BeginTx(context.Background(), nil) }
