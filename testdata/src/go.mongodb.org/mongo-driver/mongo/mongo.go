// Package mongo is a stub of go.mongodb.org/mongo-driver/mongo for the analysistest fixtures.
//
// The signatures are those of go.mongodb.org/mongo-driver/mongo v1.17.10. Only what the fixtures use is
// kept, and the bodies do nothing. Apache License 2.0.
package mongo

import (
	"context"
)

// Collection is a collection.
type Collection struct{}

// SessionContext is a context with a session.
type SessionContext interface {
	context.Context
}

// Session is a session.
type Session interface {
	WithTransaction(ctx context.Context, fn func(ctx SessionContext) (interface{}, error)) (interface{}, error)
}

func (coll *Collection) InsertOne(ctx context.Context, document interface{}) (interface{}, error) {
	return nil, nil
}

func WithSession(ctx context.Context, sess Session, fn func(SessionContext) error) error { return nil }

// Client is a client. UseSessionWithOptions takes an
// *options.SessionOptions, which the stub keeps in this package.
type Client struct{}

// SessionOptions stands in for options.SessionOptions.
type SessionOptions struct{}

func (c *Client) UseSession(ctx context.Context, fn func(SessionContext) error) error { return nil }

func (c *Client) UseSessionWithOptions(ctx context.Context, opts *SessionOptions, fn func(SessionContext) error) error {
	return nil
}
