package gormtx

import (
	"database/sql"

	"gorm.io/gorm"
)

// ===== Function literals at the package level =====

// Cases is a table of test cases, as table-driven tests keep them.
var Cases = []struct {
	Name string
	Run  func(db *gorm.DB, u *User) error
}{
	{"bypass", func(db *gorm.DB, u *User) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return db.Create(u).Error // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		})
	}},
}

// Handler is a package-level function value.
var Handler = func(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	db.Exec("bypass") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// Opened is a transaction the package initializer opens.
var Opened = Default.Transaction(func(tx *gorm.DB) error {
	return Default.Create(&AuditLog{}).Error // want `^Default bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
})

// Not read: a declaration with no literal or call.
const answer = 42
