// Package outerhandleoff holds code that outer-handle reports, for a run
// with the rule off.
package outerhandleoff

import "gorm.io/gorm"

type User struct{ Name string }

// Not reported: outer-handle is off.
func Create(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		db.Create(u)
		return nil
	})
}

// Not reported as unused: the rule it names is off.
func Ignored(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle // the audit log must survive a rollback
		db.Create(u)
		return nil
	})
}
