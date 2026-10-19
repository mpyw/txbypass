// Package fact holds the facts the analysis of a package exports, so that
// the analysis of a package that imports it reads them.
//
// A fact says what a function does with a transaction: which parameters it
// ends, that it invokes a callback inside one, or that it returns one it
// begins. Its body says so, or a declaration does.
package fact

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mpyw/txbypass/internal/txcall"
)

// Closes says which parameters a function ends, before it returns or by
// letting them go. The receiver of a method is parameter 0.
type Closes struct {
	Params []int
	// Declared is true when a //txbypass:closes directive, not the body,
	// says so. Params then holds every input, and only the first that may
	// be a transaction is one.
	Declared bool
}

// AFact marks Closes as an analysis fact.
func (*Closes) AFact() {}

func (f *Closes) String() string { return fmt.Sprintf("closes%v", f.Params) }

// Invokes says that a function is an invoker, and of what shape.
type Invokes struct {
	Shape txcall.InvokerShape
}

// AFact marks Invokes as an analysis fact.
func (*Invokes) AFact() {}

func (f *Invokes) String() string {
	ctx := ""
	if f.Shape.InContext {
		ctx = ", context"
	}
	return fmt.Sprintf("invokes[p%d%s, p%d(%d)%s]", f.Shape.Outer, fields(f.Shape.Fields), f.Shape.Callback, f.Shape.Tx, ctx)
}

// Begins says that a function returns a transaction it begins, and on
// what.
type Begins struct {
	Shape txcall.BeginShape
}

// AFact marks Begins as an analysis fact.
func (*Begins) AFact() {}

func (f *Begins) String() string {
	return "begins[p" + strconv.Itoa(f.Shape.Outer) + fields(f.Shape.Fields) + "]"
}

// fields spells fields selected one after another, as .db.
func fields(names []string) string {
	var b strings.Builder
	for _, name := range names {
		b.WriteString("." + name)
	}
	return b.String()
}
