package sqltx

import (
	"context"
	"database/sql"
)

type shadowInner struct{ db *sql.DB }

// shadowOuter has a db of its own, which shadows the embedded one.
type shadowOuter struct {
	shadowInner
	db *sql.DB
}

// The handle is the embedded field's db, so the message keeps the embedded
// field: o.db is another field.
func (o *shadowOuter) Shadowed(ctx context.Context) error {
	tx, err := o.shadowInner.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _ = o.shadowInner.db.ExecContext(ctx, "audit") // want `^o\.shadowInner\.db bypasses the transaction opened with BeginTx; use tx instead \[outer-handle\]$`
	_, _ = o.db.ExecContext(ctx, "other")
	return tx.Commit()
}
