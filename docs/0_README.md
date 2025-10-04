# MySQL VFS Documentation

**Version:** v2.0
**Last Updated:** 2025-10-04
**Status:** ✅ Production Ready

---

## 📚 Documentation Index

Read the documentation in the following order:

### Getting Started

1. **[Overview & Vision](1_OVERVIEW.md)** - What is MySQL VFS v2 and implementation status
2. **[Architecture](2_ARCHITECTURE.md)** - System design, layers, and data flow
3. **[Quick Start](3_QUICKSTART.md)** - Get up and running in 5 minutes

### Core Features

4. **[Special Files](4_SPECIAL_FILES.md)** - `.jsonschema`, `.rego`, `.quota` files
5. **[Authentication](5_AUTHENTICATION.md)** - Pluggable auth (JWT, OAuth, mTLS)
6. **[Authorization](6_AUTHORIZATION.md)** - OPA policies and access control

### Configuration & Deployment

7. **[Configuration Guide](7_CONFIGURATION.md)** - All environment variables and settings
8. **[Authentication Setup](8_AUTH_SETUP.md)** - Detailed auth configuration examples
9. **[Deployment Guide](9_DEPLOYMENT.md)** - Docker, Kubernetes, production setup

### Development & Testing

10. **[API Reference](10_API.md)** - Complete API documentation
11. **[Testing Guide](11_TESTING.md)** - Running tests, writing new tests
12. **[Development Guide](12_DEVELOPMENT.md)** - Contributing, code structure

### Implementation Details

13. **[Migration from v1](13_MIGRATION.md)** - Upgrading from v1 to v2
14. **[Implementation History](14_IMPLEMENTATION.md)** - How v2 was built
15. **[Design Decisions](15_DESIGN_DECISIONS.md)** - Why we made certain choices

---

## 🎯 Quick Links

**First Time Here?** Start with:
- [Overview](1_OVERVIEW.md) → [Quick Start](3_QUICKSTART.md) → [Configuration](7_CONFIGURATION.md)

**Deploying to Production?**
- [Authentication Setup](8_AUTH_SETUP.md) → [Deployment Guide](9_DEPLOYMENT.md)

**Want to Understand Special Files?**
- [Special Files](4_SPECIAL_FILES.md) → [Authorization](6_AUTHORIZATION.md)

**Developing/Contributing?**
- [Architecture](2_ARCHITECTURE.md) → [Development Guide](12_DEVELOPMENT.md) → [Testing](11_TESTING.md)

---

## 📖 What's New in v2

### Major Changes

- ✅ **Special Files System** - Everything is a file (`.jsonschema`, `.rego`, `.quota`)
- ✅ **External Authentication** - Pluggable auth (JWT, OAuth, mTLS)
- ✅ **Simplified Architecture** - No built-in user management
- ✅ **Layered Design** - Clean separation of concerns
- ✅ **Production Ready** - 104 tests passing, secure by default

### Removed from v1

- ❌ Built-in user/group management (now external)
- ❌ Custom auth endpoints (now pluggable)
- ❌ Database-stored policies (now `.rego` files)

---

## 🚀 Quick Start Example

```bash
# 1. Set configuration
export AUTH_PROVIDER=jwt
export AUTH_JWT_SECRET=your-secret-min-32-chars
export DB_DSN=root:password@tcp(localhost:3306)/vfs?parseTime=true

# 2. Run VFS
docker-compose up -d

# 3. Create a schema
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": ".jsonschema",
    "content_type": "application/json",
    "content": "{\"type\":\"object\",\"required\":[\"email\"]}"
  }'

# 4. Upload a file (auto-validated against schema)
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/data",
    "name": "user.json",
    "content_type": "application/json",
    "content": "{\"email\":\"alice@example.com\"}"
  }'
```

See [Quick Start](3_QUICKSTART.md) for full tutorial.

---

## 🏗️ Architecture Overview

```
External Auth → Generic Middleware → OPA Policies → VFS Core
```

VFS v2 is a **pure file system** with:
- File-based configuration (special files)
- External authentication (JWT, OAuth, etc.)
- Policy-based authorization (OPA + .rego files)
- Schema validation (.jsonschema files)

See [Architecture](2_ARCHITECTURE.md) for details.

---

## 🔐 Security

**Authentication:**
- Production: JWT (cryptographically verified)
- Enterprise: OAuth/OIDC, mTLS
- Development: Header-based (unsafe, dev only)

**Authorization:**
- OPA policies via `.rego` files
- Fine-grained access control
- Inheritable policies (directory hierarchy)

See [Authentication](5_AUTHENTICATION.md) and [Authorization](6_AUTHORIZATION.md).

---

## 📊 Project Status

| Component | Status | Tests |
|-----------|--------|-------|
| Core VFS | ✅ Complete | 104/104 passing |
| Special Files | ✅ Complete | ✅ Validated |
| Authentication | ✅ JWT Implemented | ✅ Tested |
| Authorization | ✅ OPA Integrated | ✅ Tested |
| Schema Validation | ✅ Complete | ✅ Tested |
| Documentation | ✅ Complete | - |

---

## 🤝 Contributing

See [Development Guide](12_DEVELOPMENT.md) for:
- Code structure
- How to add features
- Testing guidelines
- Pull request process

---

## 📞 Support

- **Issues:** [GitHub Issues](https://github.com/telnet2/mysql-vfs)
- **Discussions:** [GitHub Discussions](https://github.com/telnet2/mysql-vfs/discussions)
- **Documentation:** You're reading it!

---

**Let's get started!** → [1. Overview & Vision](1_OVERVIEW.md)
