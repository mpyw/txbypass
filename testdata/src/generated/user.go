// Package generated has one generated file and files that are not.
package generated

import "gorm.io/gorm"

type User struct{ Name string }

// The same code outside the generated file is reported.
func UserCreate(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		db.Create(u) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}
