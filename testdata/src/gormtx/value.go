package gormtx

import "gorm.io/gorm"

// ===== Handles held by value =====

// ValueRepo holds the handle by value, not through a pointer.
type ValueRepo struct {
	db gorm.DB
}

// A callback on a handle held by value in a field.
func (r *ValueRepo) Save(u *User) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error // want `^r\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// Writing the field holding the handle is not a use of it.
func (r *ValueRepo) Reset(other gorm.DB) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		r.db = other
		return nil
	})
}
