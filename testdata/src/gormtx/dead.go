package gormtx

import "gorm.io/gorm"

// Not reported, and no crash: the literal is after a return, so it never
// runs.
func LiteralInDeadCode(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return nil
		go func() { db.Create(u) }()
		return nil
	})
}
