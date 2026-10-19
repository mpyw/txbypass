package txcall

// endings are the names of the methods that end a transaction. go-pg's take
// a context as CommitContext and RollbackContext.
//
//declscope:shared // tx.go tells a type that has an ending method
var endings = map[string]bool{"Commit": true, "Rollback": true, "CommitContext": true, "RollbackContext": true}

// Ending reports whether a method of this name ends a transaction.
func Ending(name string) bool {
	return endings[name]
}
