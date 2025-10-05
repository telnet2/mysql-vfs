# MySQL VFS Documentation

**Version:** v2.1+
**Last Updated:** 2025-10-05
**Status:** ✅ Production Ready (103/104 tests passing)

---

## 📚 Documentation Index

Read the documentation in the following order:

### Getting Started

1. **[Overview & Status](1_OVERVIEW.md)** - What is MySQL VFS and v2.1 features
2. **[Architecture](2_ARCHITECTURE.md)** - System design, layers, and event system
3. **[Quick Start](3_QUICKSTART.md)** - Get up and running in 5 minutes

### Core Features

4. **[Special Files](4_SPECIAL_FILES.md)** - `.files`, `.events`, `.rego`, `.user` (file-based configuration)
5. **[Authentication](5_AUTHENTICATION.md)** - Hybrid auth (system admin + file-based + JWT/OAuth)
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
15. **[Lifecycle Events](15_LIFECYCLE_EVENTS.md)** - Complete event lifecycle tracking
16. **[Lifecycle Examples](16_LIFECYCLE_EXAMPLES.md)** - Event handler examples
17. **[Webhooks](17_WEBHOOKS.md)** - Webhook delivery and retry logic

### Implementation Guides

18. **[Bootstrap Guide](18_BOOTSTRAP.md)** - Initial setup and user creation
19. **[Resource Protection](19_RESOURCE_PROTECTION.md)** - Protected resource patterns
20. **[Owner-Based Access](20_OWNER_BASED_ACCESS.md)** - Ownership and access control
21. **[Implementation Status](21_IMPLEMENTATION_STATUS.md)** - Feature completion tracking

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

## 📖 What's New in v2.1+

### Major Changes

- ✅ **`.files` Pattern Validation** - Flexible pattern-based validation (replaces `.jsonschema`)
- ✅ **File-Based Authentication** - Self-contained `.user` files (groups deprecated)
- ✅ **Hybrid Auth** - System admin token + file-based + external providers
- ✅ **Event System (`.events`)** - Complete lifecycle event tracking with webhooks
- ✅ **Resource Protection** - Pattern-based protection for special files
- ✅ **Owner-Based Access** - Directory ownership and access control

### New Special Files

| File | Purpose | Status | Implementation |
|------|---------|--------|----------------|
| `.files` | Pattern-based file validation | ✅ Complete | `pkg/domain/files_loader.go` |
| `.user` | User credentials and tokens | ✅ Complete | `pkg/domain/user_loader.go` |
| `.events` | Event handlers (webhooks, logs, metrics) | ✅ Complete | `pkg/domain/events_loader.go` |
| `.rego` | OPA authorization policies | ✅ Complete | `pkg/domain/policy_loader.go` |
| `.owner` | Directory ownership | ✅ Complete | `pkg/domain/owner_loader.go` |

### Breaking Changes from v2.0

- ❌ `.jsonschema` → Replaced by `.files` (more flexible pattern matching)
- ❌ `.group` → Deprecated (use role-based auth only)
- ❌ `.quota`, `.lifecycle` → Removed (admin features deprecated)
- ✅ System admin uses `SYSTEM_ADMIN_TOKEN` (not `SUPER_USER_TOKEN`)

---

## 🚀 Quick Start Example

```bash
# 1. Generate system admin token for bootstrap
export SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)
export AUTH_PROVIDER=file

# 2. Start VFS
docker-compose up -d

# 3. Create .user file (bootstrap with system admin token)
curl -X POST http://localhost:8080/api/v1/files \
  -H "Authorization: Bearer $SYSTEM_ADMIN_TOKEN" \
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
- Enterprise: OAuth/OIDC (planned), mTLS (planned)
- Bootstrap: System admin token (`SYSTEM_ADMIN_TOKEN` env var)
- Development: Header-based (unsafe, dev only)

**Implementation:** See `pkg/middleware/auth.go`, `pkg/middleware/auth_providers.go`, `pkg/domain/user_loader.go`

**Authorization:**
- OPA policies via `.rego` files
- Fine-grained access control
- Inheritable policies (directory hierarchy)
- Owner-based access control

**Implementation:** See `pkg/middleware/authorization.go`, `pkg/domain/policy_loader.go`, `pkg/domain/owner_loader.go`

See [Authentication](5_AUTHENTICATION.md) and [Authorization](6_AUTHORIZATION.md).

---

## 📊 Project Status (v2.1+)

| Component | Status | Tests | Implementation |
|-----------|--------|-------|----------------|
| Core VFS | ✅ Complete | 103/104 passing (1 flaky) | `pkg/domain/file_service.go`, `pkg/domain/directory_service.go` |
| `.files` Validation | ✅ Complete | Included in core | `pkg/domain/files_loader.go` |
| File-Based Auth (`.user`) | ✅ Complete | Included in core | `pkg/domain/user_loader.go` |
| Hybrid Auth (System Admin) | ✅ Complete | Included in core | `pkg/middleware/auth.go` |
| `.events` Lifecycle System | ✅ Complete | Included in core | `pkg/domain/events_loader.go`, `pkg/domain/event_trigger.go` |
| Webhook Handlers | ✅ Complete | Included in core | `pkg/events/handlers/webhook.go` |
| Resource Protection | ✅ Complete | Included in core | `pkg/domain/protection.go` |
| Owner-Based Access | ✅ Complete | Included in core | `pkg/domain/owner_loader.go` |
| Documentation | ✅ Complete | - | 21 comprehensive guides |

**Overall v2.1+ Progress: ✅ Production Ready**

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
