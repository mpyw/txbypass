// Package gcptx covers outer-handle with Cloud Firestore, Spanner and
// Datastore.
package gcptx

import (
	"context"

	"cloud.google.com/go/datastore"
	"cloud.google.com/go/firestore"
	"cloud.google.com/go/spanner"
)

func Firestore(ctx context.Context, client *firestore.Client) error {
	return client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		return client.Doc("users/a").Set(ctx, map[string]any{"n": 1}) // want `^client bypasses the transaction opened with RunTransaction; use tx instead \[outer-handle\]$`
	})
}

func Spanner(ctx context.Context, client *spanner.Client) error {
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		_, err := client.Apply(ctx, nil) // want `^client bypasses the transaction opened with ReadWriteTransaction; use txn instead \[outer-handle\]$`
		return err
	})
	if err != nil {
		return err
	}
	_, err = client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		_, err := client.Apply(ctx, nil) // want `^client bypasses the transaction opened with ReadWriteTransactionWithOptions; use txn instead \[outer-handle\]$`
		return err
	}, spanner.TransactionOptions{})
	return err
}

func Datastore(ctx context.Context, client *datastore.Client, k *datastore.Key) error {
	_, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		_, err := client.Put(ctx, k, nil) // want `^client bypasses the transaction opened with RunInTransaction; use tx instead \[outer-handle\]$`
		return err
	})
	if err != nil {
		return err
	}
	tx, err := client.NewTransaction(ctx)
	if err != nil {
		return err
	}
	client.Put(ctx, k, nil) // want `^client bypasses the transaction opened with NewTransaction; use tx instead \[outer-handle\]$`
	_, err = tx.Commit()
	return err
}

// A reference the client builds for the transaction runs nothing.
func References(ctx context.Context, client *firestore.Client) error {
	return client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if _, err := tx.Get(client.Doc("users/a")); err != nil {
			return err
		}
		ref := client.Collection("users").Doc("b")
		if err := tx.Set(ref, map[string]any{"n": 1}); err != nil {
			return err
		}
		if err := client.Collection("users").Doc("c").Set(ctx, nil); err != nil { // want `^client bypasses the transaction opened with RunTransaction; use tx instead \[outer-handle\]$`
			return err
		}
		return save(ctx, client) // want `^client bypasses the transaction opened with RunTransaction; use tx instead \[outer-handle\]$`
	})
}

func save(ctx context.Context, client *firestore.Client) error { return nil }
