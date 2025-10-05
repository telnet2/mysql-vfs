package mysql

import (
	"context"

	"github.com/telnet2/mysql-vfs/pkg/persistence/db"
	"github.com/telnet2/mysql-vfs/pkg/persistence/storage"
	"gorm.io/gorm"
)

// GormUnitOfWork implements the UnitOfWork interface using GORM
type GormUnitOfWork struct {
	db        *gorm.DB
	storage   storage.Storage
	dirRepo   *GormDirectoryRepository
	fileRepo  *GormFileRepository
	eventRepo *GormEventRepository
}

// NewGormUnitOfWork creates a new GORM unit of work
func NewGormUnitOfWork(db *gorm.DB, storage storage.Storage) *GormUnitOfWork {
	return &GormUnitOfWork{
		db:        db,
		storage:   storage,
		dirRepo:   NewGormDirectoryRepository(db),
		fileRepo:  NewGormFileRepository(db, storage),
		eventRepo: NewGormEventRepository(db),
	}
}

// BeginTransaction starts a new transaction
func (uow *GormUnitOfWork) BeginTransaction(ctx context.Context) (db.Transaction, error) {
	tx := uow.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return NewGormTransaction(tx), nil
}

// Directories returns the directory repository
func (uow *GormUnitOfWork) Directories() db.DirectoryRepository {
	return uow.dirRepo
}

// Files returns the file repository
func (uow *GormUnitOfWork) Files() db.FileRepository {
	return uow.fileRepo
}

// Events returns the event repository
func (uow *GormUnitOfWork) Events() db.EventRepository {
	return uow.eventRepo
}
