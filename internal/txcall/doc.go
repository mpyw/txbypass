// Package txcall knows the calls that open a transaction.
//
// There are two shapes. An invoker runs a function literal inside the
// transaction, as gorm's Transaction does. A begin returns the transaction,
// which runs until it is committed or rolled back, as database/sql's BeginTx
// does.
//
// Each is read from types, by its package path, its receiver and its name, so
// an alias or a wrapper type of the caller's does not hide it. ent generates
// its client into the caller's package, so its begin is read from the shape
// of the generated code instead.
package txcall
