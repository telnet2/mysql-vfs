# Documentation Guide

**Last Updated**: October 7, 2025

---

## Quick Navigation

### 🌟 Start Here - Feature Guides

If you want comprehensive, end-to-end information about major features:

1. **[Workflows](./WORKFLOWS.md)** - Complete workflow system guide
   - Directory-as-state architecture
   - Rego policy gates
   - Audit trails and events
   - REST API reference
   - Examples and best practices

2. **[Special Files Framework](./SPECIAL_FILES_FRAMEWORK.md)** - Framework architecture
   - All 7 special file types (`.workflow`, `.rego`, `.events`, etc.)
   - Validation system (3 layers)
   - Framework features (SchemaValidator, LoaderFactory, etc.)
   - Cross-file validation
   - API reference and examples

---

## Documentation Structure

### Feature Guides (Comprehensive, ⭐ Recommended)

**When to use**: You want complete understanding of a major feature

| Document | Purpose | Audience |
|----------|---------|----------|
| [WORKFLOWS.md](./WORKFLOWS.md) | Complete workflow system guide | Developers, Operators |
| [SPECIAL_FILES_FRAMEWORK.md](./SPECIAL_FILES_FRAMEWORK.md) | Framework architecture & all special files | Developers, Architects |
| [WORKFLOW_API.md](./WORKFLOW_API.md) | Workflow REST API reference | API Users |
| [WORKFLOW_AUTHORIZATION.md](./WORKFLOW_AUTHORIZATION.md) | Workflow authorization integration | Security Engineers |

---

### User Guides (Task-Oriented)

**When to use**: You want to accomplish a specific task

#### Getting Started
- [Overview](./1_OVERVIEW.md) - System concepts and architecture overview
- [Quickstart](./3_QUICKSTART.md) - Get up and running quickly
- [User Guide](./USER_GUIDE.md) - End-user documentation

#### Core Features
- [Special Files](./4_SPECIAL_FILES.md) - All special file types reference
- [Authentication](./5_AUTHENTICATION.md) - User authentication system
- [Authorization](./6_AUTHORIZATION.md) - OPA-based authorization
- [Metadata](./METADATA.md) - File and directory metadata
- [System Files](./SYSTEM_FILES.md) - System-managed files in `/etc`

#### Operations
- [Configuration](./7_CONFIGURATION.md) - Environment configuration
- [Deployment](./9_DEPLOYMENT.md) - Production deployment guide
- [Bootstrap](./18_BOOTSTRAP.md) - System initialization

#### API & Integration
- [API Reference](./10_API.md) - Complete REST API documentation
- [CLI How-To](./CLI_HOWTO.md) - Command-line interface guide
- [Webhooks](./17_WEBHOOKS.md) - Webhook integration
- [Event Pub/Sub](./EVENT_PUBSUB.md) - Event system details

#### Development
- [Architecture](./2_ARCHITECTURE.md) - System architecture deep-dive
- [Design](./DESIGN.md) - Design decisions and rationale
- [Development](./12_DEVELOPMENT.md) - Developer setup and workflow
- [Testing](./11_TESTING.md) - Testing strategies and tools
- [Microservices](./MICROSERVICES.md) - Microservices architecture

#### Advanced Features
- [Lifecycle Events](./15_LIFECYCLE_EVENTS.md) - Event system implementation
- [Event Examples](./16_LIFECYCLE_EXAMPLES.md) - Event handler examples
- [Files Spec](./13_FILES_SPEC.md) - File validation specification
- [Events Spec](./14_EVENTS_SPEC.md) - Event system specification
- [Resource Protection](./19_RESOURCE_PROTECTION.md) - Resource protection system
- [Owner-Based Access](./20_OWNER_BASED_ACCESS.md) - Ownership model
- [On Behalf Of](./ON_BEHALF_OF.md) - Delegation mechanism
- [Schema Ref Feature](./SCHEMA_REF_FEATURE.md) - Schema reference system
- [Trigger Feature](./TRIGGER_FEATURE.md) - File triggers
- [Webhook Quick Start](./WEBHOOK_QUICK_START.md) - Quick webhook setup
- [Webhook Triggers Guide](./WEBHOOK_TRIGGERS_GUIDE.md) - Webhook triggers

#### Design Documents
- [Event Publisher Design](./EVENT_PUBLISHER_DESIGN.md) - Future SSE event streaming
- [Security](./SECURITY.md) - Security architecture and best practices

#### Status & Planning
- [Implementation Status](./21_IMPLEMENTATION_STATUS.md) - Current implementation status
- [Operations](./OPERATIONS.md) - Operational procedures

---

## Documentation by Role

### For End Users

1. Start with [User Guide](./USER_GUIDE.md)
2. Learn [CLI How-To](./CLI_HOWTO.md)
3. Understand [Authentication](./5_AUTHENTICATION.md)
4. Explore [Webhooks](./WEBHOOK_QUICK_START.md) if needed

### For Developers

1. Read [Overview](./1_OVERVIEW.md) and [Architecture](./2_ARCHITECTURE.md)
2. Follow [Quickstart](./3_QUICKSTART.md)
3. Study [**Workflows**](./WORKFLOWS.md) for workflow implementation
4. Study [**Special Files Framework**](./SPECIAL_FILES_FRAMEWORK.md) for special files
5. Check [Development](./12_DEVELOPMENT.md) for dev workflow
6. Review [Testing](./11_TESTING.md) for testing approach

### For Operators

1. Review [Deployment](./9_DEPLOYMENT.md)
2. Set up with [Bootstrap](./18_BOOTSTRAP.md)
3. Configure via [Configuration](./7_CONFIGURATION.md)
4. Understand [Operations](./OPERATIONS.md)
5. Monitor with [Microservices](./MICROSERVICES.md) guide

### For Architects

1. Study [Design](./DESIGN.md)
2. Understand [Architecture](./2_ARCHITECTURE.md)
3. Review [**Special Files Framework**](./SPECIAL_FILES_FRAMEWORK.md)
4. Read [Security](./SECURITY.md)
5. Explore [Event Publisher Design](./EVENT_PUBLISHER_DESIGN.md)

### For Security Engineers

1. Read [Security](./SECURITY.md)
2. Understand [Authorization](./6_AUTHORIZATION.md)
3. Study [**Workflow Authorization**](./WORKFLOW_AUTHORIZATION.md)
4. Review [Authentication](./5_AUTHENTICATION.md)
5. Check [Owner-Based Access](./20_OWNER_BASED_ACCESS.md)

---

## Finding What You Need

### "How do I...?"

| Task | Document |
|------|----------|
| Set up the system | [Quickstart](./3_QUICKSTART.md), [Bootstrap](./18_BOOTSTRAP.md) |
| Create a workflow | [**Workflows**](./WORKFLOWS.md) |
| Write an OPA policy | [Authorization](./6_AUTHORIZATION.md), [**.rego in Special Files**](./SPECIAL_FILES_FRAMEWORK.md#2-rego---opa-policies) |
| Set up webhooks | [Webhook Quick Start](./WEBHOOK_QUICK_START.md) |
| Validate files | [Files Spec](./13_FILES_SPEC.md), [**.files in Special Files**](./SPECIAL_FILES_FRAMEWORK.md#4-files---file-pattern-rules) |
| Manage users | [Authentication](./5_AUTHENTICATION.md), [**.user in Special Files**](./SPECIAL_FILES_FRAMEWORK.md#5-user---user-credentials) |
| Control access | [Authorization](./6_AUTHORIZATION.md), [Owner-Based Access](./20_OWNER_BASED_ACCESS.md) |
| Use the API | [API Reference](./10_API.md) |
| Use the CLI | [CLI How-To](./CLI_HOWTO.md) |
| Deploy to production | [Deployment](./9_DEPLOYMENT.md) |
| Run tests | [Testing](./11_TESTING.md) |
| Understand events | [Lifecycle Events](./15_LIFECYCLE_EVENTS.md), [Event Pub/Sub](./EVENT_PUBSUB.md) |

### "What is...?"

| Concept | Document |
|---------|----------|
| Workflow | [**Workflows**](./WORKFLOWS.md) |
| Special files | [**Special Files Framework**](./SPECIAL_FILES_FRAMEWORK.md) |
| Directory-as-state | [**Workflows**](./WORKFLOWS.md#core-concepts) |
| OPA policy | [Authorization](./6_AUTHORIZATION.md) |
| Lifecycle events | [Lifecycle Events](./15_LIFECYCLE_EVENTS.md) |
| Metadata | [Metadata](./METADATA.md) |
| System files | [System Files](./SYSTEM_FILES.md) |
| Resource protection | [Resource Protection](./19_RESOURCE_PROTECTION.md) |
| Ownership model | [Owner-Based Access](./20_OWNER_BASED_ACCESS.md) |

---

## Documentation Conventions

### File Naming

- **UPPERCASE.md** - Major feature guides (e.g., WORKFLOWS.md)
- **NUMBER_NAME.md** - Sequential user guides (e.g., 1_OVERVIEW.md)
- **PascalCase.md** - Legacy or specific topics (e.g., UserGuide.md)

### Symbols

- **⭐ Recommended** - Start here for comprehensive information
- **🌟 Start Here** - Best entry point for new users
- **✅ Complete** - Fully documented feature
- **🚧 In Progress** - Work in progress
- **📚 Reference** - API or specification reference

### Structure

Most feature guides follow this structure:
1. **Overview** - What it is and why
2. **Core Concepts** - Key ideas and terminology
3. **Architecture** - How it works
4. **Configuration** - How to set it up
5. **API Reference** - Technical details
6. **Examples** - Real-world usage
7. **Best Practices** - Do's and don'ts

---

## Contributing to Documentation

### Adding New Documentation

1. **Feature Guides** - Add to `docs/FEATURE_NAME.md`
   - Use for comprehensive, end-to-end feature documentation
   - Include examples and best practices
   - Link from README.md

2. **User Guides** - Add to `docs/NUMBER_NAME.md`
   - Use for task-oriented, how-to content
   - Keep focused on specific tasks
   - Link from this guide

3. **Design Docs** - Add to `docs/DESIGN_NAME.md`
   - Use for architecture decisions and future plans
   - Include diagrams and rationale

### Updating Documentation

1. **Update the source document** in `docs/`
2. **Update this guide** if navigation changes
3. **Update README.md** if new major features added
4. **Test all links** to ensure they work

### Documentation Quality

Good documentation:
- ✅ Has clear purpose and audience
- ✅ Includes working examples
- ✅ Uses consistent formatting
- ✅ Has up-to-date information
- ✅ Links to related docs
- ✅ Includes troubleshooting section

---

## Archive

Historical implementation documents and old versions are archived in [docs/archive/](./archive/).

See [Archive README](./archive/README.md) for details on what was consolidated and why.

---

## Getting Help

### Documentation Issues

- **Missing information**: Check archive first, then open an issue
- **Broken links**: Open an issue with the broken link
- **Outdated content**: Open a PR with corrections
- **Unclear explanations**: Open an issue with feedback

### Questions

1. **Read the docs** - Check feature guides first
2. **Search issues** - Someone may have asked already
3. **Open discussion** - For design questions
4. **Open issue** - For bugs or missing features

---

*Last Updated: October 7, 2025*  
*Documentation Structure: v2.0*
