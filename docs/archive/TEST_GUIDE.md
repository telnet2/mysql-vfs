# Testing Guide

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
│   ├── vfs_test.go                  # VFS service E2E tests
│   ├── events_test.go               # Event system E2E tests
│   ├── webhooks_test.go             # Webhook delivery E2E tests
│   ├── scheduler_test.go            # Scheduler E2E tests
│   ├── idempotency_test.go          # Idempotency E2E tests
│   ├── opa_test.go                  # OPA policy E2E tests
│   └── citest_suite_test.go         # Ginkgo suite setup
├── pkg/                             # Unit tests (when needed)
│   ├── idempotency/
│   │   └── middleware_test.go
│   └── services/
│       └── tree_lock_test.go        # Complex algorithm tests
└── docs/
    └── TEST_GUIDE.md                # This file
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

### CI/CD Pipeline

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3

      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install Ginkgo
        run: go install github.com/onsi/ginkgo/v2/ginkgo@latest

      - name: Download dependencies
        run: go mod download

      - name: Run tests
        run: ginkgo -r -p --randomize-all --race --cover citest/

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
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

## Next Steps

Phase 6 Test Implementation Order:

1. **VFS Core Tests** (`citest/vfs_test.go`)
   - Directory CRUD operations
   - File upload/download
   - Versioning and tree locking

2. **Idempotency Tests** (`citest/idempotency_test.go`)
   - Duplicate request handling
   - TTL expiration

3. **Event System Tests** (`citest/events_test.go`)
   - Transactional outbox
   - Event processing

4. **Webhook Tests** (`citest/webhooks_test.go`)
   - Delivery and retries
   - Circuit breaker

5. **Scheduler Tests** (`citest/scheduler_test.go`)
   - Cron execution
   - Lease management

6. **OPA Tests** (`citest/opa_test.go`)
   - Policy evaluation
   - Access control

7. **Load Tests** (`citest/load_test.go`)
   - Concurrent operations
   - Performance benchmarks
