# Workflow Integration Tests - Usage Guide

This document describes how to run the workflow integration tests using Ginkgo.

## Prerequisites

- Ginkgo v2.26.0+ installed
- Docker running (for MySQL and LocalStack)
- Go 1.25+

## Test Suite Overview

The workflow integration test suite includes **2 test files**:

### File 1: `e2e_workflow_simple_test.go` (✅ WORKING)
Simple, positive workflow tests that demonstrate core functionality:

**Test 1: Basic Document Workflow**
Tests basic state transitions: draft → review → final
- Creates workflow configuration
- Creates file in draft state
- Moves through states sequentially
- Verifies file integrity

**Test 2: Workflow with Subdirectories**
Validates subdirectory structure preservation during transitions
- Creates nested directory structure
- Moves file between states while preserving path structure
- Tests deep directory hierarchies

**Test 3: Multiple Files Workflow**
Handles batch file transitions through workflow states
- Creates multiple files in one state
- Archives them one by one
- Validates each transition

### File 2: `e2e_workflow_integration_test.go` (⚠️ NEEDS UPDATES)
More complex workflow tests including negative cases. These tests include 7 scenarios but may need user context setup to work properly:

### Test 1: Document Approval Workflow
Tests basic state transitions: draft → review → published

### Test 2: Escape Prevention
Verifies files cannot be moved outside workflow scope

### Test 3: Deletion Gates
Ensures gates are enforced for file deletion operations

### Test 4: State Directory Protection
Confirms state directories cannot be deleted while workflow is active

### Test 5: Subdirectory Structure Preservation
Validates that subdirectory structures are preserved during transitions

### Test 6: System Admin Bypass
Tests that system-admin group can bypass workflow gates

### Test 7: Same-State Movement
Verifies files can be moved within the same state directory

## Running Tests

### Run All Working Workflow Tests (RECOMMENDED)

```bash
cd citest
ginkgo -v --silence-skips --focus="Simple Workflow Integration"
```

### Run Specific Simple Workflow Tests

```bash
cd citest

# Test 1: Basic Document Workflow (draft → review → final)
ginkgo -v --silence-skips --focus="Basic Document Workflow"

# Test 2: Subdirectory Structure Preservation
ginkgo -v --silence-skips --focus="Workflow with Subdirectories"

# Test 3: Multiple Files Workflow
ginkgo -v --silence-skips --focus="Multiple Files Workflow"
```

### Run Advanced Tests (May Need Fixes)

```bash
cd citest

# All E2E workflow tests (some may fail)
ginkgo -v --silence-skips --focus="E2E Workflow"

# Specific E2E tests
ginkgo -v --silence-skips --focus="Document Approval Workflow"
ginkgo -v --silence-skips --focus="Escape Prevention"
ginkgo -v --silence-skips --focus="Deletion Gates"
ginkgo -v --silence-skips --focus="State Directory Protection"
ginkgo -v --silence-skips --focus="System Admin Bypass"
ginkgo -v --silence-skips --focus="Same-State Movement"
```

### Run Multiple Tests with Regex

```bash
cd citest

# Run all deletion-related tests
ginkgo -v --silence-skips --focus="Deletion|Protection"

# Run tests 1 and 2
ginkgo -v --silence-skips --focus="Document Approval|Escape Prevention"
```

### Dry Run (Check Structure)

```bash
cd citest
ginkgo -v --dry-run --focus="Document Approval Workflow"
```

### Run with Coverage

```bash
cd citest
ginkgo -v --silence-skips --focus="E2E Workflow" --cover
```

### Run All Integration Tests

```bash
cd citest
ginkgo -v
```

## Test Flags

### Useful Ginkgo Flags

- `-v` - Verbose output
- `--silence-skips` - Don't show skipped tests
- `--focus="pattern"` - Run tests matching pattern
- `--skip="pattern"` - Skip tests matching pattern
- `--dry-run` - Show what would run without executing
- `--cover` - Generate coverage report
- `--race` - Enable race detector
- `--trace` - Print stack traces on failure
- `--fail-fast` - Stop on first failure
- `-p` - Run specs in parallel

### Examples

```bash
# Verbose with no skipped test noise
ginkgo -v --silence-skips --focus="Document Approval"

# Fail fast on errors
ginkgo -v --fail-fast --focus="E2E Workflow"

# Run with race detector
ginkgo -v --race --focus="Document Approval"

# Run in parallel (if tests are thread-safe)
ginkgo -v -p --focus="E2E Workflow"
```

## Test Environment

The tests use:
- **TestDatabase** - Ephemeral MySQL database with migrations
- **TestS3** - LocalStack S3-compatible storage
- **Real Services** - Actual FileService and DirectoryService instances
- **Full Workflow Stack** - Loader, Engine, Gates, Audit

Each test suite creates a fresh environment in `BeforeAll` and cleans up in `AfterAll`.

## Troubleshooting

### MySQL Connection Errors

Ensure MySQL is running:
```bash
docker ps | grep mysql
```

Start services if needed:
```bash
cd .. && make up
```

### LocalStack Errors

Check LocalStack is running:
```bash
docker ps | grep localstack
```

### Ginkgo Not Found

Install Ginkgo:
```bash
go install github.com/onsi/ginkgo/v2/ginkgo@latest
```

### Tests Timing Out

Increase timeout (default 90s):
```bash
ginkgo -v --timeout=5m --focus="Document Approval"
```

## Expected Output

Successful test run should show:

```
Running Suite: Integration Test Suite
Random Seed: 1759816229

Will run 1 of 213 specs
------------------------------
E2E Workflow Integration Tests Test 1: Document Approval Workflow 
  should enforce state transitions through draft -> review -> published
  
  🚀 Setting up Workflow Integration Test Environment...
  ✅ Workflow test environment ready
  
  Step: Creating workflow directory structure
  Step: Creating .workflow file with state definitions
  Step: Creating a file in draft (initial state)
  Step: Moving file from draft to review
  Step: Moving file from review to published
  Step: Attempting to move back to draft should fail
  
• [2.145 seconds]
------------------------------

Ran 1 of 213 Specs in 2.145 seconds
SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 212 Skipped

Ginkgo ran 1 suite in 6.23s
Test Suite Passed
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Workflow Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'
      
      - name: Start Services
        run: docker-compose up -d mysql localstack
      
      - name: Wait for Services
        run: |
          sleep 10
          docker ps
      
      - name: Install Ginkgo
        run: go install github.com/onsi/ginkgo/v2/ginkgo@latest
      
      - name: Run Workflow Tests
        run: |
          cd citest
          ginkgo -v --silence-skips --focus="E2E Workflow"
```

## Contributing

When adding new workflow tests:

1. Follow the existing test structure
2. Use descriptive `By()` steps
3. Clean up resources in test body
4. Add test description to this document
5. Ensure tests are idempotent

## Related Documentation

- [Workflow API Documentation](../docs/WORKFLOW_API.md)
- [Workflow Authorization Guide](../docs/WORKFLOW_AUTHORIZATION.md)
- [Workflow Completion Summary](../WORKFLOW_COMPLETION_SUMMARY.md)
- [Main README](../README.md)
