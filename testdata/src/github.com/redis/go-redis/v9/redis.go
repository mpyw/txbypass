// Package redis is a stub of github.com/redis/go-redis/v9 for the analysistest fixtures.
//
// The signatures are those of github.com/redis/go-redis/v9 v9.7.0. Only what the fixtures use is
// kept, and the bodies do nothing. BSD 2-Clause License.
package redis

import (
	"context"
	"time"
)

// StatusCmd is the result of a command.
type StatusCmd struct{}

// Cmder is any command.
type Cmder interface{}

// Cmdable is what every client has.
type Cmdable interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *StatusCmd
	TxPipelined(ctx context.Context, fn func(Pipeliner) error) ([]Cmder, error)
}

// Pipeliner queues commands.
type Pipeliner interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *StatusCmd
}

// UniversalClient is any client.
type UniversalClient interface {
	Cmdable
	Watch(ctx context.Context, fn func(*Tx) error, keys ...string) error
}

// Client is a client.
type Client struct{}

// Tx is the connection that Watch holds.
type Tx struct{}

func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *StatusCmd {
	return nil
}

func (c *Client) Watch(ctx context.Context, fn func(*Tx) error, keys ...string) error { return nil }

func (c *Client) TxPipelined(ctx context.Context, fn func(Pipeliner) error) ([]Cmder, error) {
	return nil, nil
}

func (c *Tx) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *StatusCmd {
	return nil
}

func (c *Tx) TxPipelined(ctx context.Context, fn func(Pipeliner) error) ([]Cmder, error) {
	return nil, nil
}
