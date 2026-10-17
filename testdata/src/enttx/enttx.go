// Package enttx covers outer-handle with a client that ent generated, read
// from its shape.
package enttx

import (
	"context"

	"enttx/ent"
	"enttx/noback"
	"enttx/nocommit"
	"enttx/twoback"
	"enttx/value"
)

func Create(ctx context.Context, client *ent.Client) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	tx.User.Create()
	client.User.Create() // want `^client bypasses the transaction opened with Tx; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// ===== Not reported =====

// Client has a Tx method of another shape.
type Client struct{ db *ent.Client }

type Tx struct{}

func (c *Client) Tx(ctx context.Context) (*Tx, error) { return &Tx{}, nil }

func (c *Client) BeginTx(ctx context.Context) *Tx { return &Tx{} }

func (tx *Tx) Commit() error { return nil }

func (tx *Tx) Client() *ent.Client { return nil }

type Pool struct{}

func (p *Pool) Tx(ctx context.Context) (*Pool, error) { return p, nil }

// Clients of other shapes, each in a package of its own.
func Other(ctx context.Context, c *Client, pool *Pool, v *value.Client, n *noback.Client, t *twoback.Client) error {
	tx, _ := c.Tx(ctx)
	c.db.User.Create()
	tx2 := c.BeginTx(ctx)
	pp, _ := pool.Tx(ctx)
	vt, _ := v.Tx(ctx)
	nt, _ := n.Tx(ctx)
	tt, _ := t.Tx(ctx)
	_, _, _, _, _ = tx2, pp, vt, nt, tt
	v.Tx(ctx)
	n.Tx(ctx)
	t.Tx(ctx)
	pool.Tx(ctx)
	return tx.Commit()
}

// Not reported: a Tx with no Commit or Rollback is no transaction.
func NotATransaction(ctx context.Context, c *nocommit.Client) error {
	tx, err := c.Tx(ctx)
	if err != nil {
		return err
	}
	_ = tx
	return c.Ping(ctx)
}
