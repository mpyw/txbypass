// Package trmtx covers outer-context with avito-tech's go-transaction-manager.
package trmtx

import (
	"context"

	"github.com/gin-gonic/gin"

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

// saveAs is generic over the context it saves with.
func saveAs[C context.Context](ctx C) error { return nil }

// logAll takes anything.
func logAll(args ...any) {}

// Ctx is a framework's context interface, as fiber v3's fiber.Ctx: it has
// more methods than context.Context.
type Ctx interface {
	context.Context
	Locals(key any) any
}

// userID takes the framework's interface, which the inner context does not
// fit.
func userID(c Ctx) any { return c.Locals("user") }

// audit keeps the gin context in a field.
type audit struct{ c *gin.Context }

// render takes the gin context as itself, not as a context.
func render(c *gin.Context, id string) { c.JSON(200, gin.H{"id": id}) }

// A gin handler gives its *gin.Context to the opener. Its own methods and
// fields run no statement, and the inner context has none of them, so they
// are no use of the outer context. Using it as a context is.
func Handler(c *gin.Context, m *manager.Manager) error {
	return m.Do(c, func(ctx context.Context) error {
		id := c.Param("id")
		_ = c.Request
		if err := save(c); err != nil { // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
			return err
		}
		_ = c.Value("user")           // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		_ = save(c.Request.Context()) // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		_ = len(c.Copy().Param("id"))
		c.JSON(201, gin.H{"id": id})
		render(c, id)
		_ = audit{c: c}
		_ = []*gin.Context{c}
		_ = audit{c}
		_ = map[string]*gin.Context{"c": c}
		var kept *gin.Context = c
		kept = c
		var same = c
		_, _ = kept, same
		_ = map[*gin.Context]int{c: 1} // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		_ = c == nil                   // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		ch := make(chan *gin.Context, 1)
		ch <- c
		_ = saveAs(c)                // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		_ = any(c)                   // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		logAll(c)                    // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		_ = save(context.Context(c)) // want `^c bypasses the transaction opened with Do; use ctx instead \[outer-context\]$`
		return nil
	})
}

// Not reported: the framework's interface takes the context as itself.
func FrameworkInterface(c Ctx, m *manager.Manager) error {
	return m.Do(c, func(ctx context.Context) error {
		_ = userID(c)
		return save(ctx)
	})
}

// Literals and channels of a type parameter with a core type.
func GenericSlots[S ~[]context.Context, M ~map[string]context.Context, C ~chan context.Context](ctx context.Context, m *manager.Manager, ch C) error {
	return m.Do(ctx, func(txCtx context.Context) error {
		_ = S{ctx}      // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		_ = M{"a": ctx} // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		ch <- ctx       // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		return save(txCtx)
	})
}

// job holds a context in its first field.
type job struct {
	ctx context.Context
	n   int
}

// A send on a type parameter whose channels differ in direction, and an
// element literal with its & left out.
func MixedSlots[C interface {
	chan *gin.Context | chan<- *gin.Context
}](c *gin.Context, m *manager.Manager, ch C) error {
	return m.Do(c, func(txCtx context.Context) error {
		ch <- c
		_ = []*job{{c, 2}} // want `^c bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		return save(txCtx)
	})
}

// saver saves with a context, through a method expression too.
type saver struct{}

func (saver) Save(ctx context.Context) error { return nil }

// The outer context given through a method expression, whose receiver is
// its first argument.
func MethodExpression(ctx context.Context, m *manager.Manager, r saver) error {
	return m.Do(ctx, func(txCtx context.Context) error {
		_ = saver.Save(r, ctx)      // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		<-context.Context.Done(ctx) // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		return save(txCtx)
	})
}

// getter has a generic method that takes a context last.
type getter struct{}

func (getter) Get[T any](id int, ctx context.Context) (T, error) {
	var zero T
	return zero, nil
}

func (getter) Pair[K, V any](ctx context.Context) {}

// The outer context given through a generic method expression.
func GenericMethodExpression(ctx context.Context, m *manager.Manager, g getter) error {
	return m.Do(ctx, func(txCtx context.Context) error {
		_, _ = getter.Get[string](g, 1, ctx) // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		getter.Pair[int, string](g, ctx)     // want `^ctx bypasses the transaction opened with Do; use txCtx instead \[outer-context\]$`
		return save(txCtx)
	})
}

// Not reported: a nested Do given the outer callback's context joins its
// transaction, so a statement with that context runs inside it.
func NestedDo(ctx context.Context, m *manager.Manager) error {
	return m.Do(ctx, func(outer context.Context) error {
		return m.Do(outer, func(inner context.Context) error {
			return save(outer)
		})
	})
}
