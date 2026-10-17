// Package twoback holds a client that is not shaped like ent's.
package twoback

import "context"

type Client struct{}

type Tx struct{}

func (c *Client) Tx(ctx context.Context) (*Tx, error) { return &Tx{}, nil }

// Client gives two results.
func (tx *Tx) Client() (*Client, error) { return nil, nil }
