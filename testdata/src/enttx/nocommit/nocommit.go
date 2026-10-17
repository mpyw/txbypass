// Package nocommit holds a client whose Tx is not a transaction: it has a
// Client back-reference, but no Commit or Rollback.
package nocommit

import "context"

type Client struct{}

type Tx struct{ c *Client }

func (t *Tx) Client() *Client { return t.c }

func (c *Client) Tx(ctx context.Context) (*Tx, error) { return &Tx{c: c}, nil }

func (c *Client) Ping(ctx context.Context) error { return nil }
