package sqltx

import (
	"database/sql"
	"iter"
)

// ===== Ends in the body of a range over a function =====

// The rollback returns from the function, so the transaction is still open
// after the loop, as in a loop over a slice.
func RollbackReturnsInRangeFunc(db *sql.DB, items iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for it := range items {
		if _, err := tx.Exec("INSERT", it); err != nil {
			tx.Rollback()
			return err
		}
	}
	db.Ping() // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// Not reported: the commit breaks out of the loop, so the transaction may
// have ended after it.
func CommitBreaksInRangeFunc(db *sql.DB, items iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for it := range items {
		if it < 0 {
			tx.Commit()
			break
		}
	}
	db.Ping()
	return nil
}

// Not reported: the commit goes on to the next iteration.
func CommitGoesOnInRangeFunc(db *sql.DB, items iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for it := range items {
		if it < 0 {
			tx.Commit()
		}
	}
	db.Ping()
	return nil
}

// The inner loop's return returns from the function, so the transaction
// is still open after the loops.
func RollbackReturnsInNestedRangeFunc(db *sql.DB, items iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for range items {
		for it := range items {
			if it < 0 {
				tx.Rollback()
				return nil
			}
		}
	}
	db.Ping() // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// Not reported: the commit breaks out of the inner loop, and the outer one
// goes on.
func CommitBreaksInNestedRangeFunc(db *sql.DB, items iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for range items {
		for it := range items {
			if it < 0 {
				tx.Commit()
				break
			}
		}
	}
	db.Ping()
	return nil
}

// Not reported: a body with no branch commits and breaks.
func CommitBreaksAtOnceInRangeFunc(db *sql.DB, items iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for range items {
		tx.Commit()
		break
	}
	db.Ping()
	return nil
}

// A body with no branch rolls back and returns.
func RollbackReturnsAtOnceInRangeFunc(db *sql.DB, items iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for range items {
		tx.Rollback()
		return nil
	}
	db.Ping() // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}
