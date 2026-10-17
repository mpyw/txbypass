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
		s.Create(&AuditLog{})    // want `^s bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		s.DB.Create(&AuditLog{}) // want `^s bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Service holds a Store, so the handle is two fields deep.
type Service struct {
	store Store
}

func (s *Service) Save(u *User) error {
	return s.store.Transaction(func(tx *gorm.DB) error {
		s.store.Create(u) // want `^s\.store bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Default is a handle held by the package.
var Default *gorm.DB

func Global(u *User) error {
	return Default.Transaction(func(tx *gorm.DB) error {
		Default.Create(u) // want `^Default bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Not reported: reading an error of the handle runs no statement.
func Field(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return db.Error
	})
}

// The statement is reported once. Reading its error, or its result,
// afterwards runs nothing, and an ignore on the statement leaves no report.
func StatementResult(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle // must survive a rollback
		logErr := db.Create(u).Error
		if logErr != nil {
			return logErr
		}
		res := db.Create(u) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		return nil
	})
}

// A method of the universe, such as error's, belongs to no package.
func errText(err error) string { return err.Error() }

// The error of a statement, read through error's own method, which belongs
// to no package.
func StatementErrorText(db *gorm.DB, u *User) string {
	var text string
	db.Transaction(func(tx *gorm.DB) error {
		if err := db.Create(u).Error; err != nil { // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
			text = err.Error()
		}
		return nil
	})
	return text
}

// errRepo holds the handle in a field.
type errRepo struct{ db *gorm.DB }

// Not reported: reading the error of the handle a field holds runs no
// statement.
func (r *errRepo) FieldError() error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Error
	})
}
