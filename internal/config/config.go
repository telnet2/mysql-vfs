package config

import (
	"os"
	"strconv"
)

type MySQL struct {
	User     string
	Password string
	Host     string
	Port     int
	Database string
	Params   string
}

type Server struct {
	Address string
}

type Webhook struct {
	CallbackSecret string
	BaseURL        string
}

type Settings struct {
	Server  Server
	MySQL   MySQL
	Webhook Webhook
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Settings {
	port, _ := strconv.Atoi(getenv("MYSQL_PORT", "3306"))
	return Settings{
		Server: Server{Address: getenv("SERVER_ADDR", ":8080")},
		MySQL: MySQL{
			User:     getenv("MYSQL_USER", "root"),
			Password: getenv("MYSQL_PASSWORD", ""),
			Host:     getenv("MYSQL_HOST", "mysql"),
			Port:     port,
			Database: getenv("MYSQL_DATABASE", "vfs"),
			Params:   getenv("MYSQL_PARAMS", "charset=utf8mb4&parseTime=True&loc=Local"),
		},
		Webhook: Webhook{
			CallbackSecret: getenv("WEBHOOK_SECRET", ""),
			BaseURL:        getenv("WEBHOOK_BASE_URL", ""),
		},
	}
}
