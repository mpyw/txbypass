// Package gormtx covers outer-handle with gorm's Transaction.
package gormtx

import (
	"context"
	"fmt"
	"testing"

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

// A savepoint runs on the connection of the transaction it is opened on, so
// the outer transaction's handle runs inside it.
func Savepoint(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Transaction(func(sp *gorm.DB) error {
			// Not reported: a savepoint runs on the transaction's connection.
			tx.Create(u)
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

// Not reported: a savepoint opened on a begin's transaction.
func SavepointOnBegin(db *gorm.DB, u *User) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	err := tx.Transaction(func(sp *gorm.DB) error {
		// Not reported: a savepoint runs on the transaction's connection.
		tx.Create(u)
		return nil
	})
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// A handle that is no transaction still opens one, and is reported.
func NotASavepoint(db *gorm.DB, other func() *gorm.DB, u *User) error {
	h := other()
	return h.Transaction(func(sp *gorm.DB) error {
		return h.Create(u).Error // want `^h bypasses the transaction opened with Transaction; use sp instead \[outer-handle\]$`
	})
}

// Not reported: the outer callback captures nothing, so the opener's call
// names it directly.
func SavepointNoCapture(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Transaction(func(sp *gorm.DB) error {
			// Not reported: a savepoint runs on the transaction's connection.
			return tx.Exec("SELECT 1").Error
		})
	})
}

// A literal given to a function value is no callback of an opener, so its
// parameter is not known to be a transaction.
func ParamOfFuncValue(run func(func(*gorm.DB) error) error) error {
	return run(func(h *gorm.DB) error {
		return h.Transaction(func(sp *gorm.DB) error {
			return h.Exec("SELECT 1").Error // want `^h bypasses the transaction opened with Transaction; use sp instead \[outer-handle\]$`
		})
	})
}

// Not reported: a begin on a begin's transaction opens a savepoint.
func BeginOnBegin(db *gorm.DB, u *User) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	sp := tx.Begin()
	// Not reported: a savepoint runs on the transaction's connection.
	tx.Create(u)
	sp.Commit()
	return tx.Commit().Error
}

// Not reported: a cleanup registered in the callback runs after the test,
// when the transaction has ended.
func CleanupInCallback(t *testing.T, db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		u := User{}
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		t.Cleanup(func() { db.Delete(&u) })
		return nil
	})
}

// adminTX keeps the handle beside the transaction.
type adminTX struct {
	db *gorm.DB
	tx *gorm.DB
}

// Not reported: the literal keeps db beside the callback's transaction, as
// a constructor given both would.
func KeptBeside(db *gorm.DB) *adminTX {
	var kept *adminTX
	db.Transaction(func(tx *gorm.DB) error {
		kept = &adminTX{db: db, tx: tx}
		_ = adminTX{db, tx}
		return nil
	})
	return kept
}

// A literal of a type parameter whose core type is a struct.
func SaveGeneric[T ~struct{ ID int }](db *gorm.DB, id int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		v := T{id}
		_ = v
		return db.Create(&User{}).Error // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// Keyed is a constraint whose type set is one struct.
type Keyed interface{ ~struct{ ID int } }

// Entity embeds it, with a method.
type Entity interface {
	Keyed
	Name() string
}

// EntityRepo is a generic repository whose methods build literals of T.
type EntityRepo[T Entity] struct{ db *gorm.DB }

func (r *EntityRepo[T]) Save(id int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		v := T{id}
		_ = v
		return r.db.Create(&User{}).Error // want `^r\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// A constraint that embeds the interface with the terms.
func SaveEmbedded[T interface{ Keyed }](db *gorm.DB, id int) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	v := T{id}
	_ = v
	db.Create(&User{}) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}
