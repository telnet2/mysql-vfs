#!/bin/bash
set -e

echo "Git Credentials Setup for Docker Builds"
echo "========================================"
echo ""

# Check if .git-credentials exists
if [ -f "$HOME/.git-credentials" ]; then
    echo "✓ Git credentials file exists: $HOME/.git-credentials"

    # Check permissions
    perms=$(stat -f "%OLp" "$HOME/.git-credentials" 2>/dev/null || stat -c "%a" "$HOME/.git-credentials" 2>/dev/null)
    if [ "$perms" = "600" ]; then
        echo "✓ Permissions are secure (600)"
    else
        echo "⚠️  Permissions are $perms (should be 600)"
        echo "   Fixing permissions..."
        chmod 600 "$HOME/.git-credentials"
        echo "✓ Permissions fixed"
    fi

    # Check if code.byted.org is configured
    if grep -q "code.byted.org" "$HOME/.git-credentials"; then
        echo "✓ code.byted.org configured in credentials"
    else
        echo "❌ code.byted.org NOT found in credentials"
        echo ""
        echo "Add this line to $HOME/.git-credentials:"
        echo "  https://YOUR_USERNAME:YOUR_TOKEN@code.byted.org"
    fi
else
    echo "❌ Git credentials file not found: $HOME/.git-credentials"
    echo ""
    echo "Creating credentials file..."
    read -p "Enter your username: " username
    read -sp "Enter your personal access token: " token
    echo ""

    echo "https://$username:$token@code.byted.org" > "$HOME/.git-credentials"
    chmod 600 "$HOME/.git-credentials"

    echo "✓ Credentials file created"
fi

echo ""
echo "Checking git configuration..."

# Check credential helper
helper=$(git config --global --get credential.helper 2>/dev/null || echo "")
if [ "$helper" = "store" ]; then
    echo "✓ Git credential helper is set to 'store'"
else
    echo "⚠️  Git credential helper not set"
    echo "   Setting credential helper..."
    git config --global credential.helper store
    echo "✓ Credential helper configured"
fi

echo ""
echo "Testing git access..."

# Test access (timeout after 5 seconds)
if timeout 5 git ls-remote https://code.byted.org/gopkg/consul.git &>/dev/null; then
    echo "✓ Git access to code.byted.org works!"
else
    echo "❌ Git access test failed"
    echo ""
    echo "Please check:"
    echo "  1. Your token has 'read_repository' permissions"
    echo "  2. The token is not expired"
    echo "  3. Your username and token are correct in ~/.git-credentials"
    echo ""
    echo "You can test manually with:"
    echo "  git ls-remote https://code.byted.org/gopkg/consul.git"
    exit 1
fi

echo ""
echo "========================================"
echo "✅ Setup complete! Ready to build with:"
echo ""
echo "  export DOCKER_BUILDKIT=1"
echo "  docker compose build"
echo ""
