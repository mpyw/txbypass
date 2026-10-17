// Package opener knows the calls that run a callback inside a transaction:
// gorm's Transaction, bun's RunInTx, and pgx's BeginFunc and BeginTxFunc.
//
// Each is read from types, by its package path, its receiver and its name, so
// an alias or a wrapper type of the caller's does not hide it.
package opener

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/types/typeutil"
)

// Opener is one call that opens a transaction and runs a callback in it.
type Opener struct {
	// Name is how a message names the call: a method by its name, a
	// function by its package's name and its own, as in pgx.BeginFunc.
	Name string
	// Outer is the expression that gives the handle the transaction is
	// opened on. For a method, it is the selector of the call, so that a
	// method promoted from an embedded field names that field.
	Outer ast.Expr
	// Callback is the function literal that runs inside the transaction.
	Callback *ast.FuncLit
	// Tx is the name of the callback's parameter that holds the
	// transaction. It is empty when the parameter is unnamed or blank.
	Tx string
}

// known is one call that opens a transaction.
type known struct {
	pkg string
	// recv is the name of the receiver's type, with any pointer removed. It
	// is empty for a function.
	recv string
	name string
	// outer is the index of the argument that is the handle. It is -1 when
	// the handle is the receiver.
	outer int
	// callback is the index of the argument that is the callback.
	callback int
	// tx is the index of the callback's parameter that is the transaction.
	tx int
}

// knownOpeners lists every call this package recognizes. The signatures are
// those of gorm v1.31, bun v1.3 and pgx v5.
var knownOpeners = []known{
	// func (db *DB) Transaction(fc func(tx *DB) error, opts ...*sql.TxOptions) error
	{pkg: "gorm.io/gorm", recv: "DB", name: "Transaction", outer: -1, callback: 0, tx: 0},
	// func (db *DB) RunInTx(ctx, opts, fn func(ctx context.Context, tx Tx) error) error,
	// and the same method of Conn, of Tx (a savepoint), and of the IDB
	// interface that all three implement.
	{pkg: "github.com/uptrace/bun", recv: "DB", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "Conn", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "Tx", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	{pkg: "github.com/uptrace/bun", recv: "IDB", name: "RunInTx", outer: -1, callback: 2, tx: 1},
	// func BeginFunc(ctx, db interface{ Begin(...) }, fn func(Tx) error) error
	{pkg: "github.com/jackc/pgx/v5", name: "BeginFunc", outer: 1, callback: 2, tx: 0},
	// func BeginTxFunc(ctx, db interface{ BeginTx(...) }, txOptions TxOptions, fn func(Tx) error) error
	{pkg: "github.com/jackc/pgx/v5", name: "BeginTxFunc", outer: 1, callback: 3, tx: 0},
}

// Of reports the opener that call is. It reports nothing for a call that
// opens no transaction, and for one whose callback is not a function literal:
// a callback held in a variable or named as a function has no body here to
// read.
func Of(info *types.Info, call *ast.CallExpr) (Opener, bool) {
	fn, ok := typeutil.Callee(info, call).(*types.Func)
	if !ok || fn.Pkg() == nil {
		return Opener{}, false
	}
	recv := recvName(fn)
	for _, k := range knownOpeners {
		if k.pkg != fn.Pkg().Path() || k.name != fn.Name() || k.recv != recv {
			continue
		}
		return k.open(info, fn, call)
	}
	return Opener{}, false
}

func (k known) open(info *types.Info, fn *types.Func, call *ast.CallExpr) (Opener, bool) {
	o := Opener{Name: fn.Name()}
	if k.outer < 0 {
		// A method expression, as in (*gorm.DB).Transaction(db, fn), takes
		// its receiver as an argument, so its arguments are shifted and its
		// selector names no handle.
		sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
		if !ok || info.Selections[sel] == nil || info.Selections[sel].Kind() != types.MethodVal {
			return Opener{}, false
		}
		o.Outer = sel
	} else {
		o.Name = fn.Pkg().Name() + "." + fn.Name()
		o.Outer = call.Args[k.outer]
	}
	lit, ok := ast.Unparen(call.Args[k.callback]).(*ast.FuncLit)
	if !ok {
		return Opener{}, false
	}
	o.Callback = lit
	o.Tx = paramName(lit, k.tx)
	return o, true
}

// recvName is the name of the type a method is declared on, with any pointer
// removed. It is empty for a function.
func recvName(fn *types.Func) string {
	recv := fn.Signature().Recv()
	if recv == nil {
		return ""
	}
	t := recv.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

// paramName is the name of the parameter at index i of lit. It is empty when
// the parameter is unnamed or blank.
func paramName(lit *ast.FuncLit, i int) string {
	var names []string
	for _, field := range lit.Type.Params.List {
		if len(field.Names) == 0 {
			names = append(names, "")
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	if names[i] == "_" {
		return ""
	}
	return names[i]
}
