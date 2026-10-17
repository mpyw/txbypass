// Package noback holds a client that is not shaped like ent's.
package noback

import "context"

type Client struct{}

type Tx struct{}

// Tx has no Client method.
func (c *Client) Tx(ctx context.Context) (*Tx, error) { return &Tx{}, nil }
