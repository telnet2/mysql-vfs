package fixtures

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/telnet2/mysql-vfs/pkg/db"
	"github.com/telnet2/mysql-vfs/pkg/models"
)

// TestDatabase manages a MySQL test container
type TestDatabase struct {
	Container testcontainers.Container
	Host      string
	Port      int
	DBName    string
	DSN       string
	ctx       context.Context
}

// NewTestDatabase creates and starts a MySQL container for testing
func NewTestDatabase() *TestDatabase {
	ctx := context.Background()

	// Generate unique database name for parallel test execution
	dbName := fmt.Sprintf("test_%s", uuid.New().String()[:8])

	req := testcontainers.ContainerRequest{
		Image:        "mysql:8.0",
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": "testpass",
			"MYSQL_DATABASE":      dbName,
		},
	}
	req.WaitingFor = wait.ForLog("ready for connections").
		WithOccurrence(2).
		WithStartupTimeout(60 * time.Second)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	Expect(err).NotTo(HaveOccurred())

	host, err := container.Host(ctx)
	Expect(err).NotTo(HaveOccurred())

	mappedPort, err := container.MappedPort(ctx, "3306")
	Expect(err).NotTo(HaveOccurred())

	port := mappedPort.Int()
	dsn := fmt.Sprintf("root:testpass@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		host, port, dbName)

	testDB := &TestDatabase{
		Container: container,
		Host:      host,
		Port:      port,
		DBName:    dbName,
		DSN:       dsn,
		ctx:       ctx,
	}

	// Wait for MySQL to be fully ready
	time.Sleep(3 * time.Second)

	// Verify connection works
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		testGormDB, err := db.Connect(db.Config{
			DSN:      dsn,
			LogLevel: logger.Silent,
		})
		if err == nil {
			sqlDB, _ := testGormDB.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
			break
		}
		if i == maxRetries-1 {
			Expect(err).NotTo(HaveOccurred(), "Failed to connect after retries")
		}
		time.Sleep(1 * time.Second)
	}

	// Run migrations
	testDB.Migrate()

	return testDB
}

// Migrate runs database migrations
func (td *TestDatabase) Migrate() {
	gormDB, err := db.Connect(db.Config{
		DSN:      td.DSN,
		LogLevel: logger.Silent,
	})
	Expect(err).NotTo(HaveOccurred())

	// Run migrations explicitly
	err = db.AutoMigrate(gormDB)
	Expect(err).NotTo(HaveOccurred())

	sqlDB, err := gormDB.DB()
	Expect(err).NotTo(HaveOccurred())
	sqlDB.Close()
}

// GetDB returns a GORM database connection
func (td *TestDatabase) GetDB() *gorm.DB {
	gormDB, err := db.Connect(db.Config{
		DSN:      td.DSN,
		LogLevel: logger.Silent,
	})
	Expect(err).NotTo(HaveOccurred())
	return gormDB
}

// Cleanup terminates the test container
func (td *TestDatabase) Cleanup() {
	if td.Container != nil {
		_ = td.Container.Terminate(td.ctx)
	}
}

// Reset clears all tables for test isolation
func (td *TestDatabase) Reset() {
	gormDB := td.GetDB()
	defer func() {
		sqlDB, _ := gormDB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}()

	// Truncate all tables in reverse order of dependencies
	tables := []string{
		"cron_executions",
		"cron_jobs",
		"webhook_deliveries",
		"webhooks",
		"events",
		"file_relations",
		"file_versions",
		"files",
		"directories",
		"idempotency_records",
		"opa_policies",
	}

	for _, table := range tables {
		gormDB.Exec(fmt.Sprintf("TRUNCATE TABLE %s", table))
	}
}

// GetIdempotencyRecord retrieves an idempotency record by request ID
func GetIdempotencyRecord(db *TestDatabase, requestID string) *models.IdempotencyRecord {
	gormDB := db.GetDB()
	defer func() {
		sqlDB, _ := gormDB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}()

	var record models.IdempotencyRecord
	err := gormDB.Where("request_id = ?", requestID).First(&record).Error
	if err != nil {
		return nil
	}
	return &record
}
