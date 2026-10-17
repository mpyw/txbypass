package gcptx

import (
	"context"

	"cloud.google.com/go/datastore"
)

type Task struct{ Done bool }

// Not reported: the query carries the transaction, so the client runs it
// inside the transaction.
func QueryInTransaction(ctx context.Context, client *datastore.Client, list *datastore.Key) error {
	_, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var tasks []Task
		q := datastore.NewQuery("Task").Ancestor(list).Transaction(tx)
		if _, err := client.GetAll(ctx, q, &tasks); err != nil {
			return err
		}
		_ = client.Run(ctx, datastore.NewQuery("Task").Transaction(tx))
		return nil
	})
	return err
}

// A query not bound to the transaction runs outside it.
func QueryOutside(ctx context.Context, client *datastore.Client) error {
	_, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var tasks []Task
		_, err := client.GetAll(ctx, datastore.NewQuery("Task"), &tasks) // want `^client bypasses the transaction opened with RunInTransaction; use tx instead \[outer-handle\]$`
		return err
	})
	return err
}

// A query kept in a variable written twice, which a literal reads, is
// followed through both writes. Neither binds it to the transaction.
func QueryRebuilt(ctx context.Context, client *datastore.Client, list *datastore.Key) error {
	_, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var tasks []Task
		q := datastore.NewQuery("Task")
		q = q.Ancestor(list)
		defer func() { _ = q }()
		_, err := client.GetAll(ctx, q, &tasks) // want `^client bypasses the transaction opened with RunInTransaction; use tx instead \[outer-handle\]$`
		return err
	})
	return err
}

// Not reported: the bound query is filtered on one branch, and either way
// it carries the transaction.
func QueryFilteredOnBranch(ctx context.Context, client *datastore.Client, list *datastore.Key, deep bool) error {
	_, err := client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var tasks []Task
		q := datastore.NewQuery("Task").Transaction(tx)
		if deep {
			q = q.Ancestor(list)
		}
		for range 3 {
			q = q.Ancestor(list)
		}
		_, err := client.GetAll(ctx, q, &tasks)
		return err
	})
	return err
}
