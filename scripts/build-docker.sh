#!/bin/bash
set -e

# Docker Build Script with SSH Authentication
# This script helps build Docker images with proper SSH agent forwarding

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if BuildKit is enabled
check_buildkit() {
    if [ -z "$DOCKER_BUILDKIT" ]; then
        echo -e "${YELLOW}Warning: DOCKER_BUILDKIT not set. Enabling...${NC}"
        export DOCKER_BUILDKIT=1
        export COMPOSE_DOCKER_CLI_BUILD=1
    fi
}

# Check if SSH agent is running and has keys
check_ssh_agent() {
    if [ -z "$SSH_AUTH_SOCK" ]; then
        echo -e "${RED}Error: SSH agent is not running${NC}"
        echo "Start SSH agent with: eval \"\$(ssh-agent -s)\""
        exit 1
    fi

    if ! ssh-add -l &>/dev/null; then
        echo -e "${RED}Error: No SSH keys loaded in agent${NC}"
        echo "Add your key with: ssh-add ~/.ssh/id_rsa"
        exit 1
    fi

    echo -e "${GREEN}✓ SSH agent is running with keys loaded${NC}"
    ssh-add -l
}

# Test SSH connection to git server
test_ssh_connection() {
    echo -e "\n${YELLOW}Testing SSH connection to code.byted.org...${NC}"
    if ssh -T git@code.byted.org 2>&1 | grep -q "successfully authenticated"; then
        echo -e "${GREEN}✓ SSH connection successful${NC}"
    else
        echo -e "${YELLOW}Note: If you see 'Permission denied', check your SSH keys${NC}"
    fi
}

# Build specific service
build_service() {
    local service=$1
    echo -e "\n${GREEN}Building $service...${NC}"
    docker compose build "$service"
}

# Build all services
build_all() {
    echo -e "\n${GREEN}Building all services...${NC}"
    docker compose build
}

# Show usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS] [SERVICE]

Build Docker images with SSH authentication for private Git repositories.

Options:
    -h, --help          Show this help message
    -a, --all           Build all services
    -c, --check-only    Only check prerequisites, don't build
    -t, --test          Test SSH connection to git server

Services:
    vfs-service
    webhook-orchestrator
    event-worker
    scheduler
    event-publisher
    cli

Examples:
    $0                          # Build all services
    $0 vfs-service              # Build specific service
    $0 --check-only             # Check prerequisites
    $0 --test                   # Test SSH connection

EOF
}

# Main
main() {
    local service=""
    local check_only=false
    local test_only=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                exit 0
                ;;
            -a|--all)
                service="all"
                shift
                ;;
            -c|--check-only)
                check_only=true
                shift
                ;;
            -t|--test)
                test_only=true
                shift
                ;;
            *)
                service=$1
                shift
                ;;
        esac
    done

    echo -e "${GREEN}Docker Build with SSH Authentication${NC}"
    echo "======================================"

    # Check prerequisites
    check_buildkit
    check_ssh_agent

    if [ "$test_only" = true ]; then
        test_ssh_connection
        exit 0
    fi

    if [ "$check_only" = true ]; then
        echo -e "\n${GREEN}All prerequisites satisfied!${NC}"
        exit 0
    fi

    # Test connection
    test_ssh_connection

    # Build
    if [ -z "$service" ] || [ "$service" = "all" ]; then
        build_all
    else
        build_service "$service"
    fi

    echo -e "\n${GREEN}✓ Build completed successfully!${NC}"
}

main "$@"
