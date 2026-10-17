// Package rel is a stub of github.com/go-rel/rel for the analysistest fixtures.
//
// The signatures are those of github.com/go-rel/rel v0.42.0. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package rel

import (
	"context"
)

// Repository is a handle.
type Repository interface {
	Insert(ctx context.Context, record interface{}) error
	Transaction(ctx context.Context, fn func(ctx context.Context) error) error
}
