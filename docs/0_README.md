# MySQL VFS Documentation

**Version:** v2.1 (In Progress)
**Last Updated:** 2025-10-04
**Status:** 🚧 v2.1 Development

---

## 📚 Documentation Index

Read the documentation in the following order:

### Getting Started

1. **[Overview & Status](1_OVERVIEW.md)** - What is MySQL VFS and v2.1 features
2. **[Architecture](2_ARCHITECTURE.md)** - System design, layers, and event system
3. **[Quick Start](3_QUICKSTART.md)** - Get up and running in 5 minutes

### Core Features

4. **[Special Files](4_SPECIAL_FILES.md)** - `.files`, `.events`, `.rego`, `.quota`, `.user`, `.group`
5. **[Authentication](5_AUTHENTICATION.md)** - Hybrid auth (super user + file-based + JWT/OAuth)
6. **[Authorization](6_AUTHORIZATION.md)** - OPA policies and access control

### Configuration & Deployment

7. **[Configuration Guide](7_CONFIGURATION.md)** - All environment variables and settings
8. **[Authentication Setup](8_AUTH_SETUP.md)** - Bootstrap guide and auth configuration
9. **[Deployment Guide](9_DEPLOYMENT.md)** - Docker, Kubernetes, production setup

### Development & API

10. **[API Reference](10_API.md)** - Complete API documentation
11. **[Testing Guide](11_TESTING.md)** - Running tests, writing new tests
12. **[Development Guide](12_DEVELOPMENT.md)** - Contributing, code structure

### Technical Specifications

13. **[.files Specification](13_FILES_SPEC.md)** - Pattern-based validation spec
14. **[.events Specification](14_EVENTS_SPEC.md)** - Event system and webhooks spec

---

## 🎯 Quick Links

**First Time Here?** Start with:
- [Overview](1_OVERVIEW.md) → [Quick Start](3_QUICKSTART.md) → [Configuration](7_CONFIGURATION.md)

**Deploying to Production?**
- [Authentication Setup](8_AUTH_SETUP.md) → [Deployment Guide](9_DEPLOYMENT.md)

**Want to Understand Special Files?**
- [Special Files](4_SPECIAL_FILES.md) → [.files Spec](13_FILES_SPEC.md) → [.events Spec](14_EVENTS_SPEC.md)

**Setting Up Webhooks?**
- [.events Specification](14_EVENTS_SPEC.md) → [Special Files](4_SPECIAL_FILES.md)

**Developing/Contributing?**
- [Architecture](2_ARCHITECTURE.md) → [Development Guide](12_DEVELOPMENT.md) → [Testing](11_TESTING.md)

---

## 📖 What's New in v2.1

### Major Changes

- ✅ **`.files` Pattern Validation** - Replace `.jsonschema` with flexible pattern-based validation
- ✅ **File-Based Authentication** - Self-contained `.user`/`.group` files
- ✅ **Hybrid Auth** - Super user token + file-based + external providers
- 🚧 **Event System (`.events`)** - Webhooks, logging, metrics for file/directory operations

### New Special Files

| File | Purpose | Status |
|------|---------|--------|
| `.files` | Pattern-based file validation | ✅ Complete |
| `.user` | User credentials and tokens | ✅ Complete |
| `.group` | Group membership | ✅ Complete |
| `.events` | Event handlers (webhooks, logs, metrics) | 🚧 Designed |

### Breaking Changes from v2.0

- ❌ `.jsonschema` → Replaced by `.files` (migration required)
- ❌ Built-in user DB → Replaced by `.user` files or external auth
- 🚧 `.webhook` → Will be replaced by `.events` (when implemented)

---

## 🚀 Quick Start Example

```bash
# 1. Generate super user token
export SUPER_USER_TOKEN=$(openssl rand -hex 32)
export AUTH_PROVIDER=file

# 2. Start VFS
docker-compose up -d

# 3. Create .user file (bootstrap)
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SUPER_USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "directory_path": "/",
    "name": ".user",
    "content": "{\"users\":[{\"user_id\":\"admin\",\"token\":\"admin-token\",\"role\":\"admin\"}]}"
  }'

# 4. Create .files rules
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer admin-token" \
  -d '{
    "directory_path": "/data",
    "name": ".files",
    "content": "{\"rules\":[{\"pattern\":\"*.json\",\"type\":\"glob\",\"schema\":{\"type\":\"object\",\"required\":[\"email\"]}}]}"
  }'

# 5. Upload a file (auto-validated against pattern + schema)
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer admin-token" \
  -d '{
    "directory_path": "/data",
    "name": "user.json",
    "content": "{\"email\":\"alice@example.com\",\"name\":\"Alice\"}"
  }'
```

See [Quick Start](3_QUICKSTART.md) for full tutorial.

---

## 🏗️ Architecture Overview

```
External Auth → Middleware Chain → Event System → VFS Core
                                          ↓
                              Webhooks, Logs, Metrics
```

VFS v2.1 is a **pure file system** with:
- Pattern-based validation (`.files`)
- Event-driven architecture (`.events`)
- File-based configuration (all special files)
- External authentication (JWT, OAuth, or `.user` files)
- Policy-based authorization (OPA + `.rego` files)

See [Architecture](2_ARCHITECTURE.md) for details.

---

## 🔐 Security

**Authentication:**
- Production: File-based (`.user` files) or JWT (cryptographically verified)
- Enterprise: OAuth/OIDC, mTLS
- Bootstrap: Super user token (env var)
- Development: Header-based (unsafe, dev only)

**Authorization:**
- OPA policies via `.rego` files
- Fine-grained access control
- Inheritable policies (directory hierarchy)

See [Authentication](5_AUTHENTICATION.md) and [Authorization](6_AUTHORIZATION.md).

---

## 📊 Project Status (v2.1)

| Component | Status | Tests |
|-----------|--------|-------|
| Core VFS | ✅ Complete | 104/104 passing |
| `.files` Validation | ✅ Complete | ⏳ Pending |
| File-Based Auth (`.user`/`.group`) | ✅ Complete | ⏳ Pending |
| Hybrid Auth (Super User) | ✅ Complete | ⏳ Pending |
| `.events` System | 🚧 Designed | ⏳ Not Started |
| Webhook Handlers | ⏳ Not Started | ⏳ Not Started |
| Documentation | 🚧 60% Complete | - |

**Overall v2.1 Progress: 60%**

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

## 📁 Archive

Older documentation has been moved to `archive/`:
- `archive/AUTHENTICATION.md` - Old auth docs (superseded by 5_AUTHENTICATION.md)
- `archive/configuration-guide.md` - Old config docs (superseded by 7_CONFIGURATION.md)
- `archive/TEST_GUIDE.md` - Old test guide (superseded by 11_TESTING.md)
- `archive/PHASE_6_REPORT.md` - Implementation report
- `archive/8_AUTH_SETUP.md` - Old auth setup (replaced by bootstrap guide)

---

**Let's get started!** → [1. Overview & Status](1_OVERVIEW.md)
