package main

import (
	"log"
	"os"
	"time"
)

func main() {
	dsn := getEnv("DB_DSN", "root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local")
	webhookDaemonURL := getEnv("WEBHOOK_DAEMON_URL", "http://webhook-daemon:9000")

	log.Printf("Webhook Orchestrator starting...")
	log.Printf("Database DSN: %s", dsn)
	log.Printf("Webhook Daemon URL: %s", webhookDaemonURL)

	// Placeholder: Will be implemented in Phase 3
	log.Println("Webhook orchestrator skeleton running (Phase 3 implementation pending)")

	// Keep the service alive
	for {
		time.Sleep(30 * time.Second)
		log.Println("Webhook orchestrator heartbeat")
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
