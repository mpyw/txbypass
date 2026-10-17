package closer

import "testing"

// TestFactIsAFact pins the marker that makes Fact an analysis fact, and its
// spelling in the fixtures' want comments.
func TestFactIsAFact(t *testing.T) {
	f := &Fact{Params: []int{0, 2}}
	f.AFact()
	if got := f.String(); got != "closes[0 2]" {
		t.Errorf("String() = %q, want %q", got, "closes[0 2]")
	}
}
