package gorm

import (
	"gorm.io/gorm"
)

// GormTransaction implements the Transaction interface using GORM
type GormTransaction struct {
	tx *gorm.DB
}

// NewGormTransaction creates a new GORM transaction wrapper
func NewGormTransaction(tx *gorm.DB) *GormTransaction {
	return &GormTransaction{tx: tx}
}

// Commit commits the transaction
func (t *GormTransaction) Commit() error {
	return t.tx.Commit().Error
}

// Rollback rolls back the transaction
func (t *GormTransaction) Rollback() error {
	return t.tx.Rollback().Error
}

// GetDB returns the underlying GORM DB
func (t *GormTransaction) GetDB() interface{} {
	return t.tx
}
