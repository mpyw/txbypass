// Package spanner is a stub of cloud.google.com/go/spanner for the analysistest fixtures.
//
// The signatures are those of cloud.google.com/go/spanner v1.73.0. Only what the fixtures use is
// kept, and the bodies do nothing. Apache License 2.0.
package spanner

import (
	"context"
	"time"
)

// Client is a client.
type Client struct{}

// Mutation is a write.
type Mutation struct{}

// ReadWriteTransaction is a transaction.
type ReadWriteTransaction struct{}

// TransactionOptions are options.
type TransactionOptions struct{}

// CommitResponse is the result of a commit.
type CommitResponse struct{}

func (c *Client) Apply(ctx context.Context, ms []*Mutation) (time.Time, error) {
	return time.Time{}, nil
}

func (c *Client) ReadWriteTransaction(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error) (commitTimestamp time.Time, err error) {
	return time.Time{}, nil
}

func (c *Client) ReadWriteTransactionWithOptions(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error, options TransactionOptions) (resp CommitResponse, err error) {
	return CommitResponse{}, nil
}

func (t *ReadWriteTransaction) BufferWrite(ms []*Mutation) error { return nil }
