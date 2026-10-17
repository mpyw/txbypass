package gormtx

import "gorm.io/gorm"

// ===== gorm's Begin =====

// The manual transaction from the gorm docs. Begin gives the transaction, a
// new handle, although it has the receiver's type.
func Manual(db *gorm.DB, u *User) error {
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Error; err != nil {
		return err
	}
	if err := tx.Create(u).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// The handle that began it is still the outer handle.
func ManualBypass(db *gorm.DB, u *User) error {
	tx := db.WithContext(nil).Begin()
	tx.Create(u)
	db.Create(&AuditLog{}) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}

// A begin on a handle held by value in a field.
func (r *ValueRepo) Begin(u *User) error {
	tx := r.db.Begin()
	r.db.Create(u) // want `^r\.db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}
