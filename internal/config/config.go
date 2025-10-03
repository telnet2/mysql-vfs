package config

import (
	"fmt"
	"time"
)

// Database contains configuration for connecting to the MySQL primary and read replicas.
type Database struct {
	Host            string        `json:"host" yaml:"host"`
	Port            int           `json:"port" yaml:"port"`
	User            string        `json:"user" yaml:"user"`
	Password        string        `json:"password" yaml:"password"`
	Name            string        `json:"name" yaml:"name"`
	MaxIdleConns    int           `json:"maxIdleConns" yaml:"maxIdleConns"`
	MaxOpenConns    int           `json:"maxOpenConns" yaml:"maxOpenConns"`
	ConnMaxLifetime time.Duration `json:"connMaxLifetime" yaml:"connMaxLifetime"`
}

// BlobStorage describes the configuration for the Go Cloud blob driver endpoint.
type BlobStorage struct {
	URL string `json:"url" yaml:"url"`
}

// Webhook contains defaults for webhook execution and retry handling.
type Webhook struct {
	Timeout       time.Duration `json:"timeout" yaml:"timeout"`
	RetryInterval time.Duration `json:"retryInterval" yaml:"retryInterval"`
	MaxRetries    int           `json:"maxRetries" yaml:"maxRetries"`
}

// CronScheduler describes distributed cron configuration.
type CronScheduler struct {
	PollInterval time.Duration `json:"pollInterval" yaml:"pollInterval"`
}

// Config bundles the application configuration options.
type Config struct {
	Database Database      `json:"database" yaml:"database"`
	Blob     BlobStorage   `json:"blob" yaml:"blob"`
	Webhook  Webhook       `json:"webhook" yaml:"webhook"`
	Cron     CronScheduler `json:"cron" yaml:"cron"`
	RootPath string        `json:"rootPath" yaml:"rootPath"`
	TLSCert  string        `json:"tlsCert" yaml:"tlsCert"`
	TLSKey   string        `json:"tlsKey" yaml:"tlsKey"`
}

// DSN renders the MySQL DSN for use with GORM.
func (d Database) DSN() string {
	if d.Port == 0 {
		d.Port = 3306
	}

	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local&charset=utf8mb4,utf8", d.User, d.Password, d.Host, d.Port, d.Name)
}
