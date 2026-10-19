package sqltx

import (
	"context"
	"database/sql"

	"helperlib"
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

// Store embeds sqlc's Queries beside the pool, as sqlc's own transaction
// example does. WithTx is promoted from the embedded Queries.
type Store struct {
	*Queries
	db *sql.DB
}

func (s *Store) Transfer(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.WithTx(tx)
	if err := qtx.CreateUser(ctx, "a"); err != nil {
		return err
	}
	if err := s.CreateUser(ctx, "b"); err != nil { // want `^s bypasses the transaction opened with BeginTx; use qtx instead \[outer-handle\]$`
		return err
	}
	return tx.Commit()
}

// Not reported, and no crash: a function named WithTx rebinds nothing.
func FunctionWithTx(ctx context.Context, db *sql.DB, q *Queries) error {
	tx, _ := db.BeginTx(ctx, nil)
	_ = helperlib.WithTx(tx)
	q.CreateUser(ctx, "b")
	return tx.Commit()
}

// PreparedQueries is sqlc's Queries with emit_prepared_queries: a statement
// per query, which runs on the transaction after WithTx.
type PreparedQueries struct {
	db               DBTX
	tx               *sql.Tx
	createAuthorStmt *sql.Stmt
}

func (q *PreparedQueries) WithTx(tx *sql.Tx) *PreparedQueries {
	return &PreparedQueries{db: tx, tx: tx, createAuthorStmt: q.createAuthorStmt}
}

func (q *PreparedQueries) CreateAuthor(ctx context.Context, name string) error {
	_, err := q.db.ExecContext(ctx, "insert", name)
	return err
}

func PreparedWithTx(ctx context.Context, db *sql.DB, q *PreparedQueries) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := q.WithTx(tx)
	if err := qtx.CreateAuthor(ctx, "a"); err != nil {
		return err
	}
	if err := q.CreateAuthor(ctx, "b"); err != nil { // want `^q bypasses the transaction opened with Begin; use qtx instead \[outer-handle\]$`
		return err
	}
	return tx.Commit()
}

// NamedQueries holds a name besides its database, so WithTx rebinds
// nothing: a use of the name is no use of the transaction.
type NamedQueries struct {
	db   DBTX
	name string
}

func (q *NamedQueries) WithTx(tx *sql.Tx) *NamedQueries { return &NamedQueries{db: tx, name: q.name} }

func (q *NamedQueries) Name() string { return q.name }

func NamedWithTx(ctx context.Context, db *sql.DB, q *NamedQueries) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_ = q.WithTx(tx)
	_ = q.Name()
	return tx.Commit()
}

// Not reported: the queries built on the pool are rebound to the
// transaction before they run.
func NewWithTx(ctx context.Context, db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := New(db).WithTx(tx)
	if err := q.CreateUser(ctx, "a"); err != nil {
		return err
	}
	return tx.Commit()
}

// Not reported: the queries kept in q are rebound to the transaction
// before they run.
func NewThenWithTx(ctx context.Context, db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := New(db)
	qtx := q.WithTx(tx)
	if err := qtx.CreateUser(ctx, "a"); err != nil {
		return err
	}
	return tx.Commit()
}

// The queries kept in q run on the pool too, which is reported.
func NewThenBoth(ctx context.Context, db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := New(db) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	qtx := q.WithTx(tx)
	if err := qtx.CreateUser(ctx, "a"); err != nil {
		return err
	}
	if err := q.CreateUser(ctx, "b"); err != nil { // want `^q bypasses the transaction opened with Begin; use qtx instead \[outer-handle\]$`
		return err
	}
	return tx.Commit()
}

// Not reported: q lives in memory, as a deferred literal reads it, and is
// rebound through a load of it.
func NewKeptThenWithTx(ctx context.Context, db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := New(db)
	defer func() { _ = q }()
	if err := q.WithTx(tx).CreateUser(ctx, "a"); err != nil {
		return err
	}
	return tx.Commit()
}
