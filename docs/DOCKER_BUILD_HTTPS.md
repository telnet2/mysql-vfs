# Docker Build with HTTPS Git Authentication

Simplified Docker builds using HTTPS instead of SSH for private Git repositories.

## Quick Start

```bash
# 1. Set up git credentials (one time)
echo "https://YOUR_USERNAME:YOUR_TOKEN@code.byted.org" > ~/.git-credentials
chmod 600 ~/.git-credentials

# 2. Configure git to use credential store
git config --global credential.helper store

# 3. Build with Docker Compose
export DOCKER_BUILDKIT=1
docker compose build
```

## Setup Git Credentials

### Option 1: Manual Setup (Recommended)

```bash
# Create credentials file
cat > ~/.git-credentials << EOF
https://YOUR_USERNAME:YOUR_TOKEN@code.byted.org
EOF

# Secure the file
chmod 600 ~/.git-credentials

# Configure git
git config --global credential.helper store
```

### Option 2: Let Git Store Credentials

```bash
# Configure git first
git config --global credential.helper store

# Then clone any private repo - git will ask for credentials and store them
git clone https://code.byted.org/gopkg/consul.git

# Credentials are now saved in ~/.git-credentials
```

### Get Your Access Token

1. Go to code.byted.org settings
2. Generate a Personal Access Token
3. Give it `read_repository` permissions
4. Copy the token
5. Use it in the credentials file: `https://username:TOKEN@code.byted.org`

## How It Works

### Dockerfiles Use BuildKit Secrets

```dockerfile
# Mount credentials securely (never stored in image)
RUN --mount=type=secret,id=gitconfig,target=/root/.gitconfig \
    --mount=type=secret,id=git-credentials,target=/root/.git-credentials \
    go mod download
```

### Docker Compose Configuration

```yaml
services:
  vfs-service:
    build:
      secrets:
        - gitconfig
        - git-credentials

secrets:
  gitconfig:
    file: ${HOME}/.gitconfig
  git-credentials:
    file: ${HOME}/.git-credentials
```

## Verify Setup

```bash
# Check if credentials file exists
ls -la ~/.git-credentials

# Check if git is configured
git config --global --get credential.helper

# Test git access
git ls-remote https://code.byted.org/gopkg/consul.git
```

## Build Commands

```bash
# Enable BuildKit
export DOCKER_BUILDKIT=1

# Build all services
docker compose build

# Build specific service
docker compose build vfs-service

# Build with no cache
docker compose build --no-cache

# Build with verbose output
BUILDKIT_PROGRESS=plain docker compose build
```

## Troubleshooting

### Error: "secrets: failed to get ... no such file or directory"

**Cause:** Git credentials file doesn't exist

**Fix:**
```bash
# Check if file exists
ls ~/.git-credentials

# If not, create it
echo "https://username:token@code.byted.org" > ~/.git-credentials
chmod 600 ~/.git-credentials
```

### Error: "reading ... git ls-remote: exit status 128"

**Cause:** Invalid credentials or token expired

**Fix:**
1. Generate a new token on code.byted.org
2. Update ~/.git-credentials with new token
3. Test: `git ls-remote https://code.byted.org/gopkg/consul.git`

### Error: "credential.helper not set"

**Fix:**
```bash
git config --global credential.helper store
```

## Security Notes

✅ **Secure:**
- Credentials mounted as secrets (not in image layers)
- Credentials file has 600 permissions (owner only)
- Tokens can be rotated easily
- No credentials in build logs

⚠️ **Important:**
- Never commit `.git-credentials` to git
- Use tokens with minimal permissions
- Rotate tokens regularly
- Don't share credentials

## .git-credentials Format

```
https://username:token@code.byted.org
https://username:token@github.com
https://username:token@gitlab.com
```

Multiple hosts supported, one per line.

## Alternative: Environment Variable

For CI/CD, you can use an environment variable:

```bash
# Set token
export GIT_TOKEN="your-token-here"

# Build (requires Dockerfile modification)
docker build \
  --secret id=git-token,env=GIT_TOKEN \
  -f services/vfs/Dockerfile .
```

## Comparison: SSH vs HTTPS

### SSH (Previous Setup)
- ❌ Requires SSH agent running
- ❌ Requires ssh-keyscan for host keys
- ❌ More complex setup
- ✅ More secure (key-based)

### HTTPS (Current Setup)
- ✅ Simpler setup
- ✅ Works without SSH agent
- ✅ Easier for CI/CD
- ✅ Token-based (easy to rotate)
- ⚠️ Token in file (but secured with 600 permissions)

## Files Modified

- ✅ All Dockerfiles - Use BuildKit secrets instead of SSH
- ✅ docker-compose.yml - Added secrets configuration
- ✅ Removed SSH requirements

## Next Steps

1. Set up git credentials
2. Run `docker compose build`
3. Deploy your services

No SSH agent needed! 🎉
