// Package trmtx covers outer-context with avito-tech's go-transaction-manager.
package trmtx

import (
	"context"

	trm "github.com/avito-tech/go-transaction-manager/trm/v2"
	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
)

func save(ctx context.Context) error { return nil }

func Do(ctx context.Context, m *manager.Manager) error {
	return m.Do(ctx, func(txCtx context.Context) error {
		return save(ctx) // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
	})
}

func DoWithSettings(ctx context.Context, m *manager.Manager, s trm.Settings) error {
	return m.DoWithSettings(ctx, s, func(txCtx context.Context) error {
		return save(ctx) // want `^ctx bypasses the transaction opened with DoWithSettings; use txCtx instead \[outer-context\]$`
	})
}

func Interface(ctx context.Context, m trm.Manager, s trm.Settings) error {
	if err := m.Do(ctx, func(txCtx context.Context) error {
		return save(ctx) // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
	}); err != nil {
		return err
	}
	return m.DoWithSettings(ctx, s, func(txCtx context.Context) error {
		return save(ctx) // want `^ctx bypasses the transaction opened with DoWithSettings; use txCtx instead \[outer-context\]$`
	})
}

// A deeper context made from the outer one is another variable.
type Service struct{ ctx context.Context }

func (s *Service) Run(m *manager.Manager) error {
	return m.Do(s.ctx, func(txCtx context.Context) error {
		return save(s.ctx) // want `^s\.ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
	})
}
