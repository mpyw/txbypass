package gormtx

import "gorm.io/gorm"

// ===== A WithTx of one's own =====

type Metrics struct{ n int }

func (m *Metrics) Inc() { m.n++ }

// UserRepo has a WithTx, but holds more than the database.
type UserRepo struct {
	db      *gorm.DB
	metrics *Metrics
}

func (r *UserRepo) WithTx(tx *gorm.DB) *UserRepo { return &UserRepo{db: tx, metrics: r.metrics} }

func (r *UserRepo) Save(u *User) error { return r.db.Save(u).Error }

func (r *UserRepo) Validate(u *User) error { return nil }

type PlaceService struct {
	db   *gorm.DB
	repo *UserRepo
}

// Not reported: the repository holds more than the database, so its other
// members are no uses. sqlc's Queries holds only the database.
func (s *PlaceService) Place(u *User) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.repo.Validate(u); err != nil {
			return err
		}
		r := s.repo.WithTx(tx)
		if err := r.Save(u); err != nil {
			return err
		}
		s.repo.metrics.Inc()
		return nil
	})
}
