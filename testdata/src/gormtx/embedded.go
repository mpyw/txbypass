package gormtx

import "gorm.io/gorm"

// Store embeds the handle, so its methods are the handle's.
type Store struct {
	*gorm.DB
}

// A method promoted from an embedded handle names the embedded field.
func (s Store) Save(u *User) error {
	return s.Transaction(func(tx *gorm.DB) error {
		tx.Save(u)
		s.Create(&AuditLog{}) // want `^s\.DB bypasses the transaction it opened with Transaction; use tx instead \[outer-handle\]$`
		s.DB.Create(&AuditLog{}) // want `^s\.DB bypasses the transaction it opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Service holds a Store, so the handle is two fields deep.
type Service struct {
	store Store
}

func (s *Service) Save(u *User) error {
	return s.store.Transaction(func(tx *gorm.DB) error {
		s.store.Create(u) // want `^s\.store\.DB bypasses the transaction it opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Default is a handle held by the package.
var Default *gorm.DB

func Global(u *User) error {
	return Default.Transaction(func(tx *gorm.DB) error {
		Default.Create(u) // want `^Default bypasses the transaction it opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Reading a field of the handle is a use of it.
func Field(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return db.Error // want `^db bypasses the transaction it opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// A method of the universe, such as error's, belongs to no package.
func errText(err error) string { return err.Error() }
