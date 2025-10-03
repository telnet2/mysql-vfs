package db

import (
	"fmt"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Config struct {
	User               string
	Password           string
	Host               string
	Port               int
	Database           string
	Params             string
	DisableAutoMigrate bool
}

func (c Config) dsn() string {
	params := c.Params
	if params == "" {
		params = "charset=utf8mb4&parseTime=True&loc=Local"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s", c.User, c.Password, c.Host, c.Port, c.Database, params)
}

func NewConnection(cfg Config) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.dsn()), &gorm.Config{Logger: logger.Default.LogMode(logger.Info)})
	if err != nil {
		return nil, err
	}
	if !cfg.DisableAutoMigrate {
		if err := AutoMigrate(db); err != nil {
			return nil, err
		}
	}
	return db, nil
}
