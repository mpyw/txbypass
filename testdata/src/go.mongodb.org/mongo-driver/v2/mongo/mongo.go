// Package mongo is a stub of go.mongodb.org/mongo-driver/v2/mongo for the analysistest fixtures.
//
// The signatures are those of go.mongodb.org/mongo-driver/v2/mongo v2.2.0. Only what the fixtures use is
// kept, and the bodies do nothing. Apache License 2.0.
package mongo

import (
	"context"
)

// Collection is a collection.
type Collection struct{}

// Session is a session.
type Session struct{}

func (coll *Collection) InsertOne(ctx context.Context, document interface{}) (interface{}, error) {
	return nil, nil
}

func (s *Session) WithTransaction(ctx context.Context, fn func(ctx context.Context) (any, error)) (any, error) {
	return nil, nil
}

func WithSession(ctx context.Context, sess *Session, fn func(context.Context) error) error {
	return nil
}

// Client is a client. UseSessionWithOptions takes an
// *options.SessionOptionsBuilder, which the stub keeps in this package.
type Client struct{}

// SessionOptionsBuilder stands in for options.SessionOptionsBuilder.
type SessionOptionsBuilder struct{}

func (c *Client) UseSession(ctx context.Context, fn func(context.Context) error) error { return nil }

func (c *Client) UseSessionWithOptions(ctx context.Context, opts *SessionOptionsBuilder, fn func(context.Context) error) error {
	return nil
}
