// Package manager is a stub of github.com/avito-tech/go-transaction-manager/trm/v2/manager for the analysistest fixtures.
//
// The signatures are those of github.com/avito-tech/go-transaction-manager/trm/v2/manager v2.0.0. Only what the fixtures use is
// kept, and the bodies do nothing. MIT License.
package manager

import (
	"context"
	"github.com/avito-tech/go-transaction-manager/trm/v2"
)

// Manager runs a closure in a transaction.
type Manager struct{}

func (m *Manager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) { return nil }

func (m *Manager) DoWithSettings(ctx context.Context, s trm.Settings, fn func(ctx context.Context) error) (err error) {
	return nil
}
