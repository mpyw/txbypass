package sqltx

import "database/sql"

// ===== Helpers that pass the transaction to each other and never end it =====

func ring0(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring1(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring2(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring3(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring4(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring5(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring6(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring7(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring8(tx, d-1); err != nil {
		return err
	}
	return nil
}

func ring8(tx *sql.Tx, d int) error {
	if d <= 0 {
		return nil
	}
	if err := ring0(tx, d-1); err != nil {
		return err
	}
	if err := ring1(tx, d-1); err != nil {
		return err
	}
	if err := ring2(tx, d-1); err != nil {
		return err
	}
	if err := ring3(tx, d-1); err != nil {
		return err
	}
	if err := ring4(tx, d-1); err != nil {
		return err
	}
	if err := ring5(tx, d-1); err != nil {
		return err
	}
	if err := ring6(tx, d-1); err != nil {
		return err
	}
	if err := ring7(tx, d-1); err != nil {
		return err
	}
	return nil
}

// Nine helpers that call each other read in linear time, and none ends the
// transaction.
func UseRing(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if err := ring0(tx, 3); err != nil {
		return err
	}
	db.Exec("after the helpers") // want `^db bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}
