package sqltx

import (
	"context"
	"database/sql"
)

// DBTX is what sqlc's generated Queries runs on.
type DBTX interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

// Queries is what sqlc generates.
type Queries struct {
	db DBTX
}

func New(db DBTX) *Queries { return &Queries{db: db} }

func (q *Queries) WithTx(tx *sql.Tx) *Queries { return &Queries{db: tx} }

func (q *Queries) CreateUser(ctx context.Context, name string) error { return nil }

// Other has a WithTx that gives another type.
type Other struct{}

func (Other) WithTx(tx *sql.Tx) *Queries { return nil }

// ===== Reported =====

// Code that rebinds q to the transaction means every other q to be the
// transaction too.
func CreateTwo(ctx context.Context, db *sql.DB, q *Queries) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := q.WithTx(tx)
	if err := qtx.CreateUser(ctx, "a"); err != nil {
		return err
	}
	if err := q.CreateUser(ctx, "b"); err != nil { // want `^q bypasses the transaction opened with BeginTx; use qtx instead \[outer-handle\]$`
		return err
	}
	return tx.Commit()
}

// Without a variable, the call itself is what to use.
func Inline(ctx context.Context, db *sql.DB, q *Queries) error {
	tx, _ := db.BeginTx(ctx, nil)
	q.WithTx(tx).CreateUser(ctx, "a")
	q.CreateUser(ctx, "b") // want `^q bypasses the transaction opened with BeginTx; use q\.WithTx\(tx\) instead \[outer-handle\]$`
	return tx.Commit()
}

// An assignment of two values names no variable for the rebound one.
func Pair(ctx context.Context, db *sql.DB, q *Queries) error {
	tx, _ := db.BeginTx(ctx, nil)
	qtx, n := q.WithTx(tx), 1
	_ = q.WithTx(tx)
	q.CreateUser(ctx, "b") // want `^q bypasses the transaction opened with BeginTx; use q\.WithTx\(tx\) instead \[outer-handle\]$`
	use(qtx, n)
	return tx.Commit()
}

// ===== Not reported =====

// A WithTx of another transaction rebinds nothing here.
func OtherTx(ctx context.Context, db *sql.DB, q *Queries, other *sql.Tx) error {
	tx, _ := db.BeginTx(ctx, nil)
	q.WithTx(other).CreateUser(ctx, "a")
	q.CreateUser(ctx, "b")
	return tx.Commit()
}

// A WithTx that gives another type, or is called on no handle, rebinds
// nothing.
func OtherType(ctx context.Context, db *sql.DB) error {
	tx, _ := db.BeginTx(ctx, nil)
	var o Other
	_ = o.WithTx(tx)
	_ = Other{}.WithTx(tx)
	use(o)
	return tx.Commit()
}

// Querier is an interface with a WithTx: what it holds is not known.
type Querier interface {
	WithTx(tx *sql.Tx) Querier
	CreateUser(ctx context.Context, name string) error
}

// Not reported: an interface's WithTx says nothing of what the value holds.
func InterfaceWithTx(ctx context.Context, db *sql.DB, q Querier) error {
	tx, _ := db.BeginTx(ctx, nil)
	q.WithTx(tx).CreateUser(ctx, "a")
	q.CreateUser(ctx, "b")
	return tx.Commit()
}
