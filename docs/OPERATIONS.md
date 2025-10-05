# Operations Guide

**Deployment, Configuration, Development, and Troubleshooting**

---

## Table of Contents

- [Deployment](#deployment)
- [Configuration Reference](#configuration-reference)
- [Development](#development)
- [Monitoring & Troubleshooting](#monitoring--troubleshooting)
- [Implementation Status](#implementation-status)

---

## Deployment

### Docker Deployment

**Prerequisites:**
- Docker 20.10+
- Docker Compose 2.0+

**Quick Start:**

```bash
# Clone repository
git clone https://github.com/your-org/mysql-vfs.git
cd mysql-vfs

# Set environment variables
cp .env.example .env
# Edit .env with your configuration

# Start services
docker-compose up -d

# Check status
docker-compose ps

# View logs
docker-compose logs -f vfs
```

**docker-compose.yml:**

```yaml
version: '3.8'

services:
  vfs:
    image: mysql-vfs:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=mysql://vfs:password@mysql:3306/vfs
      - S3_ENDPOINT=http://minio:9000
      - SYSTEM_ADMIN_TOKEN=${SYSTEM_ADMIN_TOKEN}
    depends_on:
      - mysql
      - minio

  mysql:
    image: mysql:8.0
    environment:
      - MYSQL_ROOT_PASSWORD=rootpassword
      - MYSQL_DATABASE=vfs
      - MYSQL_USER=vfs
      - MYSQL_PASSWORD=password
    volumes:
      - mysql_data:/var/lib/mysql

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      - MINIO_ROOT_USER=minioadmin
      - MINIO_ROOT_PASSWORD=minioadmin
    volumes:
      - minio_data:/data
    ports:
      - "9000:9000"
      - "9001:9001"

volumes:
  mysql_data:
  minio_data:
```

---

### Kubernetes Deployment

**Prerequisites:**
- Kubernetes 1.20+
- kubectl configured
- Helm 3.0+ (optional)

**Basic Deployment:**

```yaml
# vfs-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql-vfs
spec:
  replicas: 3
  selector:
    matchLabels:
      app: mysql-vfs
  template:
    metadata:
      labels:
        app: mysql-vfs
    spec:
      containers:
      - name: vfs
        image: mysql-vfs:latest
        ports:
        - containerPort: 8080
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: vfs-secrets
              key: database-url
        - name: SYSTEM_ADMIN_TOKEN
          valueFrom:
            secretKeyRef:
              name: vfs-secrets
              key: admin-token
        - name: S3_ENDPOINT
          value: "http://minio-service:9000"
        - name: S3_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: vfs-secrets
              key: s3-access-key
        - name: S3_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: vfs-secrets
              key: s3-secret-key
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5

---
apiVersion: v1
kind: Service
metadata:
  name: mysql-vfs-service
spec:
  selector:
    app: mysql-vfs
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

**Create Secrets:**

```bash
kubectl create secret generic vfs-secrets \
  --from-literal=database-url='mysql://user:pass@mysql:3306/vfs' \
  --from-literal=admin-token='your-secure-token' \
  --from-literal=s3-access-key='minioadmin' \
  --from-literal=s3-secret-key='minioadmin'
```

**Deploy:**

```bash
kubectl apply -f vfs-deployment.yaml
kubectl apply -f vfs-service.yaml

# Check status
kubectl get pods -l app=mysql-vfs
kubectl logs -l app=mysql-vfs -f
```

**Scaling:**

```bash
# Horizontal scaling (stateless design supports this)
kubectl scale deployment mysql-vfs --replicas=5

# Autoscaling
kubectl autoscale deployment mysql-vfs \
  --cpu-percent=70 \
  --min=3 \
  --max=10
```

---

### Production Checklist

**Before Going Live:**

- [ ] Set strong `SYSTEM_ADMIN_TOKEN` (32+ chars)
- [ ] Enable HTTPS (reverse proxy or ingress)
- [ ] Configure MySQL with persistent storage
- [ ] Configure S3 with encryption
- [ ] Set up database backups
- [ ] Configure resource limits (CPU, memory)
- [ ] Set up monitoring (Prometheus, Grafana)
- [ ] Configure log aggregation (ELK, Loki)
- [ ] Set up alerts (failed auth, errors)
- [ ] Review and test disaster recovery
- [ ] Document runbook procedures

**Security:**

- [ ] Never commit secrets to git
- [ ] Use environment variables for all secrets
- [ ] Enable HTTPS/TLS
- [ ] Rotate tokens quarterly
- [ ] Review policies before deployment
- [ ] Enable audit logging
- [ ] Restrict network access (firewall rules)

---

## Configuration Reference

### Environment Variables

**Required:**

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | MySQL connection string | `mysql://user:pass@host:3306/db` |
| `S3_ENDPOINT` | S3-compatible endpoint | `http://minio:9000` |
| `S3_BUCKET` | S3 bucket name | `vfs-storage` |
| `S3_ACCESS_KEY` | S3 access key | `minioadmin` |
| `S3_SECRET_KEY` | S3 secret key | `minioadmin` |

**Authentication:**

| Variable | Description | Default |
|----------|-------------|---------|
| `SYSTEM_ADMIN_TOKEN` | System admin bearer token | *(required)* |
| `SYSTEM_ADMIN_ID` | System admin user ID | `system-admin` |
| `AUTH_JWT_SECRET` | JWT signing secret | *(none)* |
| `AUTH_JWT_ISSUER` | Expected JWT issuer | *(none)* |
| `AUTH_JWT_AUDIENCE` | Expected JWT audience | *(none)* |

**Caching:**

| Variable | Description | Default |
|----------|-------------|---------|
| `POLICY_CACHE_TTL_SECONDS` | Policy cache duration | `300` (5 min) |
| `USER_CACHE_TTL_SECONDS` | User cache duration | `300` (5 min) |
| `FILES_CACHE_TTL_SECONDS` | Files config cache duration | `300` (5 min) |

**Storage:**

| Variable | Description | Default |
|----------|-------------|---------|
| `MAX_FILE_SIZE` | Max file size (bytes) | `104857600` (100MB) |
| `S3_REGION` | S3 region | `us-east-1` |
| `S3_USE_SSL` | Use HTTPS for S3 | `false` |
| `STORAGE_THRESHOLD` | MySQL vs S3 split (bytes) | `1048576` (1MB) |

**Server:**

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `LOG_LEVEL` | Logging level | `info` |
| `DEBUG_MODE` | Enable debug logging | `false` |

**Database:**

| Variable | Description | Default |
|----------|-------------|---------|
| `DB_MAX_OPEN_CONNS` | Max open connections | `25` |
| `DB_MAX_IDLE_CONNS` | Max idle connections | `5` |
| `DB_CONN_MAX_LIFETIME` | Connection max lifetime | `5m` |

---

### Example Configurations

**Development:**

```bash
export DATABASE_URL="mysql://root:password@localhost:3306/vfs_dev"
export S3_ENDPOINT="http://localhost:9000"
export S3_BUCKET="vfs-dev"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
export SYSTEM_ADMIN_TOKEN="dev-token-not-secure"
export LOG_LEVEL="debug"
export DEBUG_MODE="true"
```

**Staging:**

```bash
export DATABASE_URL="mysql://vfs:$DB_PASSWORD@mysql-staging:3306/vfs"
export S3_ENDPOINT="https://s3.staging.example.com"
export S3_BUCKET="vfs-staging"
export S3_ACCESS_KEY="$S3_ACCESS_KEY"
export S3_SECRET_KEY="$S3_SECRET_KEY"
export SYSTEM_ADMIN_TOKEN="$STAGING_ADMIN_TOKEN"
export AUTH_JWT_SECRET="$JWT_SECRET"
export LOG_LEVEL="info"
```

**Production:**

```bash
export DATABASE_URL="mysql://vfs:$DB_PASSWORD@mysql-prod:3306/vfs"
export S3_ENDPOINT="https://s3.amazonaws.com"
export S3_BUCKET="vfs-production"
export S3_REGION="us-west-2"
export S3_USE_SSL="true"
export S3_ACCESS_KEY="$S3_ACCESS_KEY"
export S3_SECRET_KEY="$S3_SECRET_KEY"
export SYSTEM_ADMIN_TOKEN="$PROD_ADMIN_TOKEN"
export AUTH_JWT_SECRET="$JWT_SECRET"
export AUTH_JWT_ISSUER="https://auth.example.com"
export LOG_LEVEL="warn"
export DEBUG_MODE="false"

# Performance tuning
export DB_MAX_OPEN_CONNS="100"
export DB_MAX_IDLE_CONNS="10"
export POLICY_CACHE_TTL_SECONDS="600"  # 10 minutes for prod
```

---

## Development

### Prerequisites

- Go 1.21+
- MySQL 8.0+
- MinIO or S3-compatible storage
- Git

### Setup Local Environment

```bash
# Clone repository
git clone https://github.com/your-org/mysql-vfs.git
cd mysql-vfs

# Install dependencies
go mod download

# Start dependencies (MySQL + MinIO)
docker-compose up -d mysql minio

# Wait for MySQL to be ready
sleep 10

# Run migrations
go run cmd/server/main.go migrate

# Set environment variables
export DATABASE_URL="mysql://root:password@localhost:3306/vfs"
export S3_ENDPOINT="http://localhost:9000"
export S3_BUCKET="vfs-dev"
export S3_ACCESS_KEY="minioadmin"
export S3_SECRET_KEY="minioadmin"
export SYSTEM_ADMIN_TOKEN="dev-token"

# Run server
go run cmd/server/main.go
```

Server starts on `http://localhost:8080`

---

### Project Structure

```
mysql-vfs/
├── cmd/
│   └── server/
│       └── main.go              # Entry point
├── pkg/
│   ├── domain/                  # Business logic
│   │   ├── file_service.go
│   │   ├── directory_service.go
│   │   ├── policy_loader.go
│   │   ├── user_loader.go
│   │   └── *_loader.go
│   ├── middleware/              # HTTP middleware
│   │   ├── auth.go
│   │   ├── authorization.go
│   │   └── default_policy.go
│   ├── persistence/             # Data access
│   │   └── db/mysql/
│   ├── events/                  # Event system
│   │   ├── types.go
│   │   └── handlers/
│   └── setup/                   # Bootstrap
├── citest/                      # Integration tests
│   ├── fixtures/
│   └── *_test.go
├── docs/                        # Documentation
└── docker-compose.yml
```

**See:** `DESIGN.md` for detailed architecture

---

### Running Tests

**Unit Tests:**

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test ./pkg/domain/...

# Verbose
go test -v ./...
```

**Integration Tests:**

```bash
# Prerequisites: Docker running

cd citest

# Install ginkgo
go install github.com/onsi/ginkgo/v2/ginkgo

# Run tests
ginkgo -v

# Run specific test
ginkgo -v --focus="Files Validation"

# With coverage
ginkgo -v --cover
```

**Integration tests** use real MySQL + MinIO (via Docker).

**Test Files:**
- Unit tests: `pkg/**/*_test.go`
- Integration tests: `citest/*_test.go`

---

### Code Style

**Linting:**

```bash
# Install golangci-lint
brew install golangci-lint  # macOS
# or: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run

# Auto-fix
golangci-lint run --fix
```

**Formatting:**

```bash
# Format code
go fmt ./...

# Check formatting
gofmt -l .
```

---

### Adding Features

**1. Define Domain Logic:**

```go
// pkg/domain/my_feature.go
package domain

type MyFeatureService struct {
    fileRepo FileRepository
}

func (s *MyFeatureService) DoSomething(ctx context.Context) error {
    // Business logic here
    return nil
}
```

**2. Add Persistence (if needed):**

```go
// pkg/persistence/db/mysql/my_feature.go
package mysql

func (r *MyFeatureRepository) Save(ctx context.Context, data) error {
    return r.db.Create(data).Error
}
```

**3. Add HTTP Handler:**

```go
// cmd/server/handlers/my_feature.go
package handlers

func HandleMyFeature(ctx context.Context, c *app.RequestContext) {
    // Extract request
    // Call service
    // Return response
}
```

**4. Add Tests:**

```go
// pkg/domain/my_feature_test.go
func TestMyFeature(t *testing.T) {
    // Unit test
}

// citest/my_feature_test.go
var _ = Describe("My Feature E2E", func() {
    It("should work end-to-end", func() {
        // Integration test
    })
})
```

**See:** `DESIGN.md` for architecture patterns

---

### Debugging

**Enable Debug Logging:**

```bash
export LOG_LEVEL="debug"
export DEBUG_MODE="true"
go run cmd/server/main.go
```

**Debug with Delve:**

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Run with debugger
dlv debug cmd/server/main.go

# Set breakpoint
(dlv) break pkg/middleware/authorization.go:100
(dlv) continue
```

**Debug Policies:**

```bash
# Add trace to policy
# pkg/middleware/default_policy.go or .rego file

allow {
    trace(sprintf("User: %v", [input.user]))
    input.user.groups[_] == "admin"
}

# View traces in logs
```

---

## Monitoring & Troubleshooting

### Health Checks

**Endpoints:**

```bash
# Liveness probe (is server running?)
curl http://localhost:8080/health

# Readiness probe (can handle traffic?)
curl http://localhost:8080/ready
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2025-10-05T12:00:00Z"
}
```

**Configure in Kubernetes:**
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

---

### Logging

**Log Levels:**
- `debug` - Verbose, includes traces
- `info` - Normal operations
- `warn` - Warnings, recoverable errors
- `error` - Errors, failed operations

**Configure:**
```bash
export LOG_LEVEL="info"
```

**Log Format:**
```
2025-10-05T12:00:00Z [INFO] File created: /data/example.json
2025-10-05T12:00:01Z [WARN] Policy cache miss for /data
2025-10-05T12:00:02Z [ERROR] S3 upload failed: connection timeout
```

**Centralized Logging:**

**Option 1: Docker logs**
```bash
docker-compose logs -f vfs
```

**Option 2: ELK Stack**
```yaml
# docker-compose.yml
services:
  vfs:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"

  filebeat:
    image: elastic/filebeat:8.0.0
    # Configure to ship logs to Elasticsearch
```

**Option 3: Loki**
```yaml
# Promtail → Loki → Grafana
```

---

### Metrics

**Built-in Metrics:**

- File operations (create, read, update, delete)
- Authorization checks (succeeded, failed)
- Validation checks (succeeded, failed)
- Cache hits/misses
- Response times

**Prometheus Integration:**

```bash
# Metrics endpoint
curl http://localhost:8080/metrics
```

**Example metrics:**
```
# HELP vfs_file_operations_total Total file operations
# TYPE vfs_file_operations_total counter
vfs_file_operations_total{operation="create",status="success"} 1234
vfs_file_operations_total{operation="create",status="failure"} 5

# HELP vfs_authz_checks_total Authorization checks
# TYPE vfs_authz_checks_total counter
vfs_authz_checks_total{result="allowed"} 5678
vfs_authz_checks_total{result="denied"} 12
```

**Grafana Dashboard:**

Import dashboard from `docs/grafana-dashboard.json` (if exists)

---

### Common Issues

**1. Access Denied (403)**

**Symptoms:**
```json
{"error": "forbidden: access denied by policy"}
```

**Debug:**
```bash
# Check user groups
curl http://localhost:8080/api/v1/files/.user \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Check policy
curl http://localhost:8080/api/v1/files/data/.rego \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Test policy with OPA
opa eval --data policy.rego --input input.json 'data.vfs.authz.allow'
```

**Common causes:**
- User not in required group
- Policy syntax error
- Cache stale (wait 5 minutes)

---

**2. File Upload Fails**

**Symptoms:**
```json
{"error": "validation failed: email is required"}
```

**Debug:**
```bash
# Check validation rules
curl http://localhost:8080/api/v1/files/data/.files \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Verify file content matches schema
cat upload.json | jq .
```

**Common causes:**
- Content doesn't match JSON schema
- File size exceeds MAX_FILE_SIZE
- S3 connection failed

---

**3. Database Connection Failed**

**Symptoms:**
```
[ERROR] Failed to connect to database: dial tcp: connection refused
```

**Debug:**
```bash
# Test database connection
mysql -h localhost -u vfs -p -D vfs

# Check environment variable
echo $DATABASE_URL

# Check MySQL is running
docker-compose ps mysql
docker-compose logs mysql
```

**Common causes:**
- MySQL not started
- Wrong credentials
- Network issues

---

**4. S3 Upload Failed**

**Symptoms:**
```
[ERROR] S3 upload failed: connection timeout
```

**Debug:**
```bash
# Test S3 connection
curl http://localhost:9000/minio/health/ready

# Check credentials
echo $S3_ACCESS_KEY
echo $S3_SECRET_KEY

# Test upload manually
aws s3 cp test.txt s3://vfs-storage/ \
  --endpoint-url http://localhost:9000
```

**Common causes:**
- MinIO not running
- Wrong credentials
- Network issues
- Bucket doesn't exist

---

**5. Policy Not Taking Effect**

**Symptoms:**
- Updated policy but still getting old behavior

**Debug:**
```bash
# Check file was updated
curl http://localhost:8080/api/v1/files/data/.rego?metadata=true \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Wait for cache expiry (5 minutes)
# or restart service
docker-compose restart vfs
```

**Common causes:**
- Cache TTL (5 minutes)
- Updated wrong directory's policy
- Syntax error (policy fails to load)

---

### Performance Tuning

**Database:**

```bash
# Increase connection pool
export DB_MAX_OPEN_CONNS="100"
export DB_MAX_IDLE_CONNS="20"

# Add indexes (see pkg/persistence/db/mysql/models.go)
```

**Caching:**

```bash
# Increase cache TTL (reduce DB queries)
export POLICY_CACHE_TTL_SECONDS="600"  # 10 minutes

# Use Redis for shared cache (future enhancement)
```

**S3:**

```bash
# Use CDN for large files (future)
# Enable S3 multipart upload (future)
```

**Horizontal Scaling:**

```bash
# Scale replicas (stateless design)
kubectl scale deployment mysql-vfs --replicas=10

# Use load balancer
# Each instance has its own cache (5-min staleness acceptable)
```

---

## Implementation Status

### Complete Features ✅

- **File Operations:** Create, read, update, delete
- **Directory Operations:** Create, list, delete
- **Authorization:** OPA policies, group-based access
- **Authentication:** System admin, file-based (.user), JWT
- **Content Validation:** JSON schema validation
- **Events:** Lifecycle events, webhooks, log handler
- **Ownership:** Owner-based access control
- **Versioning:** File version tracking
- **Storage:** MySQL + S3 dual storage

### Partial Features ⚠️

- **Password Authentication:** Hash storage exists, login endpoint planned
- **Event Retry:** Webhooks fire once, no retry on failure
- **Search:** No full-text search

### Future Enhancements 🔮

**Q1 2026:**
- Password authentication login endpoint
- Webhook retry logic
- Redis cache for multi-instance deployments

**Q2 2026:**
- Advanced event system (async queue)
- Policy versioning
- Audit mode (log without blocking)

**Q3 2026:**
- Full-text search
- Multi-region support
- Performance optimizations

**See:** `DESIGN.md` for detailed roadmap

---

### Known Limitations

1. **No Distributed Locking**
   - Concurrent edits not prevented
   - Workaround: Use external lock service

2. **Cache Invalidation Delay**
   - Up to 5 minutes staleness
   - Workaround: Reduce TTL or restart

3. **No Built-in Encryption**
   - Files stored unencrypted in S3
   - Workaround: Client-side encryption or S3 encryption

4. **Single-Region Only**
   - No cross-region replication
   - Workaround: Manual S3 replication

---

## Backup & Recovery

### Database Backup

**MySQL Backup:**

```bash
# Backup
docker-compose exec mysql mysqldump -u root -p vfs > backup.sql

# Restore
docker-compose exec -T mysql mysql -u root -p vfs < backup.sql
```

**Automated Backups:**

```bash
# Cron job (daily at 2am)
0 2 * * * docker-compose exec mysql mysqldump -u root -p vfs > /backups/vfs-$(date +\%Y\%m\%d).sql
```

### S3 Backup

**MinIO:**

```bash
# Backup bucket
mc mirror minio/vfs-storage /backups/s3-data

# Restore
mc mirror /backups/s3-data minio/vfs-storage
```

**AWS S3:**

```bash
# Enable versioning
aws s3api put-bucket-versioning \
  --bucket vfs-production \
  --versioning-configuration Status=Enabled

# Cross-region replication
# (configure via AWS console or CLI)
```

### Disaster Recovery

**Full Recovery:**

1. Restore MySQL database
2. Restore S3 bucket
3. Recreate `.user`, `.rego` files (if needed)
4. Restart service

**RPO/RTO:**
- RPO: 24 hours (daily backups)
- RTO: 1 hour (manual restore)

---

## Security Considerations

**Production Deployment:**

✅ Use HTTPS (reverse proxy or ingress)
✅ Rotate tokens quarterly
✅ Enable MySQL SSL
✅ Enable S3 encryption
✅ Restrict network access
✅ Enable audit logging
✅ Monitor failed auth attempts
✅ Keep dependencies updated

**See:** `SECURITY.md` for detailed security guide

---

## Support

**Documentation:**
- `README.md` - Quick start
- `USER_GUIDE.md` - Features and API
- `SECURITY.md` - Authentication and authorization
- `DESIGN.md` - Architecture and design decisions

**Issues:**
- File bug reports on GitHub
- Include logs and reproduction steps

**Community:**
- Discussions on GitHub Discussions
- Slack/Discord (if available)

---

**Quick Troubleshooting:**
1. Check logs: `docker-compose logs -f vfs`
2. Check health: `curl http://localhost:8080/health`
3. Test auth: Use system admin token
4. Test policy: Use OPA CLI
5. Restart: `docker-compose restart vfs`
