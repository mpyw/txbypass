// Package require is a stub of github.com/stretchr/testify/require for the
// analysistest fixtures.
//
// The signatures are those of github.com/stretchr/testify v1.11.1. Only what
// the fixtures use is kept. Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell
// and contributors. MIT License:
// https://github.com/stretchr/testify/blob/v1.11.1/LICENSE
package require

// TestingT is what a test gives the assertions.
type TestingT interface {
	Errorf(format string, args ...interface{})
	FailNow()
}

// Error stops the test unless err is set.
func Error(t TestingT, err error, msgAndArgs ...interface{}) {
	if err == nil {
		t.FailNow()
	}
}

// ErrorIs stops the test unless err is target.
func ErrorIs(t TestingT, err error, target error, msgAndArgs ...interface{}) {
	if err != target {
		t.FailNow()
	}
}

// NoError stops the test when err is set.
func NoError(t TestingT, err error, msgAndArgs ...interface{}) {
	if err != nil {
		t.FailNow()
	}
}

// Assertions are the assertions bound to one test.
type Assertions struct{ t TestingT }

// New binds the assertions to t.
func New(t TestingT) *Assertions { return &Assertions{t: t} }

// Error stops the test unless err is set.
func (a *Assertions) Error(err error, msgAndArgs ...interface{}) { Error(a.t, err, msgAndArgs...) }
