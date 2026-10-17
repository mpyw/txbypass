package gormtx

import (
	"database/sql"

	"dbs"

	"gorm.io/gorm"
)

// ===== Literals that are made, and literals that are not =====

type group struct{}

func (g *group) Go(f func() error) {}

var tasks group

// A literal with no free variables, run by a call whose result is dropped,
// runs inside the transaction.
func SharedImmediate() error {
	return dbs.Main.Transaction(func(tx *gorm.DB) error {
		func() {
			dbs.Main.Create(&User{}) // want `^dbs\.Main bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		}()
		tasks.Go(func() error {
			return dbs.Main.Create(&User{}).Error // want `^dbs\.Main bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		})
		return nil
	})
}

// The same in a region.
func SharedRegion() error {
	tx := dbs.Main.Begin()
	func() {
		dbs.Main.Create(&User{}) // want `^dbs\.Main bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	}()
	return tx.Commit().Error
}

// Not reported: a begin or an opener in a literal that is never made never
// runs.
func NeverMadeBegin(db *sql.DB, g *gorm.DB, u *User) {
	_ = func() {
		tx, err := db.Begin()
		if err != nil {
			return
		}
		db.Exec("x")
		tx.Commit()
	}
	_ = func() error {
		return g.Transaction(func(tx *gorm.DB) error {
			return g.Create(u).Error
		})
	}
}

// A deferred literal with no free variables that resets a package-level
// handle runs after the commit.
func DeferredGlobalReset(u *User) error {
	tx := dbs.Main.Begin()
	defer func() { dbs.Main = nil }()
	dbs.Main.Create(u) // want `^dbs\.Main bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}
