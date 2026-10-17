// Package repo is a repository in the module, beside the service that
// uses it.
package repo

import "gorm.io/gorm"

type Repo struct{ DB *gorm.DB }

// Bind rebinds the repository to a transaction.
func (r *Repo) Bind(tx *gorm.DB) { r.DB = tx }
