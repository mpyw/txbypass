// Package xormtx covers outer-handle with xorm, v1 and v2.
package xormtx

import (
	"xorm.io/xorm"
	xorm2 "xorm.io/xorm/v2"
)

type User struct{ Name string }

func V1(engine *xorm.Engine, u *User) error {
	_, err := engine.Transaction(func(session *xorm.Session) (interface{}, error) {
		return engine.Insert(u) // want `^engine bypasses the transaction opened with Transaction; use session instead \[outer-handle\]$`
	})
	return err
}

func V2(engine *xorm2.Engine, u *User) error {
	_, err := engine.Transaction(func(session *xorm2.Session) (interface{}, error) {
		return engine.Insert(u) // want `^engine bypasses the transaction opened with Transaction; use session instead \[outer-handle\]$`
	})
	return err
}
