package helper

import (
	"testing"

	"github.com/mpyw/txbypass/internal/txcall"
)

// TestInvokesIsAFact pins the marker that makes Invokes an analysis fact, and its
// spelling in the fixtures' want comments.
func TestInvokesIsAFact(t *testing.T) {
	f := &Invokes{Shape: txcall.InvokerShape{Outer: 0, Fields: []string{"db"}, Callback: 2, Tx: 1}}
	f.AFact()
	if got := f.String(); got != "invokes[p0.db, p2(1)]" {
		t.Errorf("String() = %q, want %q", got, "invokes[p0.db, p2(1)]")
	}
}

// TestBeginsIsAFact does for Begins what TestInvokesIsAFact does for Invokes.
func TestBeginsIsAFact(t *testing.T) {
	f := &Begins{Shape: txcall.BeginShape{Outer: 1, Fields: []string{"store", "db"}}}
	f.AFact()
	if got := f.String(); got != "begins[p1.store.db]" {
		t.Errorf("String() = %q, want %q", got, "begins[p1.store.db]")
	}
}
