// Package directives covers //txbypass:ignore and the other //txbypass:
// directives.
package directives

import "gorm.io/gorm"

type AuditLog struct{ Action string }

func use(...any) {}

// ===== Silencing =====

// Not reported: the directive on the line above silences outer-handle.
func Above(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle // the audit log must survive a rollback
		db.Create(&AuditLog{})
		return nil
	})
}

// Not reported: the directive on the same line silences outer-handle.
func SameLine(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		db.Create(&AuditLog{}) //txbypass:ignore outer-handle // the audit log must survive a rollback
		return nil
	})
}

// Not reported: an ignore with no rules silences every rule.
func AllRules(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore // the audit log must survive a rollback
		db.Create(&AuditLog{})
		return nil
	})
}

// Not reported: one directive silences two reports on its line.
func TwoReports(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle // both must survive a rollback
		use(db.Create(&AuditLog{}), db.Create(&AuditLog{}))
		return nil
	})
}

// ===== Directives that silence nothing =====

// The directive covers its own line and the line below, not further.
func TwoLinesBelow(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle // too far // want `^unused txbypass:ignore directive$`
		use()
		db.Create(&AuditLog{}) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// Nothing is reported where the directive is.
func NothingToSilence(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle // not needed // want `^unused txbypass:ignore directive$`
		tx.Create(&AuditLog{})
		return nil
	})
}

// An ignore with no rules that silences nothing.
func NothingToSilenceAll(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore // not needed // want `^unused txbypass:ignore directive$`
		tx.Create(&AuditLog{})
		return nil
	})
}

// A trailing directive silences its own line only, not the line below.
func TrailingDoesNotReachBelow(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		use()                  //txbypass:ignore outer-handle // trailing // want `^unused txbypass:ignore directive$`
		db.Create(&AuditLog{}) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}

// ===== Malformed directives =====

// A missing or empty reason cannot be pinned here: the expectation comment
// on the directive's line would be read as its reason. A unit test covers
// "txbypass:ignore needs a reason after //".

// The known rule still silences; the unknown one is reported.
func UnknownRule(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle, bogus // intended // want `^unknown rule bogus in txbypass:ignore$`
		db.Create(&AuditLog{})
		return nil
	})
}

// Not reported as unused: every rule is unknown, so only that is reported.
func AllUnknown(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore bogus // intended // want `^unknown rule bogus in txbypass:ignore$`
		tx.Create(&AuditLog{})
		return nil
	})
}

// The known rule still silences; the empty name is reported.
func EmptyRuleName(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		//txbypass:ignore outer-handle, // intended // want `^empty rule name in txbypass:ignore$`
		db.Create(&AuditLog{})
		return nil
	})
}

//txbypass:foo // want `^unknown directive txbypass:foo$`

// A report about a directive cannot be silenced, so this ignore silences
// nothing.
//
//txbypass:ignore // tries to silence the line below // want `^unused txbypass:ignore directive$`
//txbypass:bar // want `^unknown directive txbypass:bar$`

//txbypass:ignorex // want `^unknown directive txbypass:ignorex$`

// Prose that mentions txbypass:ignore with a space is not a directive, so
// it neither silences nor is reported as unused.
func Prose(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// txbypass:ignore outer-handle
		db.Create(&AuditLog{}) // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return nil
	})
}
