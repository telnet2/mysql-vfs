package db

import (
	"time"
)

type Directory struct {
	ID        string     `gorm:"primaryKey;size:36"`
	ParentID  *string    `gorm:"size:36;index"`
	Name      string     `gorm:"size:255;not null"`
	PathHash  string     `gorm:"size:64;not null;index:idx_directories_path_hash,unique"`
	Version   int64      `gorm:"not null"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime"`
	DeletedAt *time.Time `gorm:"index"`
}

type File struct {
	ID               string  `gorm:"primaryKey;size:36"`
	DirectoryID      string  `gorm:"size:36;index"`
	Name             string  `gorm:"size:255;not null"`
	CurrentVersionID string  `gorm:"size:36"`
	OriginFileID     *string `gorm:"size:36;index"`
	Version          int64   `gorm:"not null"`
	Checksum         *string `gorm:"size:128"`
	Size             *int64
	MimeType         *string    `gorm:"size:255"`
	CreatedAt        time.Time  `gorm:"autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime"`
	DeletedAt        *time.Time `gorm:"index"`
}

type FileVersion struct {
	ID             string    `gorm:"primaryKey;size:36"`
	FileID         string    `gorm:"size:36;index"`
	ContentPointer string    `gorm:"size:255"`
	MetadataJSON   string    `gorm:"type:json"`
	CreatedBy      string    `gorm:"size:128"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
}

type FileChunk struct {
	ChunkID     string `gorm:"primaryKey;size:36"`
	Sequence    int64  `gorm:"not null"`
	Content     []byte `gorm:"type:longblob"`
	Compression string `gorm:"size:32"`
	Checksum    string `gorm:"size:128"`
}

type FileRelation struct {
	ParentFileID string    `gorm:"primaryKey;size:36"`
	ChildFileID  string    `gorm:"primaryKey;size:36"`
	RelationType string    `gorm:"size:32;not null"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

type Event struct {
	ID            string    `gorm:"primaryKey;size:36"`
	Type          string    `gorm:"size:64;index"`
	SubjectID     string    `gorm:"size:36;index"`
	Payload       []byte    `gorm:"type:json"`
	Status        string    `gorm:"size:32;index"`
	RetryCount    int       `gorm:"not null"`
	NextAttemptAt time.Time `gorm:"index"`
	RequestID     string    `gorm:"size:64;index"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}

type WebhookConfig struct {
	ID         string    `gorm:"primaryKey;size:36"`
	ScopeType  string    `gorm:"size:32;index"`
	ScopeID    *string   `gorm:"size:36"`
	EventTypes string    `gorm:"type:text"`
	TargetURL  string    `gorm:"size:1024"`
	Secret     string    `gorm:"size:256"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

type WebhookJob struct {
	ID             string    `gorm:"primaryKey;size:36"`
	EventID        string    `gorm:"size:36;index"`
	ConfigID       string    `gorm:"size:36;index"`
	Payload        []byte    `gorm:"type:json"`
	Status         string    `gorm:"size:32;index"`
	RetryCount     int       `gorm:"not null"`
	LastError      *string   `gorm:"type:text"`
	NextAttemptAt  time.Time `gorm:"index"`
	IdempotencyKey string    `gorm:"size:128;uniqueIndex"`
	CreatedAt      time.Time `gorm:"autoCreateTime"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime"`
}

type CronJob struct {
	ID          string    `gorm:"primaryKey;size:36"`
	DirectoryID string    `gorm:"size:36;index"`
	CronExpr    string    `gorm:"size:64"`
	Payload     string    `gorm:"type:text"`
	Timezone    *string   `gorm:"size:64"`
	RequestID   string    `gorm:"size:64;index"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

type CronExecution struct {
	ID            string    `gorm:"primaryKey;size:36"`
	CronJobID     string    `gorm:"size:36;index"`
	ExecutionKey  string    `gorm:"size:128;uniqueIndex"`
	Status        string    `gorm:"size:32;index"`
	ResultPayload []byte    `gorm:"type:json"`
	ErrorMessage  *string   `gorm:"type:text"`
	StartedAt     time.Time `gorm:"autoCreateTime"`
	CompletedAt   *time.Time
}

type LeaseLock struct {
	Name      string    `gorm:"primaryKey;size:128"`
	OwnerID   string    `gorm:"size:64"`
	ExpiresAt time.Time `gorm:"index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
