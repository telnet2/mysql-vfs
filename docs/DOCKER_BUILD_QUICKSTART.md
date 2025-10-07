# Docker Build Quick Start

Building Docker images that access private Git repositories (e.g., `code.byted.org/gopkg/consul`).

## TL;DR

```bash
# 1. Start SSH agent and add key
eval "$(ssh-agent -s)"
ssh-add ~/.ssh/id_rsa

# 2. Build with helper script
./scripts/build-docker.sh

# OR use docker compose directly
export DOCKER_BUILDKIT=1
docker compose build
```

## Prerequisites Check

```bash
# Check if SSH agent is running
echo $SSH_AUTH_SOCK

# Check if keys are loaded
ssh-add -l

# Test git access
ssh -T git@code.byted.org

# Check BuildKit
docker buildx version
```

## Build Commands

### Option 1: Helper Script (Recommended)

```bash
# Check prerequisites
./scripts/build-docker.sh --check-only

# Build all services
./scripts/build-docker.sh

# Build specific service
./scripts/build-docker.sh vfs-service

# Test SSH connection only
./scripts/build-docker.sh --test
```

### Option 2: Docker Compose

```bash
# Enable BuildKit
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Build all
docker compose build

# Build specific service
docker compose build vfs-service

# Build with no cache
docker compose build --no-cache vfs-service
```

### Option 3: Docker CLI

```bash
# Build with SSH forwarding
docker build --ssh default \
  -f services/vfs/Dockerfile \
  -t vfs-service .

# Build with cache optimization
docker build --ssh default \
  --cache-from vfs-service:latest \
  -t vfs-service:latest \
  -f services/vfs/Dockerfile .
```

## Troubleshooting

### ❌ "Could not read from remote repository"

```bash
# Start SSH agent
eval "$(ssh-agent -s)"

# Add your key
ssh-add ~/.ssh/id_rsa

# Verify
ssh-add -l
```

### ❌ "unknown flag: --ssh"

```bash
# Enable BuildKit
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Or permanently in ~/.docker/daemon.json
{
  "features": {
    "buildkit": true
  }
}
```

### ❌ Build is slow

```bash
# Check .dockerignore exists
cat .dockerignore

# Use cache mounts (already in Dockerfile)
# Clear old containers
docker system prune -a
```

## What's Happening?

1. **SSH Agent**: Your SSH keys are forwarded to the build process
2. **Git Config**: Dockerfile rewrites URLs to use SSH
3. **Go Modules**: Downloads private modules using SSH credentials
4. **Cache**: Go modules are cached for faster subsequent builds
5. **Security**: SSH keys never stored in the Docker image

## Files Modified

- `services/*/Dockerfile` - Added SSH mount support
- `docker-compose.yml` - Added SSH forwarding config
- `.dockerignore` - Optimized build context
- `scripts/build-docker.sh` - Helper build script

## Architecture

```
Host Machine              Docker Build
┌─────────────┐          ┌─────────────┐
│             │          │             │
│ SSH Agent   │ ────────>│ --mount=    │
│ (keys)      │ forward  │ type=ssh    │
│             │          │             │
└─────────────┘          └─────────────┘
                                │
                                ↓
                         ┌─────────────┐
                         │ Git Clone   │
                         │ via SSH     │
                         └─────────────┘
                                │
                                ↓
                         ┌─────────────┐
                         │ go mod      │
                         │ download    │
                         └─────────────┘
```

## Performance Tips

1. **Use build cache**: Don't use `--no-cache` unless necessary
2. **Keep .dockerignore updated**: Reduce build context size
3. **Multi-stage builds**: Already implemented
4. **Layer caching**: Go modules and build cache are cached
5. **Parallel builds**: `docker compose build` builds in parallel

## Production Deployment

For CI/CD or production:

```bash
# Use deployment key (not personal key)
export SSH_PRIVATE_KEY="$(cat ~/.ssh/deploy_key)"

# Build
docker build --ssh default \
  -f services/vfs/Dockerfile \
  -t registry.example.com/vfs-service:latest .

# Push
docker push registry.example.com/vfs-service:latest
```

## Next Steps

- See [DOCKER_GIT_AUTH.md](DOCKER_GIT_AUTH.md) for detailed documentation
- Configure CI/CD: [GitHub Actions](#), [GitLab CI](#)
- Set up image registry and automated builds
- Configure secrets for production deployments
