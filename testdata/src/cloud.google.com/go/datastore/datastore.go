// Package datastore is a stub of cloud.google.com/go/datastore for the analysistest fixtures.
//
// The signatures are those of cloud.google.com/go/datastore v1.20.0. Only what the fixtures use is
// kept, and the bodies do nothing. Apache License 2.0.
package datastore

import (
	"context"
)

// Client is a client.
type Client struct{}

// Key is a key.
type Key struct{}

// Transaction is a transaction.
type Transaction struct{}

// Commit is the result of a commit.
type Commit struct{}

// TransactionOption is an option.
type TransactionOption interface{}

func (c *Client) Put(ctx context.Context, key *Key, src interface{}) (*Key, error) { return key, nil }

func (c *Client) NewTransaction(ctx context.Context, opts ...TransactionOption) (t *Transaction, err error) {
	return &Transaction{}, nil
}

func (c *Client) RunInTransaction(ctx context.Context, f func(tx *Transaction) error, opts ...TransactionOption) (cmt *Commit, err error) {
	return nil, nil
}

func (t *Transaction) Put(key *Key, src interface{}) (interface{}, error) { return nil, nil }

func (t *Transaction) Commit() (c *Commit, err error) { return nil, nil }

func (t *Transaction) Rollback() (err error) { return nil }
