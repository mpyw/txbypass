package gormtx

import "gorm.io/gorm"

// ===== Variables assigned more than once =====

// Not reported: the callback points the captured variable at the
// transaction, so db holds two handles and is neither.
func Reassign(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		db = tx
		return db.Create(u).Error
	})
}

// dbPointer keeps the address of a handle.
type dbPointer struct{ p **gorm.DB }

// Not reported: the callback gives the variable's address away, so
// something else may write it, and which handle it holds is not known.
func EscapedInside(db *gorm.DB, h *dbPointer, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		h.p = &db
		return db.Create(u).Error
	})
}
