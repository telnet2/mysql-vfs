# Docker Build Guide

All Dockerfiles have been configured to use HTTPS authentication for private Git repositories.

## Quick Start

```bash
# 1. Run setup script (one time)
./scripts/setup-git-credentials.sh

# 2. Enable BuildKit
export DOCKER_BUILDKIT=1

# 3. Build all services
docker compose build
```

## Setup Git Credentials (Manual)

If you prefer to set up manually:

```bash
# Create credentials file
echo "https://YOUR_USERNAME:YOUR_TOKEN@code.byted.org" > ~/.git-credentials
chmod 600 ~/.git-credentials

# Configure git
git config --global credential.helper store

# Test access
git ls-remote https://code.byted.org/gopkg/consul.git
```

## How It Works

All Dockerfiles now use BuildKit secrets to securely mount git credentials:

```dockerfile
RUN --mount=type=secret,id=gitconfig,target=/root/.gitconfig \
    --mount=type=secret,id=git-credentials,target=/root/.git-credentials \
    go mod download
```

**Security:** Credentials are mounted as secrets and never stored in Docker image layers.

## What Was Fixed

All service Dockerfiles now include:
- ✅ HTTPS authentication (simpler than SSH)
- ✅ BuildKit secret mounts (secure)
- ✅ Git credential helper configuration
- ✅ GOPRIVATE configuration for private modules
- ✅ Build cache mounts for faster builds

## Updated Files

- ✅ services/vfs/Dockerfile
- ✅ services/scheduler/Dockerfile
- ✅ services/webhook-orchestrator/Dockerfile
- ✅ services/event-worker/Dockerfile
- ✅ services/event-publisher/Dockerfile
- ✅ cli/Dockerfile
- ✅ docker-compose.yml (secrets configuration)

## Build Commands

```bash
# Build all services
docker compose build

# Build specific service
docker compose build vfs-service

# Build without cache (clean build)
docker compose build --no-cache scheduler

# Build with verbose output
BUILDKIT_PROGRESS=plain docker compose build
```

## Troubleshooting

### Error: "secrets: failed to get ... no such file or directory"

**Cause:** Git credentials file doesn't exist

**Fix:**
```bash
./scripts/setup-git-credentials.sh
```

### Error: "git ls-remote: exit status 128"

**Cause:** Invalid credentials or expired token

**Fix:**
1. Generate new token on code.byted.org
2. Update `~/.git-credentials`
3. Test: `git ls-remote https://code.byted.org/gopkg/consul.git`

### Error: "unknown flag: --secret"

**Cause:** BuildKit not enabled

**Fix:**
```bash
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1
```

## Get Personal Access Token

1. Go to code.byted.org → Settings → Access Tokens
2. Click "Generate new token"
3. Give it `read_repository` scope
4. Copy the token
5. Use in format: `https://username:TOKEN@code.byted.org`

## Verify Setup

```bash
# Check credentials file
ls -la ~/.git-credentials

# Check git config
git config --global --get credential.helper

# Test git access
git ls-remote https://code.byted.org/gopkg/consul.git

# Check Docker BuildKit
echo $DOCKER_BUILDKIT
```

## CI/CD Integration

For automated builds:

```yaml
# GitHub Actions
- name: Set up git credentials
  run: |
    echo "https://${{ secrets.GIT_USERNAME }}:${{ secrets.GIT_TOKEN }}@code.byted.org" > ~/.git-credentials
    chmod 600 ~/.git-credentials
    git config --global credential.helper store

- name: Build
  run: |
    export DOCKER_BUILDKIT=1
    docker compose build
```

## Security Notes

✅ **Secure:**
- Credentials mounted as BuildKit secrets (not in image)
- File permissions set to 600 (owner only)
- No credentials in build logs or image layers
- Tokens can be rotated easily

⚠️ **Best Practices:**
- Never commit `.git-credentials` to git
- Use tokens with minimal permissions (`read_repository` only)
- Rotate tokens regularly
- Use different tokens for dev/prod

## Files

- **DOCKER_BUILD_HTTPS.md** - Detailed HTTPS authentication guide
- **scripts/setup-git-credentials.sh** - Interactive setup script
- **docs/DOCKER_GIT_AUTH.md** - Comprehensive authentication docs

## Advantages Over SSH

- ✅ No SSH agent required
- ✅ Simpler setup (just credentials file)
- ✅ Easier for CI/CD pipelines
- ✅ Token-based (easy to rotate)
- ✅ Works everywhere (no firewall issues)

## Next Steps

1. Run `./scripts/setup-git-credentials.sh`
2. Build with `docker compose build`
3. Done! 🎉

No SSH agent needed!
