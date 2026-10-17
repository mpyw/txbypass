// Package firestore is a stub of cloud.google.com/go/firestore for the analysistest fixtures.
//
// The signatures are those of cloud.google.com/go/firestore v1.18.0. Only what the fixtures use is
// kept, and the bodies do nothing. Apache License 2.0.
package firestore

import (
	"context"
)

// Client is a client.
type Client struct{}

// DocumentRef is a document.
type DocumentRef struct{}

// Transaction is a transaction.
type Transaction struct{}

// TransactionOption is an option.
type TransactionOption interface{}

// CollectionRef is a collection.
type CollectionRef struct{}

// DocumentSnapshot is a read document.
type DocumentSnapshot struct{}

func (c *Client) Doc(path string) *DocumentRef { return &DocumentRef{} }

func (c *Client) Collection(path string) *CollectionRef { return &CollectionRef{} }

func (c *CollectionRef) Doc(id string) *DocumentRef { return &DocumentRef{} }

func (t *Transaction) Get(dr *DocumentRef) (*DocumentSnapshot, error) { return nil, nil }

func (d *DocumentRef) Set(ctx context.Context, data interface{}) error { return nil }

func (c *Client) RunTransaction(ctx context.Context, f func(context.Context, *Transaction) error, opts ...TransactionOption) (err error) {
	return nil
}

func (t *Transaction) Set(dr *DocumentRef, data interface{}) error { return nil }
