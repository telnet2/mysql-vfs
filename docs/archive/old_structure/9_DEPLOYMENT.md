# 9. Deployment Guide

**Deploy MySQL VFS v2.1+ to production**

[← Back: Auth Setup](8_AUTH_SETUP.md) | [Index](0_README.md) | [Next: API Reference →](10_API.md)

---

## Docker Deployment

### 1. Build Image

```bash
docker build -t mysql-vfs:v2.1 .
```

### 2. Production docker-compose.yml

```yaml
version: '3.8'

services:
  vfs-service:
    image: mysql-vfs:v2.1
    environment:
      # Auth (file-based for production)
      - AUTH_PROVIDER=file
      - FILE_AUTH_DIRECTORY=/
      - SYSTEM_ADMIN_TOKEN=${SYSTEM_ADMIN_TOKEN}
      - USER_CACHE_TTL_SECONDS=300

      # Database
      - DB_DSN=${DATABASE_DSN}
      - LOG_LEVEL=info

      # Storage
      - S3_BUCKET=vfs-production
      - S3_REGION=us-east-1
      - S3_ENDPOINT=https://s3.amazonaws.com

      # Cache
      - SCHEMA_CACHE_TTL_SECONDS=300
      - POLICY_CACHE_TTL_SECONDS=300

    ports:
      - "8080:8080"
    restart: unless-stopped
    networks:
      - vfs-network
    depends_on:
      - mysql

  mysql:
    image: mysql:8.0
    environment:
      - MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}
      - MYSQL_DATABASE=vfs
    volumes:
      - mysql-data:/var/lib/mysql
    networks:
      - vfs-network
    restart: unless-stopped

volumes:
  mysql-data:

networks:
  vfs-network:
```

### 3. Environment Variables (.env)

```bash
# NEVER commit this file!
SYSTEM_ADMIN_TOKEN=$(openssl rand -hex 32)
DATABASE_DSN=root:password@tcp(mysql:3306)/vfs?parseTime=true
MYSQL_ROOT_PASSWORD=strong-password
```

### 4. Deploy

```bash
docker-compose -f docker-compose.prod.yml up -d
```

---

## Kubernetes Deployment

See [Kubernetes example](https://github.com/telnet2/mysql-vfs/tree/main/k8s) for full manifests.

---

## Production Checklist

- [ ] Use file-based authentication with `.user` files
- [ ] Set strong `SYSTEM_ADMIN_TOKEN` (64 chars, stored in secrets manager)
- [ ] Enable HTTPS (via nginx/Traefik)
- [ ] Configure database backups (MySQL)
- [ ] Set up S3/MinIO for file storage
- [ ] Set up monitoring/alerts (see test count: 103/104 passing)
- [ ] Configure log aggregation
- [ ] Test failover scenarios
- [ ] Document runbooks
- [ ] Protect `.user` files with `.rego` policies

## System Status

**Version:** v2.1+ Production Ready
**Tests:** 103/104 passing (1 flaky concurrency test)
**Special Files:** `.files`, `.user`, `.events`, `.rego`, `.owner`
**Architecture:** Event-driven with lifecycle hooks (implementation: `pkg/events/`)

## Key Implementation Files

- `services/vfs/main.go` - VFS service entry point
- `pkg/middleware/auth.go` - Authentication middleware
- `pkg/middleware/authorization.go` - OPA authorization
- `pkg/domain/user_loader.go` - File-based auth loader
- `pkg/events/handlers/webhook.go` - Webhook event handlers

---

[← Back: Auth Setup](8_AUTH_SETUP.md) | [Index](0_README.md) | [Next: API Reference →](10_API.md)
