# Archive

This directory contains archived documentation from the previous structure.

## Archive Date

**Archived:** 2025-10-05

**Reason:** Restructured from 24 files to 5 core files for better maintainability and user experience.

---

## Directory Structure

### old_structure/

Contains the previous numbered documentation structure (0-22):

**Overview & Architecture:**
- `0_README.md` - Old readme
- `1_OVERVIEW.md` - System overview
- `2_ARCHITECTURE.md` - Architecture details

**Getting Started:**
- `3_QUICKSTART.md` - Quick start guide

**Features:**
- `4_SPECIAL_FILES.md` - Special files documentation
- `13_FILES_SPEC.md` - Files validation spec
- `14_EVENTS_SPEC.md` - Events specification
- `15_LIFECYCLE_EVENTS.md` - Lifecycle events
- `16_LIFECYCLE_EXAMPLES.md` - Event examples
- `17_WEBHOOKS.md` - Webhook documentation
- `22_GROUP_MANAGEMENT.md` - Group management

**Security:**
- `5_AUTHENTICATION.md` - Authentication methods
- `6_AUTHORIZATION.md` - Authorization with OPA
- `8_AUTH_SETUP.md` - Auth setup guide
- `18_BOOTSTRAP.md` - Bootstrap guide
- `19_RESOURCE_PROTECTION.md` - Resource protection
- `20_OWNER_BASED_ACCESS.md` - Owner-based access

**Operations:**
- `7_CONFIGURATION.md` - Configuration reference
- `9_DEPLOYMENT.md` - Deployment guide
- `10_API.md` - API reference
- `11_TESTING.md` - Testing guide
- `12_DEVELOPMENT.md` - Development guide
- `21_IMPLEMENTATION_STATUS.md` - Implementation status

**Total:** 23 numbered files

### specialized/

Contains specialized documentation that may be useful for specific use cases:

- `CLI_HOWTO.md` - CLI usage guide
- `event-handler-plugin.md` - Event handler plugin development
- `performance-enhancement.md` - Performance optimization notes

---

## New Documentation Structure

The documentation has been consolidated into **5 core files** in the `docs/` root:

### 1. README.md
- **Audience:** Everyone
- **Content:** Overview, quick start, core concepts
- **Length:** ~350 lines

### 2. USER_GUIDE.md
- **Audience:** Users, developers using the API
- **Content:** Complete API reference, special files, events, validation, patterns
- **Length:** ~600 lines

### 3. SECURITY.md
- **Audience:** Security engineers, operators
- **Content:** Authentication, authorization, OPA policies, groups, best practices
- **Length:** ~550 lines

### 4. OPERATIONS.md
- **Audience:** DevOps, SREs, operators
- **Content:** Deployment, configuration, monitoring, troubleshooting
- **Length:** ~500 lines

### 5. DESIGN.md
- **Audience:** Contributors, architects, developers
- **Content:** Design philosophy, architecture, decisions, implementation
- **Length:** ~800 lines

**Total:** 5 files (~2,800 lines)

---

## Benefits of New Structure

✅ **Easier to Navigate** - Clear purpose for each doc
✅ **Less Redundancy** - No duplicate information
✅ **Better Onboarding** - Clear path: README → USER_GUIDE → SECURITY
✅ **Self-Contained** - Each doc can be read standalone
✅ **Maintainable** - 5 files vs 24 files to keep in sync

---

## Using Archived Docs

**Reference Only:**
- These docs are kept for reference
- May contain outdated information
- Use new docs for current information

**Migration:**
- All content from old docs has been consolidated into new structure
- If you find something missing, please file an issue

---

## History

**v1.0 - v2.0** (2024-2025)
- 24-file numbered structure
- Separate files for each topic
- Good for granular updates, but hard to navigate

**v2.1+** (2025-10-05)
- 5-file consolidated structure
- Audience-based organization
- Easier to maintain and use

---

**Questions?**
- See main `docs/README.md` for current documentation
- File issues if something is missing or unclear
