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

// Not reported: a transaction opened on the callback's transaction runs on
// its connection. The opener is an interface method here.
func Nested(sess db.Session, u *User) error {
	return sess.Tx(func(tx db.Session) error {
		return tx.Tx(func(inner db.Session) error {
			// Not reported: it runs on the transaction's connection.
			_, err := tx.Collection("users").Insert(u)
			return err
		})
	})
}
