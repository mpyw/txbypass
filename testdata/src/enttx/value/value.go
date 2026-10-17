// Package value holds a client that is not shaped like ent's.
package value

import "context"

type Client struct{}

type Tx struct{}

// Tx returns a value, not a pointer.
func (c *Client) Tx(ctx context.Context) (Tx, error) { return Tx{}, nil }
