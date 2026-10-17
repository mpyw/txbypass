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

type Filter struct{ A, B, C bool }

// Every optional filter returns the receiver, so q is db however many
// conditions rebind it.
func OptionalFilters(db *gorm.DB, f Filter) error {
	q := db.Where("1")
	if f.A {
		q = q.Where("a")
	}
	if f.B {
		q = q.Where("b")
	}
	if f.C {
		q = q.Where("c")
	}
	t := db.Begin()
	if t.Error != nil {
		return t.Error
	}
	q.First(&User{}) // want `^q bypasses the transaction opened with Begin; use t instead \[outer-handle\]$`
	return t.Commit().Error
}

// err is a named result a deferred literal reads, so it lives in memory.
// Its check is still the begin's: no transaction began on that branch.
func ErrorFieldInMemory(db *gorm.DB, u *User) (err error) {
	tx := db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if err = tx.Error; err != nil {
		db.Create(u)
		return err
	}
	db.Create(u) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}

// Another statement's error, stored in the same variable, is not the
// begin's.
func OtherErrorInMemory(db *gorm.DB, u *User) (err error) {
	tx := db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if err = tx.Create(u).Error; err != nil {
		db.Create(u) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		return err
	}
	return tx.Commit().Error
}

// Twenty optional filters inside a loop: every branch leans on the loop's
// phi, and the walk stays linear.
func ManyFiltersInLoop(db *gorm.DB, fs []Filter, n int) error {
	q := db.Where("1")
	for range n {
		if fs[0].A {
			q = q.Where("f0")
		}
		if fs[1].A {
			q = q.Where("f1")
		}
		if fs[2].A {
			q = q.Where("f2")
		}
		if fs[3].A {
			q = q.Where("f3")
		}
		if fs[4].A {
			q = q.Where("f4")
		}
		if fs[5].A {
			q = q.Where("f5")
		}
		if fs[6].A {
			q = q.Where("f6")
		}
		if fs[7].A {
			q = q.Where("f7")
		}
		if fs[8].A {
			q = q.Where("f8")
		}
		if fs[9].A {
			q = q.Where("f9")
		}
		if fs[10].A {
			q = q.Where("f10")
		}
		if fs[11].A {
			q = q.Where("f11")
		}
		if fs[12].A {
			q = q.Where("f12")
		}
		if fs[13].A {
			q = q.Where("f13")
		}
		if fs[14].A {
			q = q.Where("f14")
		}
		if fs[15].A {
			q = q.Where("f15")
		}
		if fs[16].A {
			q = q.Where("f16")
		}
		if fs[17].A {
			q = q.Where("f17")
		}
		if fs[18].A {
			q = q.Where("f18")
		}
		if fs[19].A {
			q = q.Where("f19")
		}
	}
	t := db.Begin()
	if t.Error != nil {
		return t.Error
	}
	q.First(&User{}) // want `^q bypasses the transaction opened with Begin; use t instead \[outer-handle\]$`
	return t.Commit().Error
}

// A query built in nested loops is db wherever it is used.
func NestedLoops(db *gorm.DB, fs [][]string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		q := db.Debug() // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		for _, f := range fs {
			for _, v := range f {
				q = q.Where(v) // want `^q bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
			}
			q.First(&User{}) // want `^q bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		}
		return nil
	})
}
