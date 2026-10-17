// Package trm is a stub of github.com/avito-tech/go-transaction-manager/trm/v2 for the analysistest fixtures.
//
// The signatures are those of github.com/avito-tech/go-transaction-manager/trm/v2 v2.0.0. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package trm

import (
	"context"
)

// Settings are the settings of a transaction.
type Settings interface{}

// Manager runs a closure in a transaction.
type Manager interface {
	Do(context.Context, func(ctx context.Context) error) error
	DoWithSettings(context.Context, Settings, func(ctx context.Context) error) error
}
