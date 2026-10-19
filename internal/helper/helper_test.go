package helper

import (
	"testing"

	"github.com/mpyw/txbypass/internal/txcall"
)

// TestFactIsAFact pins the marker that makes Fact an analysis fact, and its
// spelling in the fixtures' want comments.
func TestFactIsAFact(t *testing.T) {
	f := &Invokes{Shape: txcall.InvokerShape{Outer: 0, Fields: []string{"db"}, Callback: 2, Tx: 1}}
	f.AFact()
	if got := f.String(); got != "invokes[p0.db, p2(1)]" {
		t.Errorf("String() = %q, want %q", got, "invokes[p0.db, p2(1)]")
	}
}
