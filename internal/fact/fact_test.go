package fact

import (
	"testing"

	"github.com/mpyw/txbypass/internal/txcall"
)

// TestFactsAreFacts pins the marker that makes each type an analysis fact,
// and its spelling in the fixtures' want comments.
func TestFactsAreFacts(t *testing.T) {
	cases := []struct {
		f interface {
			AFact()
			String() string
		}
		want string
	}{
		{&Closes{Params: []int{0, 2}}, "closes[0 2]"},
		{&Invokes{Shape: txcall.InvokerShape{Outer: 0, Fields: []string{"db"}, Callback: 2, Tx: 1}}, "invokes[p0.db, p2(1)]"},
		{&Invokes{Shape: txcall.InvokerShape{Outer: 1, Callback: 2, InContext: true}}, "invokes[p1, p2(0), context]"},
		{&Begins{Shape: txcall.BeginShape{Outer: 1, Fields: []string{"store", "db"}}}, "begins[p1.store.db]"},
	}
	for _, tt := range cases {
		tt.f.AFact()
		if got := tt.f.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}
