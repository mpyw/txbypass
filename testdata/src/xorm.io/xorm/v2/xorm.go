// Package xorm is a stub of xorm.io/xorm/v2 for the analysistest fixtures.
//
// The signatures are those of xorm.io/xorm/v2 v2.0.0. Only what the fixtures use is
// kept, and the bodies do nothing. BSD 3-Clause License.
package xorm

// Session is a session, or a transaction.
type Session struct{}

func (session *Session) Insert(beans ...interface{}) (int64, error) { return 0, nil }

// Engine is a handle.
type Engine struct{}

func (engine *Engine) Insert(beans ...interface{}) (int64, error) { return 0, nil }

func (engine *Engine) Transaction(f func(*Session) (interface{}, error)) (interface{}, error) {
	return nil, nil
}
