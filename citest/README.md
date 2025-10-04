## CITest Architecture

### Overview
The `citest` package hosts the end-to-end suite that boots real service binaries against ephemeral infrastructure to validate policy enforcement and content storage flows. Tests are written with Ginkgo/Gomega and exercise the HTTP APIs via `httpexpect`.

### Infrastructure Lifecycle
- **Dockerized dependencies**: MySQL and LocalStack (S3) are provisioned with `testcontainers-go`, ensuring isolated, reproducible backing services per run.
- **Configuration**: Each run generates a temporary YAML config that points services at the freshly created containers and S3 bucket.
- **Binary execution**: `gexec.Build` compiles the metadata and content services, and the suite runs the resulting binaries directly so signals (SIGTERM/SIGKILL) land on the actual servers, enabling fast teardown.

### Test Flow
1. Start containers, wait for readiness, and create the S3 bucket used during tests.
2. Launch metadata and content binaries with the generated configuration and poll their `/ping` endpoints until healthy.
3. Execute specs (e.g., policy enforcement) by exercising the services through `httpexpect`.
4. On teardown, terminate the services, escalate to SIGKILL only if needed, and remove temporary resources.

### Test Suites

#### Core Functionality
- **filesystem_test.go** - Virtual filesystem end-to-end workflows including file operations, storage modes (inline_json, s3_blob), concurrent updates, and cleanup operations
- **policy_enforcement_test.go** - OPA policy enforcement (.rego files), JSON schema validation (.jsonschema files), admin-only operations, and policy-driven access control

#### Storage & Content
- **storage_test.go** - Storage mode selection, inline JSON for small files, S3 blob storage for large files, MIME type handling, checksum integrity verification, storage mode transitions, and binary data patterns

#### Version Management
- **versioning_test.go** - File version tracking, version history, rapid version updates, concurrent version updates, and version integrity during storage mode transitions

#### Directory Operations
- **directory_test.go** - Hierarchical directory structures, parent-child relationships, non-empty directory deletion prevention, concurrent directory operations, and directory name updates

#### Events & Webhooks
- **events_test.go** - Event generation for file/directory lifecycle (created, updated, deleted), event triggers with custom actions, event batching for rapid operations, event ordering for version updates, and storage transition events

#### Error Handling
- **error_scenarios_test.go** - Duplicate file name handling, invalid directory references, file name constraint validation, checksum mismatch detection, missing required field validation, invalid JSON payloads, blob storage reference validation, concurrent update conflicts, and non-existent resource operations

### Extending the Suite
- Add scenario-focused specs under `citest` and keep them self-contained.
- Reuse helper functions in `setup_test.go` for HTTP interactions and session management.
- Run `go test -count=1 ./citest -run TestCITest` locally to verify new specs against fresh infrastructure each time.
- Follow existing patterns: use `Ordered` for specs requiring setup/teardown, use `BeforeAll`/`AfterAll` for test fixtures, and ensure proper cleanup in `AfterAll` blocks.

### Running Tests
```bash
# Run all e2e tests
go test -v ./citest

# Run specific test suite
go test -v ./citest -run TestCITest/filesystem
go test -v ./citest -run TestCITest/policy
go test -v ./citest -run TestCITest/versioning

# Run with fresh infrastructure each time
go test -count=1 ./citest

# Verbose output with timing
go test -v -count=1 ./citest
```

### Coverage Areas
- ✅ File CRUD operations (create, read, update, delete)
- ✅ Directory hierarchy and relationships
- ✅ Storage mode selection (inline_json vs s3_blob)
- ✅ Version tracking and history
- ✅ OPA policy enforcement
- ✅ JSON schema validation
- ✅ Event generation and processing
- ✅ Concurrent operations and race conditions
- ✅ Error scenarios and edge cases
- ✅ Checksum integrity
- ✅ MIME type handling
- ✅ Binary data storage
