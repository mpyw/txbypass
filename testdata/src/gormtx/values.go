package gormtx

import (
	"context"

	"gorm.io/gorm"
)

// ===== Values that are the handle =====

// Both branches give the handle, so the variable they meet in is it.
func Branches(ctx context.Context, db *gorm.DB, debug bool, u *User) error {
	d := db
	if debug {
		d = db.Debug()
	}
	return d.Transaction(func(tx *gorm.DB) error {
		db.Create(u) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// A loop that gives the handle back each time gives the handle.
func Loop(ctx context.Context, db *gorm.DB, n int, u *User) error {
	d := db
	for range n {
		d = d.WithContext(ctx)
	}
	return d.Transaction(func(tx *gorm.DB) error {
		db.Create(u) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Branches that give two handles meet in neither.
func TwoHandles(db, logs *gorm.DB, which bool, u *User) error {
	d := db
	if which {
		d = logs
	}
	return d.Transaction(func(tx *gorm.DB) error {
		db.Create(u)
		return nil
	})
}

// A conversion, and an assertion, give the handle they convert.
type Wrapped gorm.DB

func Converted(db *gorm.DB, u *User) error {
	var i any = db
	w := (*Wrapped)(db)
	return db.Transaction(func(tx *gorm.DB) error {
		i.(*gorm.DB).Create(u)         // want `^i\.\(\*gorm\.DB\) bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		(*gorm.DB)(w).Create(u)        // want `^w bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		audit(any(db).(*gorm.DB))      // want `^any\(db\)\.\(\*gorm\.DB\) bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		if d, ok := i.(*gorm.DB); ok { // want `^i bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
			use(d)
		}
		return nil
	})
}

func use(...any) {}

// A method value binds the handle.
func MethodValue(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		create := db.Create // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		create(u)           // want `^create bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// An interface converted to another interface is the same handle.
type creator interface {
	Create(value interface{}) *gorm.DB
}

func Interfaces(db *gorm.DB, u *User) error {
	var c creator = db
	return db.Transaction(func(tx *gorm.DB) error {
		use(any(c)) // want `^c bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// A use inside the index on the left of an assignment runs.
func IndexOnLeft(db *gorm.DB, u *User, seen map[error]int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		seen[db.Create(u).Error] = 1 // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}
