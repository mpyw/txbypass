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

// Not reported: a labeled break out of the outer loop, after a rollback,
// goes on after the loops.
func RollbackBreaksOuterInRangeFunc(db *sql.DB, batches []iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
outer:
	for _, batch := range batches {
		for it := range batch {
			if it < 0 {
				tx.Rollback()
				break outer
			}
		}
	}
	db.Exec("UPDATE imports SET finished = true")
	return nil
}

// insertAll rolls the transaction back and returns when an insert fails,
// so it ends its parameter.
func insertAll(tx *sql.Tx, rows iter.Seq[int]) error { // want insertAll:"closes\\[0\\]"
	for r := range rows {
		if _, err := tx.Exec("INSERT", r); err != nil {
			tx.Rollback()
			return err
		}
	}
	return nil
}

// Not reported: insertAll may have rolled the transaction back.
func ImportThroughHelper(db *sql.DB, rows iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := insertAll(tx, rows); err != nil {
		db.Exec("INSERT INTO import_failures VALUES (1)")
		return err
	}
	return tx.Commit()
}

// Not reported: the use after the rollback, in the same iteration, runs
// with the transaction ended, as in a loop over a slice.
func RollbackThenUseInRangeFunc(db *sql.DB, rows iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for r := range rows {
		if _, err := tx.Exec("INSERT", r); err != nil {
			tx.Rollback()
			db.Exec("INSERT INTO import_failures VALUES ($1)", r)
			return err
		}
	}
	return tx.Commit()
}

// A use before the end in the same iteration is inside.
func UseThenRollbackInRangeFunc(db *sql.DB, rows iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for r := range rows {
		db.Exec("SELECT 1") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
		if r < 0 {
			tx.Rollback()
			return nil
		}
	}
	return tx.Commit()
}

// Not reported: an iteration after one that committed and went on may run
// with the transaction ended.
func CommitGoesOnThenUseInRangeFunc(db *sql.DB, rows iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for r := range rows {
		db.Exec("SELECT 1")
		if r < 0 {
			tx.Commit()
		}
	}
	return nil
}

// Not reported: a goto out of the body, after a rollback, to code that has
// no other way in.
func RollbackGotoInRangeFunc(db *sql.DB, rows iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for r := range rows {
		if _, err := tx.Exec("INSERT", r); err != nil {
			tx.Rollback()
			goto failed
		}
	}
	return tx.Commit()
failed:
	db.Exec("UPDATE imports SET failed = true")
	return nil
}

// Not reported: the labeled break is the only way out of the outer loop.
func RollbackBreaksForeverInRangeFunc(db *sql.DB, pages func() iter.Seq[int]) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
outer:
	for {
		for r := range pages() {
			if _, err := tx.Exec("INSERT", r); err != nil {
				tx.Rollback()
				break outer
			}
		}
	}
	db.Exec("UPDATE imports SET failed = true")
	return nil
}

// A return with a deferred call still returns.
func RollbackReturnsWithDeferInRangeFunc(db *sql.DB, rows iter.Seq[int]) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = err }()
	for r := range rows {
		if _, err = tx.Exec("INSERT", r); err != nil {
			tx.Rollback()
			return err
		}
	}
	db.Ping() // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}
