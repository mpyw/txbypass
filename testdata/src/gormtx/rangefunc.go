package gormtx

import (
	"iter"

	"gorm.io/gorm"
)

// ===== The body of a range over a function =====

// The builder makes the loop body a function of its own. An invoker in it
// is read.
func InvokerInRangeFunc(db *gorm.DB, users iter.Seq[*User]) error {
	for u := range users {
		if err := db.Transaction(func(tx *gorm.DB) error {
			return db.Create(u).Error // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		}); err != nil {
			return err
		}
	}
	return nil
}

// A begin in it is read.
func BeginInRangeFunc(db *gorm.DB, users iter.Seq[*User]) error {
	for u := range users {
		tx := db.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		db.Create(u) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		if err := tx.Commit().Error; err != nil {
			return err
		}
	}
	return nil
}

// A use in it, inside a callback, is read.
func UseInRangeFunc(db *gorm.DB, users iter.Seq[*User]) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for u := range users {
			db.Create(u) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		}
		return nil
	})
}

// A use in it, inside a region, is read.
func UseInRangeFuncRegion(db *gorm.DB, users iter.Seq[*User]) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	for u := range users {
		db.Create(u) // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	}
	return tx.Commit().Error
}

// The range expression is evaluated outside the loop body.
func RangeExprInCallback(db *gorm.DB, seq func(*gorm.DB) iter.Seq[*User]) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for u := range seq(db) { // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
			if err := tx.Create(u).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
