# Docker Build with Git Authentication

This guide explains how to build Docker images that need access to private Git repositories (e.g., for Go private modules from `code.byted.org`).

## Prerequisites

- Docker BuildKit enabled (default in Docker 23.0+)
- SSH key configured for git access
- SSH agent running with your key loaded

## Quick Start

### 1. Start SSH Agent (if not already running)

```bash
# Start SSH agent
eval "$(ssh-agent -s)"

# Add your SSH key
ssh-add ~/.ssh/id_rsa  # or your specific key

# Verify key is loaded
ssh-add -l
```

### 2. Build with Docker Compose

```bash
# Enable BuildKit
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Build all services with SSH forwarding
docker compose build

# Or build a specific service
docker compose build vfs-service
```

### 3. Build with Docker CLI

```bash
# Build with SSH agent forwarding
docker build --ssh default -f services/vfs/Dockerfile -t vfs-service .

# Build with cache optimization
docker build --ssh default \
  --cache-from vfs-service:latest \
  -f services/vfs/Dockerfile \
  -t vfs-service .
```

## How It Works

### Dockerfile Configuration

The Dockerfile uses BuildKit's SSH mount feature:

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder

# Install SSH client
RUN apk add --no-cache git openssh-client

# Configure SSH for git access (add known_hosts)
RUN mkdir -p ~/.ssh && \
    chmod 700 ~/.ssh && \
    ssh-keyscan code.byted.org >> ~/.ssh/known_hosts 2>/dev/null || \
    echo "StrictHostKeyChecking accept-new" >> ~/.ssh/config

# Configure git to use SSH for private repos
RUN git config --global url."ssh://git@code.byted.org/".insteadOf "https://code.byted.org/"

# Mount SSH agent socket during go mod download
RUN --mount=type=ssh \
    --mount=type=cache,target=/go/pkg/mod \
    go env -w GOPRIVATE=code.byted.org && \
    go mod download
```

Key features:
- `--mount=type=ssh`: Mounts SSH agent socket (secure, no keys in image)
- `--mount=type=cache`: Caches Go modules between builds
- `GOPRIVATE`: Tells Go to not use proxy for private repos

### Docker Compose Configuration

```yaml
services:
  vfs-service:
    build:
      context: .
      dockerfile: services/vfs/Dockerfile
      ssh:
        - default  # Use default SSH agent
```

## Alternative Methods

### Method 1: SSH Agent Forwarding (Current Setup) ✅ Recommended

**Pros:**
- ✅ Secure (credentials never stored in image)
- ✅ No credential files
- ✅ Works with SSH keys
- ✅ Build cache friendly

**Cons:**
- ❌ Requires SSH agent running
- ❌ Requires BuildKit

**Setup:**
```bash
ssh-add ~/.ssh/id_rsa
docker compose build
```

### Method 2: Git Credentials with BuildKit Secrets

For HTTPS-based authentication:

```dockerfile
# Dockerfile
RUN --mount=type=secret,id=gitconfig,target=/root/.gitconfig \
    --mount=type=secret,id=git-credentials,target=/root/.git-credentials \
    git config --global credential.helper store && \
    go mod download
```

```bash
# Build
docker build \
  --secret id=gitconfig,src=$HOME/.gitconfig \
  --secret id=git-credentials,src=$HOME/.git-credentials \
  -f services/vfs/Dockerfile .
```

**Pros:**
- ✅ Works with HTTPS URLs
- ✅ Secure (credentials not in layers)

**Cons:**
- ❌ More complex setup
- ❌ Requires credential files

### Method 3: Personal Access Token (For CI/CD)

For automated builds (GitHub Actions, GitLab CI, etc.):

```dockerfile
# Dockerfile
ARG GIT_TOKEN
RUN git config --global url."https://oauth2:${GIT_TOKEN}@code.byted.org/".insteadOf "https://code.byted.org/"
```

```bash
# Build
docker build --build-arg GIT_TOKEN=$GITHUB_TOKEN -f services/vfs/Dockerfile .
```

**Pros:**
- ✅ Works in CI/CD
- ✅ Simple setup

**Cons:**
- ⚠️ Token appears in build logs
- ⚠️ Must use `--secret` instead of `--build-arg` for security

**Secure version:**
```dockerfile
RUN --mount=type=secret,id=git_token \
    git config --global url."https://oauth2:$(cat /run/secrets/git_token)@code.byted.org/".insteadOf "https://code.byted.org/"
```

```bash
docker build --secret id=git_token,env=GIT_TOKEN -f services/vfs/Dockerfile .
```

## Troubleshooting

### Error: "Host key verification failed"

**Cause:** SSH doesn't have the git server's host key in known_hosts

**Solution:**
The Dockerfiles now automatically handle this with:
```dockerfile
RUN ssh-keyscan code.byted.org >> ~/.ssh/known_hosts 2>/dev/null || \
    echo "StrictHostKeyChecking accept-new" >> ~/.ssh/config
```

This either adds the host key or tells SSH to accept new hosts automatically.

### Error: "Could not read from remote repository"

**Cause:** SSH agent not running or key not loaded

**Solution:**
```bash
# Check SSH agent
echo $SSH_AUTH_SOCK

# If empty, start agent
eval "$(ssh-agent -s)"

# Add key
ssh-add ~/.ssh/id_rsa

# Verify
ssh-add -l

# Test connection
ssh -T git@code.byted.org
```

### Error: "unknown flag: --ssh"

**Cause:** BuildKit not enabled

**Solution:**
```bash
# Enable BuildKit
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Or set in Docker daemon config (~/.docker/daemon.json)
{
  "features": {
    "buildkit": true
  }
}
```

### Error: "module code.byted.org/gopkg/consul: git ls-remote"

**Cause:** Git URL rewrite not configured or credentials not working

**Solution:**
```bash
# Test SSH access
ssh -T git@code.byted.org

# Check git config in container (add to Dockerfile temporarily)
RUN git config --global --list

# Verify GOPRIVATE is set
RUN go env GOPRIVATE
```

### Error: "no such host code.byted.org"

**Cause:** DNS or network issue in container

**Solution:**
```dockerfile
# Add to Dockerfile
RUN cat /etc/resolv.conf
RUN ping -c 1 code.byted.org
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Build Docker Images

on: [push]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up SSH
        uses: webfactory/ssh-agent@v0.8.0
        with:
          ssh-private-key: ${{ secrets.SSH_PRIVATE_KEY }}

      - name: Build with Docker
        run: |
          docker build --ssh default \
            -f services/vfs/Dockerfile \
            -t vfs-service .
```

### GitLab CI

```yaml
build:
  image: docker:latest
  services:
    - docker:dind
  before_script:
    - eval $(ssh-agent -s)
    - echo "$SSH_PRIVATE_KEY" | tr -d '\r' | ssh-add -
    - mkdir -p ~/.ssh
    - chmod 700 ~/.ssh
  script:
    - docker build --ssh default -f services/vfs/Dockerfile -t vfs-service .
```

## Performance Optimization

### Build Cache

The Dockerfile uses multi-stage cache mounts:

```dockerfile
# Cache Go modules
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Cache build artifacts
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -o app
```

Benefits:
- Go modules cached between builds (faster downloads)
- Build artifacts cached (faster compilation)
- Cache survives even if Dockerfile changes

### .dockerignore

Create `.dockerignore` to exclude unnecessary files:

```
.git
.github
*.md
docs/
citest/
.DS_Store
*.log
tmp/
```

This speeds up the Docker build context transfer.

## Security Best Practices

1. ✅ **Use SSH agent forwarding** (credentials never in image)
2. ✅ **Never use `--build-arg` for secrets** (visible in history)
3. ✅ **Use `--mount=type=secret`** for sensitive data
4. ✅ **Scan images for leaked credentials**: `docker history vfs-service`
5. ✅ **Use multi-stage builds** (final image doesn't contain build tools)
6. ✅ **Rotate SSH keys regularly**

## Local Development vs Production

### Local Development
```bash
# Use host SSH agent
docker compose build
```

### Production Build
```bash
# Use dedicated deployment key
docker build --ssh default \
  --secret id=git_token,env=DEPLOY_TOKEN \
  -f services/vfs/Dockerfile .
```

## Resources

- [Docker BuildKit SSH mount](https://docs.docker.com/build/building/secrets/#ssh-mounts)
- [Go private modules](https://go.dev/ref/mod#private-modules)
- [Docker secrets](https://docs.docker.com/build/building/secrets/)
