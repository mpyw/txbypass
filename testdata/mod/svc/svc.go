// Package svc covers outer-handle in module mode, where a function of
// another package of the module is not a library.
package svc

import (
	"example.com/app/repo"

	"gorm.io/gorm"
)

type User struct{}

// Not reported: Bind, of another package of the module, may rebind r.DB.
func RebindInModule(db *gorm.DB, u *User) error {
	r := &repo.Repo{DB: db}
	return r.DB.Transaction(func(tx *gorm.DB) error {
		r.Bind(tx)
		return r.DB.Create(u).Error
	})
}

// A library's method is followed: Begin on a handle held by value does not
// rebind it.
func ByValue(u *User) error {
	var db gorm.DB
	tx := db.Begin()
	db.Create(u) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}
