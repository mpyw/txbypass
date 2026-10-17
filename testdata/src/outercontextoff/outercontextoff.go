// Package outercontextoff holds code that outer-context reports, for a run
// with the rule off.
package outercontextoff

import (
	"context"

	"github.com/avito-tech/go-transaction-manager/trm/v2/manager"
)

func save(ctx context.Context) error { return nil }

// Not reported: outer-context is off.
func Do(ctx context.Context, m *manager.Manager) error {
	return m.Do(ctx, func(txCtx context.Context) error {
		return save(ctx)
	})
}
