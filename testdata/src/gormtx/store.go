package gormtx

import (
	"context"
	"database/sql"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ===== A struct that embeds the handle, among other things =====

// Repository embeds the handle beside fields and methods of its own.
type Repository struct {
	*gorm.DB
	logger *log.Logger
	cache  map[string]*User
}

func (r *Repository) validate(u *User) error { return nil }

// Only what reaches the embedded handle is reported.
func (r *Repository) Save(u *User) error {
	return r.Transaction(func(tx *gorm.DB) error {
		if err := r.validate(u); err != nil {
			return err
		}
		r.logger.Printf("saving %s", u.Name)
		r.cache[u.Name] = u
		r.Create(&AuditLog{}) // want `^r bypasses the transaction opened with Transaction; use tx instead \[outer-handle\]$`
		return tx.Create(u).Error
	})
}

// Locked embeds a mutex beside the handle.
type Locked struct {
	sync.Mutex
	*sql.DB
}

func (l *Locked) Do() error {
	tx, err := l.Begin()
	if err != nil {
		return err
	}
	l.Lock()
	defer l.Unlock()
	if _, err := tx.Exec("x"); err != nil {
		tx.Rollback()
		return err
	}
	l.Exec("y") // want `^l bypasses the transaction opened with Begin; use tx instead \[outer-handle\]$`
	return tx.Commit()
}

// Backend embeds two handles. The one the transaction was not opened on is
// another handle.
type Backend struct {
	*sql.DB
	*redis.Client
}

func (b *Backend) Do(ctx context.Context) error {
	tx, err := b.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("x"); err != nil {
		return err
	}
	b.Set(ctx, "k", "v", 0)
	return tx.Commit()
}
