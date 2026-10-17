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

// gorm's Begin reports failure through tx.Error. In that branch, no
// transaction began.
func ManualFailed(db *gorm.DB, u *User) error {
	tx := db.Begin()
	if tx.Error != nil {
		db.Exec("INSERT INTO failures VALUES (1)")
		return tx.Error
	}
	db.Create(u) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}

// The same, checked as "== nil".
func ManualFailedElse(db *gorm.DB, u *User) error {
	tx := db.Begin()
	if tx.Error == nil {
		tx.Create(u)
		return tx.Commit().Error
	}
	db.Exec("INSERT INTO failures VALUES (1)")
	return tx.Error
}

// Another statement's error, or any other value, compared with nil is not
// the begin's.
func ManualOtherErrors(db *gorm.DB, u *User, check func() error) error {
	tx := db.Begin()
	if err := tx.Create(u).Error; err != nil {
		db.Exec("INSERT INTO failures VALUES (1)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		tx.Rollback()
		return err
	}
	if err := check(); err != nil {
		db.Exec("INSERT INTO failures VALUES (2)") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// A transaction kept in a variable a literal captures: its Error is still
// the begin's.
func ManualCaptured(db *gorm.DB, u *User) error {
	tx := db.Begin()
	defer func() { _ = tx }()
	if tx.Error != nil {
		db.Exec("INSERT INTO failures VALUES (1)")
		return tx.Error
	}
	return tx.Commit().Error
}
