# 9. Deployment Guide

**Deploy MySQL VFS v2 to production**

[← Back: Auth Setup](8_AUTH_SETUP.md) | [Index](0_README.md) | [Next: API Reference →](10_API.md)

---

## Docker Deployment

### 1. Build Image

```bash
docker build -t mysql-vfs:v2.0 .
```

### 2. Production docker-compose.yml

```yaml
version: '3.8'

services:
  vfs-service:
    image: mysql-vfs:v2.0
    environment:
      # Auth (JWT for production)
      - AUTH_PROVIDER=jwt
      - AUTH_JWT_SECRET=${JWT_SECRET}
      - AUTH_JWT_ISSUER=https://auth.yourcompany.com
      - AUTH_ALLOW_ANONYMOUS=false

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
JWT_SECRET=your-production-secret-min-32-chars
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

- [ ] Use JWT or OAuth authentication (not headers!)
- [ ] Set strong `AUTH_JWT_SECRET` (min 32 chars)
- [ ] Enable HTTPS (via nginx/Traefik)
- [ ] Configure database backups
- [ ] Set up monitoring/alerts
- [ ] Configure log aggregation
- [ ] Test failover scenarios
- [ ] Document runbooks

---

[← Back: Auth Setup](8_AUTH_SETUP.md) | [Index](0_README.md) | [Next: API Reference →](10_API.md)
