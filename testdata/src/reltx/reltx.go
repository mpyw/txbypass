// Package reltx covers outer-context with go-rel.
package reltx

import (
	"context"

	"github.com/go-rel/rel"
)

type User struct{ Name string }

func Create(ctx context.Context, repo rel.Repository, u *User) error {
	return repo.Transaction(ctx, func(txCtx context.Context) error {
		if err := repo.Insert(txCtx, u); err != nil {
			return err
		}
		return repo.Insert(ctx, u) // want `^ctx bypasses the transaction opened with Transaction; use txCtx instead \[outer-context\]$`
	})
}

// Not reported: go-rel's Transaction given the outer callback's context
// opens a savepoint in its transaction, so that context runs inside it.
func Nested(ctx context.Context, repo rel.Repository, u *User) error {
	return repo.Transaction(ctx, func(outer context.Context) error {
		return repo.Transaction(outer, func(inner context.Context) error {
			return repo.Insert(outer, u)
		})
	})
}
