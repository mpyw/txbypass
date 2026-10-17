package gormtx

import "gorm.io/gorm"

// ===== Hooks that run after the commit =====

type Event struct{}

// Not reported: the callback keeps the literal in a variable declared
// outside it, to run after the transaction returns.
func AfterCommit(db *gorm.DB, u *User) error {
	var after func()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		after = func() { db.Create(&Event{}) }
		return nil
	})
	if err != nil {
		return err
	}
	after()
	return nil
}

// Not reported: the same, appended to a list of hooks, or kept in a field
// of a value declared outside.
type outbox struct{ hooks []func() }

func AfterCommitAll(db *gorm.DB, o *outbox, u *User) error {
	var hooks []func()
	err := db.Transaction(func(tx *gorm.DB) error {
		hooks = append(hooks, func() { db.Create(&Event{}) })
		o.hooks = append(o.hooks, func() { db.Create(&Event{}) }, func() { db.Create(u) })
		return tx.Create(u).Error
	})
	for _, h := range hooks {
		h()
	}
	return err
}

// A literal kept in a variable of the callback itself runs inside it.
func InsideHook(db *gorm.DB, u *User) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var now func()
		now = func() { db.Create(&Event{}) } // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		now()
		return tx.Create(u).Error
	})
}

type holder struct{ fn func() }

func pick(h *holder) *holder { return h }

// Not reported: a hook kept through a pointer or in an element of a value
// declared outside. A call's result, or a value that is not a function, is
// no hook.
func HookShapes(db *gorm.DB, p *func(), m map[string]func(), h *holder, u *User) error {
	var n int
	err := db.Transaction(func(tx *gorm.DB) error {
		*p = func() { db.Create(&Event{}) }
		m["after"] = func() { db.Create(&Event{}) }
		n = len(m)
		pick(h).fn = func() { db.Create(u) } // want `^db bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return tx.Create(u).Error
	})
	_ = n
	return err
}
