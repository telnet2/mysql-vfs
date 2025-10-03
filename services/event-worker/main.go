package main

import (
	"log"
	"os"
	"time"
)

func main() {
	dsn := getEnv("DB_DSN", "root:root@tcp(localhost:3306)/vfs?charset=utf8mb4&parseTime=True&loc=Local")
	workerConcurrency := getEnv("WORKER_CONCURRENCY", "10")
	pollInterval := getEnv("POLL_INTERVAL", "1s")

	log.Printf("Event Worker starting...")
	log.Printf("Database DSN: %s", dsn)
	log.Printf("Worker Concurrency: %s", workerConcurrency)
	log.Printf("Poll Interval: %s", pollInterval)

	// Placeholder: Will be implemented in Phase 3
	log.Println("Event worker skeleton running (Phase 3 implementation pending)")

	// Keep the service alive
	for {
		time.Sleep(30 * time.Second)
		log.Println("Event worker heartbeat")
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
