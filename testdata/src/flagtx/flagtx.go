// Package flagtx covers helpers declared by flags. It runs with
// -invokes=(flaglib.Manager).InTx, -begins=(*flaglib.Manager).Begin and
// -closes=(flaglib.Tx).Finish.
package flagtx

import (
	"context"
	"database/sql"

	"flaglib"
)

func Invoked(ctx context.Context, m *flaglib.Manager) error {
	return m.InTx(ctx, func(tx *sql.Tx) error {
		_, err := m.DB.ExecContext(ctx, "q") // want `^m bypasses the transaction opened with InTx; use tx instead \[outer-handle\]$`
		return err
	})
}

func Begun(ctx context.Context, m *flaglib.Manager) error {
	tx, err := m.Begin(ctx)
	if err != nil {
		return err
	}
	m.DB.ExecContext(ctx, "q") // want `^m bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	if err := tx.Finish(); err != nil {
		return err
	}
	_, err = m.DB.ExecContext(ctx, "after")
	return err
}
