# On-Behalf-Of Delegation

**Version**: 1.0
**Status**: Production Ready
**Last Updated**: 2025-10-06

---

## Overview

The VFS system supports **on-behalf-of delegation**, allowing authorized actors to perform operations on behalf of principals. This enables automation, administrative assistance, and service-to-service workflows while maintaining clear audit trails.

### Key Concepts

- **Actor**: The authenticated user/service making the request (who performs)
- **Principal**: The user on whose behalf the operation is performed (who owns)
- **Delegation**: Actor acts on behalf of principal (requires permission)

---

## Use Cases

### 1. Automated Backups

Backup service creates snapshots on behalf of users:

```bash
# Backup service authenticates as service-account
# Creates backup file owned by alice
vfs-cli import alice-data.tar.gz /backups/alice/ \
  --on-behalf-of=alice \
  --reason="nightly-backup-job"
```

**Result**:
- File owner: `alice`
- File creator: `service-account`
- Audit trail: "nightly-backup-job"

### 2. Admin Assistance

Administrator helps user organize workspace:

```bash
# Admin creates directories for new user
vfs-cli mkdir /users/newuser/projects \
  --on-behalf-of=newuser \
  --reason="onboarding-setup"
```

### 3. CI/CD Pipelines

Jenkins deploys artifacts to application directories:

```bash
# Jenkins pipeline
curl -X POST "${VFS_URL}/api/v1/files?path=/prod/app/release.zip" \
  -H "Authorization: Bearer ${JENKINS_TOKEN}" \
  -H "X-VFS-On-Behalf-Of: deployment-bot" \
  -H "X-VFS-Delegation-Reason: release-v1.2.3" \
  --data-binary @release.zip
```

---

## Security Model

### 4-Layer Validation

```
┌──────────────────────────────────────┐
│ 1. Authentication                    │
│    - Verify JWT/API key/mTLS         │
│    - Establish actor identity        │
└──────────────────────────────────────┘
          ↓
┌──────────────────────────────────────┐
│ 2. Delegation Validation             │
│    - Check impersonate permission    │
│    - Log all attempts                │
└──────────────────────────────────────┘
          ↓
┌──────────────────────────────────────┐
│ 3. Authorization Policy (Rego)       │
│    - can_impersonate rules           │
│    - Defense-in-depth                │
└──────────────────────────────────────┘
          ↓
┌──────────────────────────────────────┐
│ 4. Operation Authorization           │
│    - Authorized as principal         │
│    - Resource-level checks           │
└──────────────────────────────────────┘
```

### Who Can Impersonate?

By default, only these groups can impersonate:
- `service-accounts` - For automation/services
- `system-admin` - For administrative operations

**Default Policy** (`/.rego`):
```rego
can_impersonate {
    input.user.groups[_] == "service-accounts"
}

can_impersonate {
    input.user.groups[_] == "system-admin"
}
```

### Attack Prevention

**Scenario**: Attacker tries to impersonate admin

```http
POST /api/v1/files
Authorization: Bearer attacker-token
X-VFS-On-Behalf-Of: admin
```

**System Response**:
```json
{
  "error": "impersonation denied",
  "message": "user 'attacker' not authorized to impersonate"
}
```

**Security Log**:
```json
{
  "event_type": "impersonation_denied",
  "actor": "attacker",
  "principal": "admin",
  "remote_ip": "192.168.1.100",
  "timestamp": "2025-10-06T10:00:00Z"
}
```

---

## API Usage

### HTTP Headers

**Required**:
- `Authorization: Bearer <token>` - Establishes actor identity

**Optional** (for delegation):
- `X-VFS-On-Behalf-Of: <user-id>` - Principal to act on behalf of
- `X-VFS-Delegation-Reason: <reason>` - Audit trail (optional)

### Examples

#### Create File with Delegation

```bash
curl -X POST "${VFS_URL}/api/v1/files?path=/data/report.txt" \
  -H "Authorization: Bearer ${SERVICE_TOKEN}" \
  -H "X-VFS-On-Behalf-Of: alice" \
  -H "X-VFS-Delegation-Reason: automated-report" \
  -H "Content-Type: text/plain" \
  -d "Report content..."
```

#### Create Directory with Delegation

```bash
curl -X POST "${VFS_URL}/api/v1/directories" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "X-VFS-On-Behalf-Of: newuser" \
  -H "X-VFS-Delegation-Reason: workspace-setup" \
  -H "Content-Type: application/json" \
  -d '{"parent_path":"/","name":"workspace"}'
```

#### Update File with Delegation

```bash
curl -X PUT "${VFS_URL}/api/v1/files/data/config.json" \
  -H "Authorization: Bearer ${SERVICE_TOKEN}" \
  -H "X-VFS-On-Behalf-Of: app-owner" \
  -H "X-VFS-Delegation-Reason: config-update" \
  -H "Content-Type: application/json" \
  -d '{"key":"value"}'
```

---

## CLI Usage

### Global Flags

All CLI commands support delegation via global flags:

```bash
--on-behalf-of <user-id>    Act on behalf of another user
--reason <reason>           Delegation reason (audit trail)
```

### Examples

#### Import Files

```bash
# Import single file
vfs-cli import local.txt /data/ \
  --on-behalf-of=alice \
  --reason="backup-restore"

# Import multiple files
vfs-cli import *.log /logs/ \
  --on-behalf-of=app-service \
  --reason="log-collection"
```

#### Create Directories

```bash
vfs-cli mkdir /users/alice/workspace \
  --on-behalf-of=alice \
  --reason="workspace-init"
```

#### Read Files

```bash
# Read file (delegation for authorization)
vfs-cli cat /sensitive/data.txt \
  --on-behalf-of=authorized-user \
  --reason="troubleshooting"
```

#### Move Files

```bash
vfs-cli mv /old/location.txt /new/location.txt \
  --on-behalf-of=file-owner \
  --reason="reorganization"
```

---

## Metadata Tracking

All delegated operations are tracked in file/directory metadata:

### Delegated File Metadata

```json
{
  "owner": "alice",
  "creator": "service-account",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z",
  "delegated": true,
  "delegation_reason": "automated-backup"
}
```

### Non-Delegated File Metadata

```json
{
  "owner": "alice",
  "creator": "alice",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z"
}
```

### Update Tracking

When a file is updated with delegation:

```json
{
  "owner": "alice",
  "creator": "alice",
  "system": false,
  "created_at": "2025-10-06T10:00:00Z",
  "updated_at": "2025-10-06T11:00:00Z",
  "updated_by": "service-account"
}
```

---

## Audit Trail

All delegation attempts (success + failure) are logged:

### Successful Delegation

```json
{
  "timestamp": "2025-10-06T10:00:00Z",
  "event_type": "impersonation_granted",
  "request_id": "req-123",
  "actor": {
    "user_id": "service-account",
    "groups": ["service-accounts"]
  },
  "principal": {
    "user_id": "alice"
  },
  "delegation": {
    "reason": "automated-backup"
  },
  "operation": "file.create",
  "resource_path": "/data/report.txt",
  "outcome": "success"
}
```

### Failed Delegation

```json
{
  "timestamp": "2025-10-06T10:00:01Z",
  "event_type": "impersonation_denied",
  "request_id": "req-124",
  "actor": {
    "user_id": "attacker",
    "groups": ["user"]
  },
  "principal": {
    "user_id": "admin"
  },
  "reason": "user 'attacker' not in authorized groups for impersonation",
  "remote_ip": "192.168.1.100",
  "outcome": "denied"
}
```

---

## Configuration

### Customizing Impersonation Rules

Edit `/.rego` to customize who can impersonate:

```rego
# Allow specific service accounts
can_impersonate {
    input.user.user_id == "backup-service"
}

# Allow based on custom attribute
can_impersonate {
    input.user.metadata.role == "automation"
}

# Deny specific users explicitly
deny_impersonate["User blocked from impersonation"] {
    input.user.user_id == "blocked-user"
    input.principal != ""
}
```

### Resource-Scoped Delegation (Future)

Limit delegation to specific paths:

```rego
# Only allow backup service to write to /backups
allow {
    input.user.user_id == "backup-service"
    input.principal != ""
    startswith(input.resource.path, "/backups/")
    input.action == "write"
}
```

---

## Best Practices

### 1. Use Service Accounts for Automation

```bash
# Create dedicated service account
curl -X POST "${VFS_URL}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "backup-service",
    "groups": ["service-accounts"]
  }'
```

### 2. Always Provide Delegation Reason

```bash
# Good: Clear reason
vfs-cli import backup.tar.gz /backups/alice/ \
  --on-behalf-of=alice \
  --reason="nightly-backup-job"

# Bad: No reason (harder to audit)
vfs-cli import backup.tar.gz /backups/alice/ \
  --on-behalf-of=alice
```

### 3. Limit Impersonation Scope

Use Rego policies to limit what can be done with delegation:

```rego
# Only allow read operations for support team
allow {
    input.user.groups[_] == "support"
    input.principal != ""
    input.action == "read"
}
```

### 4. Monitor Delegation Patterns

Set up alerts for unusual delegation:
- Too many impersonation denials
- Impersonation by unexpected actors
- Delegation outside business hours

### 5. Rotate Service Account Tokens

```bash
# Regularly rotate tokens for service accounts
# Store in secure vault (e.g., HashiCorp Vault, AWS Secrets Manager)
export SERVICE_TOKEN=$(vault kv get -field=token secret/vfs/service-account)
```

---

## Troubleshooting

### "Impersonation denied" Error

**Cause**: Actor doesn't have impersonate permission

**Solution**: Check actor's groups:
```bash
# Verify user groups
curl "${VFS_URL}/api/v1/auth/whoami" \
  -H "Authorization: Bearer ${TOKEN}"
```

Ensure user is in `service-accounts` or `system-admin` group.

### Metadata Shows Wrong Owner

**Cause**: Delegation header not sent or not validated

**Solution**: Verify headers are set:
```bash
# Check request includes delegation headers
curl -v "${VFS_URL}/api/v1/files?path=/test.txt" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "X-VFS-On-Behalf-Of: alice"
```

### Audit Logs Not Showing Delegation

**Cause**: Logs not configured or filtered

**Solution**: Check server logs for `SECURITY:` prefix:
```bash
# Search for security events
docker logs vfs-service | grep "SECURITY:"
```

---

## Security Considerations

### 1. Least Privilege

Only grant impersonate permission to services that need it:
- Backup services
- Deployment pipelines
- Administrative tools

### 2. Audit Review

Regularly review delegation logs for:
- Unexpected impersonation attempts
- Service accounts acting outside their scope
- Privilege escalation attempts

### 3. Token Security

- Use short-lived tokens for service accounts
- Store tokens in secure vaults
- Rotate tokens regularly
- Never commit tokens to source control

### 4. Network Security

- Use TLS for all API requests
- Implement rate limiting
- Use network policies to restrict service account access

---

## Related Documentation

- [Metadata Guide](./METADATA.md) - Metadata structure and usage
- [Authorization](./AUTHORIZATION.md) - Authorization system
- [Security](./SECURITY.md) - Security best practices
- [API Reference](./API_REFERENCE.md) - Complete API documentation

---

## Examples Repository

See [examples/delegation](../examples/delegation/) for:
- Service account setup
- Rego policy examples
- Automation scripts
- Monitoring queries

---

## Support

For questions or issues:
- GitHub Issues: https://github.com/telnet2/mysql-vfs/issues
- Documentation: https://github.com/telnet2/mysql-vfs/docs
