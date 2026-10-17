// Package uppertx covers outer-handle with upper/db.
package uppertx

import (
	"context"

	db "github.com/upper/db/v4"
)

type User struct{ Name string }

func Tx(sess db.Session, u *User) error {
	return sess.Tx(func(tx db.Session) error {
		_, err := sess.Collection("users").Insert(u) // want `^sess bypasses the transaction opened with Tx; use tx instead \[outer-handle\]$`
		return err
	})
}

func TxContext(ctx context.Context, sess db.Session, u *User) error {
	return sess.TxContext(ctx, func(tx db.Session) error {
		_, err := sess.Collection("users").Insert(u) // want `^sess bypasses the transaction opened with TxContext; use tx instead \[outer-handle\]$`
		return err
	}, nil)
}
