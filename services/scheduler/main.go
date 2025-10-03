package main

import (
	"log"
	"os"
	"time"
)

func main() {
	dsn := getEnv("DB_DSN", "root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local")
	schedulerID := getEnv("SCHEDULER_ID", "scheduler-1")
	pollInterval := getEnv("POLL_INTERVAL", "10s")

	log.Printf("Scheduler starting...")
	log.Printf("Database DSN: %s", dsn)
	log.Printf("Scheduler ID: %s", schedulerID)
	log.Printf("Poll Interval: %s", pollInterval)

	// Placeholder: Will be implemented in Phase 4
	log.Println("Scheduler skeleton running (Phase 4 implementation pending)")

	// Keep the service alive
	for {
		time.Sleep(30 * time.Second)
		log.Println("Scheduler heartbeat")
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
