# 12. Development Guide

[← Back: Testing](11_TESTING.md) | [Index](0_README.md)

---

## Project Structure

```
mysql-vfs/
├── pkg/                  # Core packages
│   ├── config/           # Configuration
│   ├── middleware/       # Auth, validation, etc.
│   ├── domain/           # Business logic
│   ├── repository/       # Data access
│   ├── models/           # Domain models
│   └── storage/          # S3/MinIO
├── services/vfs/         # Main VFS service
├── cli/                  # CLI tool
├── citest/               # Integration tests
└── docs/                 # Documentation

## Development Setup

```bash
# Clone repo
git clone https://github.com/telnet2/mysql-vfs
cd mysql-vfs

# Start dependencies
docker-compose up -d mysql minio

# Run VFS locally
export AUTH_PROVIDER=headers
export AUTH_ALLOW_ANONYMOUS=true
export DB_DSN=root:root@tcp(localhost:3306)/vfs?parseTime=true
go run ./services/vfs/main.go

# Run tests
go test ./citest -v
```

---

## Adding a New Feature

1. **Domain Layer** - Add business logic
2. **Repository** - Add data access (if needed)
3. **Handler** - Add HTTP endpoint
4. **Middleware** - Add cross-cutting concern (if needed)
5. **Tests** - Add E2E tests

---

## Code Style

- Follow Go best practices
- Use `gofmt`
- Write tests for new features
- Document public APIs

---

[← Back: Testing](11_TESTING.md) | [Index](0_README.md)
