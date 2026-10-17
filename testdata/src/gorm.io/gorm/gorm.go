// Package gorm is a stub of gorm.io/gorm for the analysistest fixtures.
//
// The signatures are those of gorm.io/gorm v1.31.2. Only what the fixtures
// use is kept, and the bodies do nothing.
//
// Copyright (c) 2013-present Jinzhu <wosmvp@gmail.com>. MIT License:
// https://github.com/go-gorm/gorm/blob/v1.31.2/LICENSE
package gorm

import (
	"context"
	"database/sql"
)

// DB is a handle, a session, or a transaction.
type DB struct {
	Error error
}

// Session is the configuration of a new session.
type Session struct {
	DryRun bool
}

func (db *DB) WithContext(ctx context.Context) *DB { return db }

func (db *DB) Session(config *Session) *DB { return db }

func (db *DB) Debug() (tx *DB) { return db }

func (db *DB) Create(value interface{}) (tx *DB) { return db }

func (db *DB) Save(value interface{}) (tx *DB) { return db }

func (db *DB) First(dest interface{}, conds ...interface{}) (tx *DB) { return db }

func (db *DB) Where(query interface{}, args ...interface{}) (tx *DB) { return db }

func (db *DB) Exec(sql string, values ...interface{}) (tx *DB) { return db }

func (db *DB) Transaction(fc func(tx *DB) error, opts ...*sql.TxOptions) (err error) {
	return fc(db)
}

func (db *DB) Begin(opts ...*sql.TxOptions) *DB { return db }

func (db *DB) Commit() *DB { return db }

func (db *DB) Rollback() *DB { return db }

// DB returns the underlying *sql.DB. It is not a method that returns its
// receiver's type.
func (db *DB) DB() (*sql.DB, error) { return nil, nil }
