package gormtx

import (
	"context"

	"gorm.io/gorm"
)

// ===== Handles held in structs the function builds =====

type LitRepo struct {
	db *gorm.DB
}

func PtrLiteral(db *gorm.DB, u *User) error {
	r := &LitRepo{db: db}
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error // want `^r\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

func ValueLiteral(db *gorm.DB, u *User) error {
	r := LitRepo{db: db}
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error // want `^r\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

func FieldByField(db *gorm.DB, u *User) error {
	var r LitRepo
	r.db = db
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error // want `^r\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// A value receiver whose copy binds the context.
func (r LitRepo) Save(ctx context.Context, u *User) error {
	r.db = r.db.WithContext(ctx)
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error // want `^r\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

func PtrLiteralRegion(db *gorm.DB, u *User) error {
	r := &LitRepo{db: db}
	tx := r.db.Begin()
	r.db.Create(u) // want `^r\.db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit().Error
}

// Not reported: a literal inside the callback writes the field.
func FieldWrittenLater(db *gorm.DB, u *User) error {
	r := &LitRepo{db: db}
	return r.db.Transaction(func(tx *gorm.DB) error {
		func() { r.db = tx }()
		return r.db.Create(u).Error
	})
}

// ===== Addresses that go where this analysis does not follow =====

var (
	keptField **gorm.DB
	keptRepo  struct{ repo *LitRepo }
)

func keepRepo(pp **LitRepo) {}

// Not reported: the field's address is kept elsewhere, so something else
// may write it.
func FieldAddressKept(db *gorm.DB, u *User) error {
	r := LitRepo{db: db}
	keptField = &r.db
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error
	})
}

// Not reported: the pointer is kept elsewhere.
func PointerKept(db *gorm.DB, u *User) error {
	r := &LitRepo{db: db}
	keptRepo.repo = r
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error
	})
}

// Not reported: a literal keeps the pointer elsewhere.
func PointerKeptByLiteral(db *gorm.DB, u *User) error {
	r := &LitRepo{db: db}
	keep := func() { keptRepo.repo = r }
	keep()
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error
	})
}

// Not reported: the variable's own address is given away.
func VariableAddressGiven(db *gorm.DB, u *User) error {
	r := &LitRepo{db: db}
	keepRepo(&r)
	return r.db.Transaction(func(tx *gorm.DB) error {
		return r.db.Create(u).Error
	})
}
