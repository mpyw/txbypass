// Package poptx covers outer-handle with pop.
package poptx

import (
	"context"

	"github.com/gobuffalo/pop/v6"
)

type User struct{ Name string }

func Callback(c *pop.Connection, u *User) error {
	return c.Transaction(func(tx *pop.Connection) error {
		return c.Create(u) // want `^c bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

func RollbackCallback(c *pop.Connection, u *User) error {
	return c.Rollback(func(tx *pop.Connection) {
		c.Create(u) // want `^c bypasses the transaction opened with Rollback; use tx instead \[outer-handle\]$`
	})
}

// The transaction is committed through its TX field.
func Region(ctx context.Context, c *pop.Connection, u *User) error {
	tx, err := c.NewTransaction()
	if err != nil {
		return err
	}
	c.Create(u) // want `^c bypasses the transaction opened with NewTransaction; use tx instead \[outer-handle\]$`
	if err := tx.TX.Commit(); err != nil {
		return err
	}
	c.Create(u)
	tx2, _ := c.NewTransactionContext(ctx)
	c.Create(u) // want `^c bypasses the transaction opened with NewTransactionContext; use tx2 instead \[outer-handle\]$`
	tx2.TX.Rollback()
	tx3, _ := c.NewTransactionContextOptions(ctx, nil)
	c.Create(u) // want `^c bypasses the transaction opened with NewTransactionContextOptions; use tx3 instead \[outer-handle\]$`
	return tx3.TX.Rollback()
}

// Not reported: a transaction opened on a begin's transaction runs on its
// connection.
func NestedRegion(c *pop.Connection, u *User) error {
	tx, err := c.NewTransaction()
	if err != nil {
		return err
	}
	inner, err := tx.NewTransaction()
	if err != nil {
		return err
	}
	// Not reported: it runs on the transaction's connection.
	tx.Create(u)
	if err := inner.TX.Commit(); err != nil {
		return err
	}
	return tx.TX.Commit()
}

// A handle looked up in a map is no transaction, and is reported.
func FromMap(conns map[string]*pop.Connection, u *User) error {
	c, ok := conns["main"]
	if !ok {
		return nil
	}
	return c.Transaction(func(tx *pop.Connection) error {
		return c.Create(u) // want `^c bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}
