// Package gormtx covers outer-handle with gorm's Transaction.
package gormtx

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"dbs"
)

type User struct{ Name string }

type AuditLog struct{ Action string }

// ===== Reported =====

// The handle that opened the transaction is used inside it.
func Create(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		tx.Create(u)
		db.Create(&AuditLog{Action: "create"}) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// A method that returns its receiver's type gives the same handle back, on
// either side of the transaction.
func Chained(ctx context.Context, db *gorm.DB, u *User) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Create(u)
		db.WithContext(ctx).Debug().Create(&AuditLog{}) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

type Repo struct {
	db *gorm.DB
}

// A field is the same handle when it is selected from the same variable.
func (r *Repo) Save(u *User) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		tx.Save(u)
		r.db.Save(&AuditLog{}) // want `^r\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// The handle is passed on.
func Passed(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		tx.Create(u)
		return audit(db) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

func audit(db *gorm.DB) error { return db.Create(&AuditLog{}).Error }

// A function literal nested in the callback, such as a goroutine or a
// deferred call, runs while the transaction is open too.
func Nested(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		defer func() {
			db.Create(&AuditLog{}) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		}()
		go func() {
			db.Create(&AuditLog{}) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		}()
		tx.Create(u)
		return nil
	})
}

// A second transaction opened on the outer handle inside the first is a
// use of it. What runs in the second is reported once, not once for each
// transaction.
func Twice(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return db.Transaction(func(tx2 *gorm.DB) error { // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
			db.Create(u) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
			return nil
		})
	})
}

// A savepoint is a transaction too: the handle of the outer transaction
// bypasses it.
func Savepoint(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Transaction(func(sp *gorm.DB) error {
			tx.Create(u) // want `^tx bypasses the transaction opened with Transaction; use sp instead \[outer-handle\]$`
			return nil
		})
	})
}

// A blank parameter leaves the transaction unnamed.
func Blank(db *gorm.DB, u *User) error {
	return db.Transaction(func(_ *gorm.DB) error {
		db.Create(u) // want `^db bypasses the transaction opened with Transaction; use the callback's transaction instead \[outer-handle\]$`
		return nil
	})
}

// So does a parameter with no name.
func Unnamed(db *gorm.DB, u *User) error {
	return db.Transaction(func(*gorm.DB) error {
		db.Create(u) // want `^db bypasses the transaction opened with Transaction; use the callback's transaction instead \[outer-handle\]$`
		return nil
	})
}

// A parenthesized callback is still the callback.
func Paren(db *gorm.DB, u *User) error {
	return db.Transaction((func(tx *gorm.DB) error {
		(*&db).Create(u) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	}))
}

// A handle another package holds is qualified by its package's name.
func Shared(u *User) error {
	return dbs.Main.Transaction(func(tx *gorm.DB) error {
		dbs.Main.Create(u) // want `^dbs\.Main bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// ===== Not reported =====

// The transaction is used.
func Clean(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		tx.Session(&gorm.Session{DryRun: true}).Create(u)
		create := (*gorm.DB).Create
		create(tx, &AuditLog{Action: fmt.Sprint(u)})
		return nil
	})
}

// A parameter named as the outer handle shadows it.
func Shadowed(db *gorm.DB, u *User) error {
	return db.Transaction(func(db *gorm.DB) error {
		db.Create(u)
		return nil
	})
}

// Another handle is not the one the transaction was opened on.
func Other(db, logs *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		tx.Create(u)
		logs.Create(&AuditLog{})
		return nil
	})
}

// Another field of the same variable is another handle.
type TwoRepos struct {
	db, logs *gorm.DB
}

func (r *TwoRepos) Save(u *User) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		tx.Save(u)
		r.logs.Save(&AuditLog{})
		_ = TwoRepos{db: tx}
		return nil
	})
}

// The handle is used after the transaction ends.
func After(db *gorm.DB, u *User) error {
	if err := db.Transaction(func(tx *gorm.DB) error {
		tx.Create(u)
		return nil
	}); err != nil {
		return err
	}
	db.Create(&AuditLog{})
	return nil
}

// A callback that is not a function literal has no body here to read.
func Value(db *gorm.DB, u *User) error {
	fn := func(tx *gorm.DB) error {
		db.Create(u)
		return nil
	}
	return db.Transaction(fn)
}

// A method expression takes its receiver as an argument.
func MethodExpr(db *gorm.DB, u *User) error {
	return (*gorm.DB).Transaction(db, func(tx *gorm.DB) error {
		db.Create(u)
		return nil
	})
}

// A handle that is not named from a variable is not followed.
func Fresh(u *User) error {
	return open().Transaction(func(tx *gorm.DB) error {
		open().Create(u)
		return nil
	})
}

func open() *gorm.DB { return &gorm.DB{} }

// A copy of the handle is the handle.
func Copied(db *gorm.DB, u *User) error {
	d := db
	return db.Transaction(func(tx *gorm.DB) error {
		d.Create(u) // want `^d bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// A method that returns something other than its receiver's type gives
// another value.
func Underlying(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		sqlDB, err := tx.DB()
		_ = sqlDB
		return err
	})
}
