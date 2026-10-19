package txcall

import (
	"go/types"
	"slices"
)

// connTypes are the types of one connection, rather than a pool of them,
// whose begin or invoker the tables cannot tell apart by the receiver. pgx v5's
// BeginFunc takes any value that begins. go-pg's Conn begins through the
// *baseDB it embeds, as DB does. A transaction of pgx or bun opens a
// savepoint, on the connection the transaction holds. The begins of a
// transaction stay in the table of begins, so that the savepoint is a
// handle of its own: its commit ends nothing of the outer transaction.
var connTypes = []known{
	{pkg: "github.com/jackc/pgx/v5", name: "Conn"},
	{pkg: "github.com/jackc/pgx/v5/pgxpool", name: "Conn"},
	{pkg: "github.com/go-pg/pg/v10", name: "Conn"},
	{pkg: "github.com/jackc/pgx/v5", name: "Tx"},
	{pkg: "github.com/jackc/pgx/v4", name: "Tx"},
	{pkg: "github.com/uptrace/bun", name: "Tx"},
}

// onConn reports whether t is one connection, rather than a pool, or a
// transaction that holds one. A method declared on such a type is read as
// one too, when it is promoted to a type that embeds it, as pgxmock's mock
// embeds pgx.Tx. A transaction begun on it runs on that connection, so a
// statement on the connection runs inside the transaction: it is no
// bypass. Begins on a connection are not listed at all.
//
//declscope:shared // invoker.go, begin.go and shape.go tell one connection from a pool
func onConn(t types.Type) bool {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil {
		return false
	}
	return slices.ContainsFunc(connTypes, func(k known) bool {
		return k.pkg == n.Obj().Pkg().Path() && k.name == n.Obj().Name()
	})
}

// beginsOnConn reports whether the Begin method of t, which pgx v5's
// BeginFunc calls, is declared on the type of one connection, as it is for
// a type that embeds pgx.Tx or *pgx.Conn.
//
//declscope:shared // invoker.go and shape.go tell a begin on one connection apart
func beginsOnConn(t types.Type) bool {
	m, _, _ := types.LookupFieldOrMethod(t, true, nil, "Begin")
	fn, ok := m.(*types.Func)
	return ok && onConn(fn.Signature().Recv().Type())
}
