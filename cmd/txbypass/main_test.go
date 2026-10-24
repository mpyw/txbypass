package main

import (
	"os"
	"testing"
)

// TestMain prepares the flags as main does, so the tests see them.
func TestMain(m *testing.M) {
	mainPrepare()
	os.Exit(m.Run())
}
