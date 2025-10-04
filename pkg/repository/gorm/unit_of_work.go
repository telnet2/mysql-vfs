package gorm

import (
	"context"

	"github.com/telnet2/mysql-vfs/pkg/repository"
	"gorm.io/gorm"
)

// GormUnitOfWork implements the UnitOfWork interface using GORM
type GormUnitOfWork struct {
	db        *gorm.DB
	dirRepo   *GormDirectoryRepository
	fileRepo  *GormFileRepository
	eventRepo *GormEventRepository
}

// NewGormUnitOfWork creates a new GORM unit of work
func NewGormUnitOfWork(db *gorm.DB) *GormUnitOfWork {
	return &GormUnitOfWork{
		db:        db,
		dirRepo:   NewGormDirectoryRepository(db),
		fileRepo:  NewGormFileRepository(db),
		eventRepo: NewGormEventRepository(db),
	}
}

// BeginTransaction starts a new transaction
func (uow *GormUnitOfWork) BeginTransaction(ctx context.Context) (repository.Transaction, error) {
	tx := uow.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	return NewGormTransaction(tx), nil
}

// Directories returns the directory repository
func (uow *GormUnitOfWork) Directories() repository.DirectoryRepository {
	return uow.dirRepo
}

// Files returns the file repository
func (uow *GormUnitOfWork) Files() repository.FileRepository {
	return uow.fileRepo
}

// Events returns the event repository
func (uow *GormUnitOfWork) Events() repository.EventRepository {
	return uow.eventRepo
}
