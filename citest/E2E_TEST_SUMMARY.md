# End-to-End Schema Validation Workflow Test

## Overview

The `e2e_schema_workflow_test.go` provides a comprehensive end-to-end test that validates the complete mysql-vfs system with schema validation, including:

- ✅ MySQL database integration (via testcontainers)
- ✅ S3 storage integration (MinIO)
- ✅ Schema library management
- ✅ External schema references with `schema://` protocol
- ✅ Nested `$ref` resolution (schemas referencing other schemas)
- ✅ File validation with complex schemas
- ✅ CRUD operations with validation
- ✅ Error handling for invalid data

## Test Execution

```bash
# Run the E2E test
ginkgo -v --focus="End-to-End Schema Validation Workflow" ./citest

# Or run all E2E tests
ginkgo -v --focus="End-to-End" ./citest
```

## Test Workflow

### Step 1: Schema Library Setup (`/schemas`)

Creates reusable schema library:
- `/schemas/address.json` - Address schema with validation rules
- `/schemas/contact.json` - Contact information schema
- `/schemas/customer.json` - Customer schema **with nested `$ref`** to address and contact
- `/schemas/product.json` - Product schema with enum validation

### Step 2: Application Directories with Validation Rules

Creates two application directories with `.files` configs:

**`/customers/.files`:**
```json
{
  "rules": [{
    "pattern": "*.json",
    "type": "glob",
    "schema": {"$ref": "schema:///schemas/customer.json"}
  }],
  "default_action": "deny"
}
```

**`/products/.files`:**
```json
{
  "rules": [{
    "pattern": "*.json",
    "type": "glob",
    "schema": {"$ref": "schema:///schemas/product.json"}
  }],
  "default_action": "deny"
}
```

### Step 3-4: Valid and Invalid Customer Uploads

**Valid customers** (✅ accepted):
- `alice.json` - Complete customer with billing and shipping addresses
- `bob.json` - Customer without optional shipping address

**Invalid customers** (❌ rejected):
- Missing required `billing_address`
- Wrong `customer_id` format (pattern validation)
- Invalid email in nested `contact` object
- Invalid zipcode in nested `billing_address` object

### Step 5-6: Valid and Invalid Product Uploads

**Valid products** (✅ accepted):
- `laptop.json` - Electronics category
- `tshirt.json` - Clothing category

**Invalid products** (❌ rejected):
- Negative price (minimum validation)
- Invalid category (enum validation)

### Step 7: Update Operations with Validation

- ✅ Valid update to `alice.json` (version 1 → 2)
- ❌ Invalid update rejected (version stays at 2)

### Step 8: File Listing Verification

Verifies directory contents:
- `/customers` → 3 files (alice.json, bob.json, .files)
- `/products` → 3 files (laptop.json, tshirt.json, .files)

### Step 9: Pattern Matching

Tests `default_action: deny`:
- ❌ Non-JSON file rejected (doesn't match `*.json` pattern)

### Step 10: Cleanup

- Delete individual file (bob.json)
- Recursive directory deletion
- Verify final state

## What This Test Validates

### Core Functionality
1. **Schema Loading** - Schemas loaded from VFS correctly
2. **Schema Caching** - Schemas cached for performance
3. **Nested $ref Resolution** - Multi-level schema references work
4. **Validation on Create** - Invalid files rejected on upload
5. **Validation on Update** - Invalid updates rejected, version unchanged
6. **Pattern Matching** - Glob patterns work correctly
7. **Default Action** - `default_action: deny` blocks non-matching files

### Schema Validation
1. **Required Fields** - Missing required fields caught
2. **Pattern Validation** - Regex patterns enforced (customer_id, phone, zipcode, state)
3. **Format Validation** - Email format validated
4. **Enum Validation** - Category enum enforced
5. **Numeric Validation** - Minimum price enforced
6. **Nested Validation** - Validation works in nested objects (address, contact)

### System Integration
1. **MySQL Integration** - Database operations via testcontainers
2. **S3 Integration** - File storage in MinIO
3. **Directory Service** - Directory CRUD operations
4. **File Service** - File CRUD operations with validation
5. **FilesLoader** - .files config loading and caching

## Real-World Scenario

This test simulates a real-world e-commerce application:

```
/
├── schemas/              # Centralized schema library
│   ├── address.json      # Reusable address schema
│   ├── contact.json      # Reusable contact schema
│   ├── customer.json     # Customer (refs address + contact)
│   └── product.json      # Product schema
│
├── customers/            # Customer data directory
│   ├── .files            # Validation rules for customers
│   ├── alice.json        # Valid customer (v2)
│   └── bob.json          # Valid customer (deleted later)
│
└── products/             # Product catalog directory
    ├── .files            # Validation rules for products
    ├── laptop.json       # Valid product
    └── tshirt.json       # Valid product
```

## Test Results

```
🚀 Setting up End-to-End Schema Workflow test environment...
   ✓ Created /schemas directory
   ✓ Created /schemas/address.json
   ✓ Created /schemas/contact.json
   ✓ Created /schemas/customer.json (with nested $ref)
   ✓ Created /schemas/product.json
   ✓ Created /customers/.files with schema validation
   ✓ Created /products/.files with schema validation
   ✓ Uploaded valid customer: alice.json
   ✓ Uploaded valid customer: bob.json
   ✓ Correctly rejected invalid customer (missing billing_address)
   ✓ Correctly rejected invalid customer (wrong ID format)
   ✓ Correctly rejected invalid customer (invalid email)
   ✓ Correctly rejected invalid customer (invalid zipcode)
   ✓ Uploaded valid product: laptop.json
   ✓ Uploaded valid product: tshirt.json
   ✓ Correctly rejected invalid product (negative price)
   ✓ Correctly rejected invalid product (invalid category)
   ✓ Updated alice.json successfully
   ✓ Correctly rejected invalid update
   ✓ Found 3 files in /customers (2 customers + .files)
   ✓ Found 3 files in /products (2 products + .files)
   ✓ Correctly rejected non-matching file (notes.txt)
   ✓ Deleted bob.json
   ✓ Cleaned up all test directories

🎉 Complete end-to-end workflow with nested schema validation passed!

SUCCESS! -- 1 Passed | 0 Failed | 0 Pending | 157 Skipped
Test completed in 9.747 seconds
```

## Key Achievements

1. **Complete Local Environment** - Full MySQL + S3 + Application stack
2. **Nested Schema Resolution** - Customer schema references both address and contact schemas
3. **Real-world Data Model** - E-commerce use case with realistic validation
4. **Comprehensive Error Cases** - Tests 8+ different validation failure scenarios
5. **Full CRUD Cycle** - Create, read, update, delete with validation
6. **Performance** - Completes in ~10 seconds including container startup

## Running This Test

```bash
# Prerequisites
- Docker (for testcontainers)
- Go 1.21+
- Ginkgo test framework

# Run the test
ginkgo -v --focus="End-to-End Schema Validation" ./citest

# Run with verbose output
ginkgo -v -trace --focus="End-to-End Schema Validation" ./citest
```

## Troubleshooting

If the test fails:

1. **Check Docker** - Ensure Docker is running
2. **Check Ports** - Ensure MySQL (random port) and MinIO ports are available
3. **Check Logs** - Look for MySQL/S3 connection errors in output
4. **Cleanup** - Previous test runs should auto-cleanup, but check `docker ps`

## Future Enhancements

Potential additions to this test:

1. **Concurrent Operations** - Multiple clients uploading simultaneously
2. **Large Files** - Test schema validation on large files (>16MB)
3. **Schema Evolution** - Update schema and test backward compatibility
4. **Performance Metrics** - Measure validation latency
5. **Cache Invalidation** - Test schema cache invalidation on schema updates
