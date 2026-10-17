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

// ===== Cycles that must not stop the analysis =====

// Pointers copied back and forth between two captured variables.
func CopiedBackAndForth(db *gorm.DB, u *User) error {
	repo := &LitRepo{db: db}
	a := repo
	b := a
	a = b
	defer func() { _, _ = a, b }()
	return repo.db.Transaction(func(tx *gorm.DB) error {
		return repo.db.Create(u).Error // want `^repo\.db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// A variable whose only store is a call on itself.
func Normalized(db *gorm.DB, conds []string) error {
	var q *gorm.DB
	defer func() { _ = q }()
	for _, c := range conds {
		q = q.Where(c)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Create(q).Error
	})
}

// ===== A method of the package that may rebind the field =====

type BindRepo struct{ db *gorm.DB }

func (r *BindRepo) Bind(tx *gorm.DB)   { r.db = tx }
func (r *BindRepo) Unbind(db *gorm.DB) { r.db = db }

// Not reported: Bind writes r.db, so it runs on tx.
func RebindViaMethod(db *gorm.DB, u *User) error {
	r := &BindRepo{db: db}
	return r.db.Transaction(func(tx *gorm.DB) error {
		r.Bind(tx)
		defer r.Unbind(db)
		return r.db.Create(u).Error
	})
}

// Not reported: the same, on a struct held by value.
func RebindViaMethodValue(db *gorm.DB, u *User) error {
	var r BindRepo
	r.db = db
	return r.db.Transaction(func(tx *gorm.DB) error {
		r.Bind(tx)
		return r.db.Create(u).Error
	})
}

// Not reported: r is given to a function value, which may rebind r.db.
func RebindViaFuncValue(db *gorm.DB, u *User, bind func(*BindRepo, *gorm.DB)) error {
	var r BindRepo
	r.db = db
	return r.db.Transaction(func(tx *gorm.DB) error {
		bind(&r, tx)
		return r.db.Create(u).Error
	})
}

// Not reported: a copy of the pointer is given away in a variable, so what
// it points to may be written there.
func PointerCopyGivenAway(db *gorm.DB, u *User, sink func(**BindRepo)) error {
	r := &BindRepo{db: db}
	q := r
	sink(&q)
	tx := r.db.Begin()
	r.db.Create(u)
	return tx.Commit().Error
}
