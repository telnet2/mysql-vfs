package db

import "gorm.io/gorm"

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Directory{},
		&File{},
		&FileVersion{},
		&FileChunk{},
		&FileRelation{},
		&Event{},
		&WebhookConfig{},
		&WebhookJob{},
		&CronJob{},
		&CronExecution{},
		&LeaseLock{},
	)
}
