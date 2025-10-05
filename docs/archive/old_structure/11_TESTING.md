# 11. Testing Guide

**MySQL VFS v2.1+ Testing Strategy and Best Practices**

[← Back: API Reference](10_API.md) | [Index](0_README.md) | [Next: Development →](12_DEVELOPMENT.md)

---

## Current Status

**Test Count:** 103/104 passing
**Known Issues:** 1 flaky concurrency test
**Test Location:** `citest/` directory
**Coverage:** Critical paths (file/directory operations, auth, events, webhooks)

This guide describes the testing strategy, tools, and best practices for the MySQL-based Distributed VFS project.

## Testing Philosophy

**Behavioral Testing Over Unit Testing**: We prioritize end-to-end behavioral tests that validate complete workflows over isolated unit tests. This approach:
- Tests the system as users interact with it
- Validates integration between components
- Catches real-world issues earlier
- Provides better documentation of system behavior
- Reduces test brittleness from implementation changes

**When to Write Different Test Types**:
- **E2E Behavioral Tests (Primary)**: Complete workflows across services
- **Integration Tests**: Component interactions (DB, S3, webhooks)
- **Unit Tests (Minimal)**: Complex algorithms, edge cases in pure functions

## Testing Stack

### Core Frameworks

**Ginkgo v2** (`github.com/onsi/ginkgo/v2`): BDD testing framework
- Expressive `Describe`/`Context`/`It` structure
- Built-in parallel execution support
- Rich matchers via Gomega
- Table-driven tests with `DescribeTable`

**Gomega** (`github.com/onsi/gomega`): Assertion library
- Readable assertions: `Expect(result).To(Equal(expected))`
- Async testing: `Eventually()` and `Consistently()`
- Rich matcher library

**httpexpect v2** (`github.com/gavv/httpexpect/v2`): HTTP API testing
- Fluent API for building and validating requests
- JSON path support
- Status code and header validation
- Request/response chaining

**Testcontainers Go** (`github.com/testcontainers/testcontainers-go`): Container management
- Ephemeral MySQL instances per test suite
- Random port allocation for parallel execution
- Automatic cleanup on test completion
- Real database testing without mocking

## Directory Structure

```
.
├── citest/                          # Integration tests (parallel-safe)
│   ├── fixtures/                    # Test data and helpers
│   │   ├── db.go                    # Database test helpers
│   │   ├── s3.go                    # S3 test helpers
│   │   └── server.go                # Service startup helpers
│   ├── vfs_files_test.go            # File operations E2E tests
│   ├── vfs_directories_test.go      # Directory operations E2E tests
│   ├── vfs_edge_cases_test.go       # Edge case tests
│   ├── e2e_workflow_test.go         # Complete workflow tests
│   ├── file_based_auth_test.go      # File-based auth (.user) tests
│   ├── auth_login_test.go           # Authentication tests
│   ├── opa_integration_test.go      # OPA policy E2E tests
│   ├── concurrency_test.go          # Concurrent operation tests (1 flaky)
│   ├── schema_validation_test.go    # Validation tests
│   ├── integrity_test.go            # Data integrity tests
│   └── citest_suite_test.go         # Ginkgo suite setup
├── pkg/                             # Unit tests (when needed)
│   ├── domain/
│   │   ├── file_service_lifecycle_test.go
│   │   ├── directory_service_lifecycle_test.go
│   │   ├── user_loader_test.go
│   │   ├── policy_loader_test.go
│   │   ├── owner_loader_test.go
│   │   ├── group_loader_test.go
│   │   ├── protection_test.go
│   │   └── veto_integration_test.go
│   └── events/handlers/
│       ├── webhook_test.go
│       ├── log_test.go
│       └── metrics_test.go
└── docs/
    └── 11_TESTING.md                # This file
```

## Test Organization

### Suite Structure

Each test file follows the Ginkgo BDD structure:

```go
package citest

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/gavv/httpexpect/v2"
)

var _ = Describe("VFS Directory Operations", func() {
    var (
        api    *httpexpect.Expect
        testDB *TestDatabase
    )

    BeforeEach(func() {
        // Setup: Start services, create test fixtures
        testDB = NewTestDatabase()
        api = NewAPIClient(testDB.Port())
    })

    AfterEach(func() {
        // Teardown: Cleanup resources
        testDB.Cleanup()
    })

    Context("when creating directories", func() {
        It("should create a new directory under root", func() {
            api.POST("/api/v1/directories").
                WithJSON(map[string]interface{}{
                    "name": "projects",
                    "parent_id": "root",
                }).
                Expect().
                Status(201).
                JSON().Object().
                ContainsKey("id").
                Value("name").Equal("projects")
        })

        It("should reject duplicate directory names", func() {
            // Create first directory
            api.POST("/api/v1/directories").
                WithJSON(map[string]interface{}{
                    "name": "projects",
                    "parent_id": "root",
                }).
                Expect().
                Status(201)

            // Attempt duplicate
            api.POST("/api/v1/directories").
                WithJSON(map[string]interface{}{
                    "name": "projects",
                    "parent_id": "root",
                }).
                Expect().
                Status(409)
        })
    })

    Context("when moving directories", func() {
        It("should prevent circular parent relationships", func() {
            // Create hierarchy: /a/b/c
            a := createDirectory(api, "a", "root")
            b := createDirectory(api, "b", a.ID)
            c := createDirectory(api, "c", b.ID)

            // Try to move /a under /a/b/c (circular)
            api.PATCH("/api/v1/directories/" + a.ID).
                WithJSON(map[string]interface{}{
                    "parent_id": c.ID,
                }).
                Expect().
                Status(400).
                JSON().Object().
                Value("error").String().Contains("circular")
        })
    })
})
```

### Parallel Execution

**Random Port Allocation**: Each test suite gets a unique MySQL port
```go
func NewTestDatabase() *TestDatabase {
    ctx := context.Background()

    req := testcontainers.ContainerRequest{
        Image:        "mysql:8.0",
        ExposedPorts: []string{"3306/tcp"},  // Maps to random host port
        Env: map[string]string{
            "MYSQL_ROOT_PASSWORD": "testpass",
            "MYSQL_DATABASE":      fmt.Sprintf("test_%s", uuid.New().String()[:8]),
        },
        WaitStrategy: wait.ForLog("ready for connections").WithStartupTimeout(60 * time.Second),
    }

    container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: req,
        Started:          true,
    })
    Expect(err).NotTo(HaveOccurred())

    port, err := container.MappedPort(ctx, "3306")
    Expect(err).NotTo(HaveOccurred())

    return &TestDatabase{
        Container: container,
        Port:      port.Int(),
        DBName:    req.Env["MYSQL_DATABASE"],
    }
}
```

**Run Tests in Parallel**:
```bash
# Run all integration tests with parallel execution
ginkgo -r -p --randomize-all citest/

# Run with verbose output
ginkgo -r -p -v citest/

# Run specific test
ginkgo -r --focus="Directory Operations" citest/
```

## Testing Patterns

### Pattern 1: Complete Workflow Tests

Test entire user journeys, not isolated functions:

```go
Describe("File Upload and Download Workflow", func() {
    It("should upload, version, and download files correctly", func() {
        // 1. Create directory
        dir := createDirectory(api, "documents", "root")

        // 2. Upload file
        file := api.POST("/api/v1/files").
            WithMultipart().
            WithFile("file", "testdata/sample.pdf").
            WithFormField("name", "report.pdf").
            WithFormField("directory_id", dir.ID).
            Expect().
            Status(201).
            JSON().Object()

        fileID := file.Value("id").String().Raw()

        // 3. Verify file metadata
        api.GET("/api/v1/files/" + fileID).
            Expect().
            Status(200).
            JSON().Object().
            Value("name").Equal("report.pdf").
            Value("version").Equal(1)

        // 4. Upload new version
        api.POST("/api/v1/files/" + fileID + "/versions").
            WithMultipart().
            WithFile("file", "testdata/sample_v2.pdf").
            Expect().
            Status(201).
            JSON().Object().
            Value("version").Equal(2)

        // 5. Download specific version
        content := api.GET("/api/v1/files/" + fileID + "/download").
            WithQuery("version", 2).
            Expect().
            Status(200).
            Header("Content-Type").Equal("application/pdf").
            Body().Raw()

        Expect(content).NotTo(BeEmpty())
    })
})
```

### Pattern 2: Idempotency Testing

```go
Describe("Idempotency", func() {
    It("should return identical results for duplicate requests", func() {
        requestID := uuid.New().String()

        createRequest := func() *httpexpect.Response {
            return api.POST("/api/v1/directories").
                WithHeader("X-Request-ID", requestID).
                WithJSON(map[string]interface{}{
                    "name": "projects",
                    "parent_id": "root",
                }).
                Expect()
        }

        // First request
        resp1 := createRequest().Status(201).JSON().Object()
        dirID1 := resp1.Value("id").String().Raw()

        // Duplicate request (same X-Request-ID)
        resp2 := createRequest().Status(201).JSON().Object()
        dirID2 := resp2.Value("id").String().Raw()

        // Should return same directory ID
        Expect(dirID1).To(Equal(dirID2))

        // Verify only one directory exists
        api.GET("/api/v1/directories").
            Expect().
            Status(200).
            JSON().Array().
            Length().Equal(2)  // root + projects
    })
})
```

### Pattern 3: Concurrent Operations

```go
Describe("Concurrent File Updates", func() {
    It("should handle optimistic locking correctly", func() {
        // Create file
        fileID := createFile(api, "data.json", "root")

        // Fetch current version
        file := api.GET("/api/v1/files/" + fileID).
            Expect().
            Status(200).
            JSON().Object()

        version := file.Value("version").Number().Raw()

        // Concurrent updates with same expected_version
        results := make(chan int, 2)

        updateFile := func() {
            resp := api.PATCH("/api/v1/files/" + fileID).
                WithJSON(map[string]interface{}{
                    "expected_version": version,
                    "name": "updated.json",
                }).
                Expect().
                Raw()

            results <- resp.StatusCode
        }

        go updateFile()
        go updateFile()

        status1 := <-results
        status2 := <-results

        // One succeeds (200), one fails (409 Conflict)
        statuses := []int{status1, status2}
        Expect(statuses).To(ContainElement(200))
        Expect(statuses).To(ContainElement(409))
    })
})
```

### Pattern 4: Event and Webhook Testing

```go
Describe("Webhook Delivery", func() {
    var webhookServer *httptest.Server
    var receivedEvents []map[string]interface{}

    BeforeEach(func() {
        receivedEvents = []map[string]interface{}{}
        webhookServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            var event map[string]interface{}
            json.NewDecoder(r.Body).Decode(&event)
            receivedEvents = append(receivedEvents, event)
            w.WriteHeader(200)
        }))
    })

    AfterEach(func() {
        webhookServer.Close()
    })

    It("should deliver events to registered webhooks", func() {
        // Register webhook
        api.POST("/api/v1/webhooks").
            WithJSON(map[string]interface{}{
                "url": webhookServer.URL,
                "events": []string{"file.created"},
            }).
            Expect().
            Status(201)

        // Trigger event
        api.POST("/api/v1/files").
            WithMultipart().
            WithFile("file", "testdata/sample.txt").
            WithFormField("name", "test.txt").
            WithFormField("directory_id", "root").
            Expect().
            Status(201)

        // Wait for webhook delivery
        Eventually(func() int {
            return len(receivedEvents)
        }, 5*time.Second).Should(Equal(1))

        Expect(receivedEvents[0]["event_type"]).To(Equal("file.created"))
    })
})
```

### Pattern 5: Scheduler Testing

```go
Describe("Cron Scheduler", func() {
    It("should execute scheduled jobs", func() {
        // Create cron job (every minute)
        jobID := api.POST("/api/v1/cron-jobs").
            WithJSON(map[string]interface{}{
                "name": "cleanup_test",
                "cron_expression": "* * * * *",
                "handler_type": "cleanup_idempotency",
            }).
            Expect().
            Status(201).
            JSON().Object().
            Value("id").String().Raw()

        // Wait for execution
        Eventually(func() int {
            executions := api.GET("/api/v1/cron-jobs/" + jobID + "/executions").
                Expect().
                Status(200).
                JSON().Array()

            return len(executions.Raw())
        }, 90*time.Second, 5*time.Second).Should(BeNumerically(">=", 1))

        // Verify execution status
        api.GET("/api/v1/cron-jobs/" + jobID + "/executions").
            Expect().
            Status(200).
            JSON().Array().
            First().Object().
            Value("status").Equal("completed")
    })
})
```

## Test Fixtures

### Database Helper

```go
// citest/fixtures/db.go
package fixtures

type TestDatabase struct {
    Container testcontainers.Container
    Port      int
    DBName    string
    DSN       string
}

func NewTestDatabase() *TestDatabase {
    // Implementation shown above
}

func (td *TestDatabase) Cleanup() {
    td.Container.Terminate(context.Background())
}

func (td *TestDatabase) GetDB() *gorm.DB {
    db, err := db.Connect(db.Config{
        DSN: td.DSN,
        LogLevel: logger.Silent,
    })
    Expect(err).NotTo(HaveOccurred())
    return db
}
```

### S3 Helper

```go
// citest/fixtures/s3.go
package fixtures

func NewTestS3(port int) *TestS3 {
    // Start LocalStack container with S3
    // Return helper for S3 operations
}
```

### Server Helper

```go
// citest/fixtures/server.go
package fixtures

func StartVFSService(dbDSN string, s3Endpoint string) *TestServer {
    // Start VFS service on random port
    // Return API client
}
```

## Running Tests

### Local Development

```bash
# Install Ginkgo CLI
go install github.com/onsi/ginkgo/v2/ginkgo@latest

# Run all integration tests
ginkgo -r -p citest/

# Run with coverage
ginkgo -r -p --cover --coverprofile=coverage.out citest/

# Run specific suite
ginkgo -r --focus="VFS Directory Operations" citest/

# Run with race detector
ginkgo -r -p --race citest/
```

## Best Practices

### DO

✅ **Test complete workflows**: Upload → Download → Delete
✅ **Use real dependencies**: Real MySQL, real S3 (LocalStack)
✅ **Test concurrent scenarios**: Multiple users, race conditions
✅ **Validate edge cases**: Circular references, orphaned records
✅ **Test idempotency**: Duplicate requests, retry logic
✅ **Use descriptive names**: `It("should prevent circular parent relationships")`
✅ **Clean up resources**: Use `AfterEach` for teardown
✅ **Use Eventually()**: For async operations (webhooks, events)

### DON'T

❌ **Don't mock database**: Use testcontainers for real MySQL
❌ **Don't test implementation details**: Test behavior, not internals
❌ **Don't share state**: Each test should be independent
❌ **Don't hardcode ports**: Use random port allocation
❌ **Don't skip cleanup**: Always terminate containers
❌ **Don't write brittle tests**: Avoid testing exact error messages
❌ **Don't over-test pure functions**: Focus on integration points

## Coverage Goals

- **Critical Paths**: 100% (file upload/download, directory operations)
- **Business Logic**: 90% (versioning, OPA, idempotency)
- **Error Handling**: 80% (validation, conflicts)
- **Edge Cases**: 70% (circular refs, orphans)

## Troubleshooting

### Tests Hang

Check for:
- Container startup timeout (increase wait strategy timeout)
- Resource leaks (ensure `Cleanup()` called in `AfterEach`)
- Deadlocks (enable race detector with `--race`)

### Port Conflicts

- Testcontainers automatically allocates random ports
- If issues persist, check for leaked containers: `docker ps -a`
- Clean up: `docker rm -f $(docker ps -aq)`

### Flaky Tests

- Use `Eventually()` instead of `time.Sleep()`
- Increase timeout for async operations
- Check for race conditions with `--race` flag
- Ensure test isolation (no shared state)

## Key Test Files

### Core VFS Tests (✅ Complete)

1. **citest/vfs_files_test.go** - File operations (create, read, update, delete)
2. **citest/vfs_directories_test.go** - Directory operations and hierarchy
3. **citest/vfs_edge_cases_test.go** - Edge cases and error handling
4. **citest/e2e_workflow_test.go** - End-to-end workflows

### Authentication & Authorization (✅ Complete)

5. **citest/file_based_auth_test.go** - File-based auth with `.user` files
6. **citest/auth_login_test.go** - Login and token validation
7. **citest/opa_integration_test.go** - OPA policy evaluation
8. **citest/directory_access_test.go** - Directory-level access control

### Advanced Features (✅ Complete)

9. **citest/concurrency_test.go** - Concurrent operations (1 flaky test)
10. **citest/schema_validation_test.go** - Data validation
11. **citest/integrity_test.go** - Data integrity checks

### Domain Layer Tests (✅ Complete)

12. **pkg/domain/user_loader_test.go** - User loading from `.user` files
13. **pkg/domain/policy_loader_test.go** - Policy loading from `.rego` files
14. **pkg/domain/owner_loader_test.go** - Owner loading from `.owner` files
15. **pkg/domain/protection_test.go** - Resource protection logic
16. **pkg/domain/veto_integration_test.go** - Event veto mechanism

### Event System Tests (✅ Complete)

17. **pkg/events/handlers/webhook_test.go** - Webhook event handlers
18. **pkg/events/handlers/log_test.go** - Log event handlers
19. **pkg/events/handlers/metrics_test.go** - Metrics event handlers

---

[← Back: API Reference](10_API.md) | [Index](0_README.md) | [Next: Development →](12_DEVELOPMENT.md)
