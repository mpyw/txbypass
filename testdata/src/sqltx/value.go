package sqltx

import "database/sql"

// A local handle held by value.
func LocalValue() {
	var db sql.DB
	tx, _ := db.Begin()
	db.Exec("SELECT 1") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	tx.Commit()
}

// SQLRepo holds the handle by value in a field.
type SQLRepo struct {
	db sql.DB
}

// A begin on a handle held by value in a field.
func (r *SQLRepo) Begin() error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	r.db.Exec("SELECT 1") // want `^r\.db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}
