package fs

import (
	"context"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// TransactionManager coordinates multi-operation transactions with savepoint support.
type TransactionManager struct {
	mu sync.Mutex
}

// WithTransaction executes the provided function inside a GORM transaction.
func (m *TransactionManager) WithTransaction(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// Savepoint creates a database savepoint.
func Savepoint(tx *gorm.DB, name string) error {
	return tx.Exec(fmt.Sprintf("SAVEPOINT %s", name)).Error
}

// RollbackTo rolls the transaction back to a savepoint.
func RollbackTo(tx *gorm.DB, name string) error {
	return tx.Exec(fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", name)).Error
}
