package gormtx

import (
	"context"

	"gorm.io/gorm"
)

// ===== Handles assigned again before the transaction =====

// db is assigned again once before the transaction, then captured. The
// callback runs while db holds that value.
func ReassignedBefore(ctx context.Context, db *gorm.DB, u *User) error {
	db = db.WithContext(ctx)
	return db.Transaction(func(tx *gorm.DB) error {
		tx.Create(u)
		return db.Create(u).Error // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// The same with a local.
func ReassignedLocal(open func() *gorm.DB, u *User) error {
	db := open()
	db = db.Debug()
	return db.Transaction(func(tx *gorm.DB) error {
		return db.Create(u).Error // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
	})
}

// ===== Fields and copies set to the transaction =====

type TxService struct {
	db *gorm.DB
}

// Not reported: the field is set to the transaction inside the callback.
func (s *TxService) Swap(u *User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		s.db = tx
		return s.db.Create(u).Error
	})
}

// Not reported: a copy of the service, bound to the transaction.
func (s *TxService) Copy(u *User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		c := *s
		c.db = tx
		return c.db.Create(u).Error
	})
}

// Not reported: the whole service is replaced inside the callback.
func ReplaceService(r *TxService, u *User) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		r = &TxService{db: tx}
		return r.db.Create(u).Error
	})
}

// Not reported: the same through a promoted method.
func ReplacePromoted(s *Repository, u *User) error {
	return s.Transaction(func(tx *gorm.DB) error {
		s = &Repository{DB: tx}
		return s.Create(u).Error
	})
}

// ===== Methods that bind a value to the transaction =====

func (s *TxService) Using(tx *gorm.DB) *TxService { c := *s; c.db = tx; return &c }

func (r *Repository) On(tx *gorm.DB) *Repository { return &Repository{DB: tx} }

// Not reported: a method given the transaction gives a value bound to it,
// whatever its name.
func (s *TxService) Bound(r *Repository, u *User) error {
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.Using(tx).db.Create(u).Error
	}); err != nil {
		return err
	}
	return r.Transaction(func(tx *gorm.DB) error {
		return r.On(tx).Create(u).Error
	})
}

// Not reported: a literal inside the callback sets the field.
func (s *TxService) SwapInLiteral(u *User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		func() { s.db = tx }()
		return s.db.Create(u).Error
	})
}
