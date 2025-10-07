#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${SCRIPT_DIR}/bin"
CONF_DIR="${SCRIPT_DIR}/conf"

# Array to track background process PIDs
declare -a PIDS=()

# Cleanup function
cleanup() {
  echo ""
  echo -e "${YELLOW}Shutting down services...${NC}"

  for pid in "${PIDS[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      echo "  → Stopping process $pid"
      kill -TERM "$pid" 2>/dev/null || true
    fi
  done

  # Wait for processes to exit
  for pid in "${PIDS[@]}"; do
    wait "$pid" 2>/dev/null || true
  done

  echo -e "${GREEN}All services stopped${NC}"
  exit 0
}

# Trap SIGINT and SIGTERM
trap cleanup SIGINT SIGTERM

# Check if config file exists
CONFIG_FILE="${CONF_DIR}/config.yaml"
if [ ! -f "$CONFIG_FILE" ]; then
  if [ -f "${CONF_DIR}/config.local.yaml" ]; then
    CONFIG_FILE="${CONF_DIR}/config.local.yaml"
  elif [ -f "${CONF_DIR}/config.production.yaml" ]; then
    CONFIG_FILE="${CONF_DIR}/config.production.yaml"
  else
    echo -e "${RED}Error: No configuration file found${NC}"
    echo "Please create ${CONF_DIR}/config.yaml or set VFS_CONFIG_FILE environment variable"
    exit 1
  fi
fi

echo -e "${GREEN}=== MySQL VFS Bootstrap ===${NC}"
echo "Using config: ${CONFIG_FILE}"
echo ""

# Default all services to enabled if not specified
: ${ENABLE_VFS_SERVICE:=true}
: ${ENABLE_EVENT_WORKER:=true}
: ${ENABLE_SCHEDULER:=true}
: ${ENABLE_WEBHOOK_ORCHESTRATOR:=true}
: ${ENABLE_NATS:=true}
: ${ENABLE_EVENT_PUBLISHER:=true}

# If NATS is disabled, force event-publisher to be disabled (it requires NATS)
if [ "${ENABLE_NATS}" = "false" ]; then
  if [ "${ENABLE_EVENT_PUBLISHER}" = "true" ]; then
    echo -e "${YELLOW}Warning: NATS is disabled, disabling event-publisher (requires NATS)${NC}"
    ENABLE_EVENT_PUBLISHER=false
  fi
fi

# Start VFS Service
if [ "${ENABLE_VFS_SERVICE}" = "true" ]; then
  echo -e "${BLUE}Starting vfs-service...${NC}"
  if [ "${ENABLE_NATS}" = "false" ]; then
    echo "  → NATS disabled - vfs-service will run without NATS event publishing"
  fi
  "${BIN_DIR}/vfs-service" --conf "${CONFIG_FILE}" &
  PIDS+=($!)
  echo "  → PID: ${PIDS[-1]}"
  sleep 2  # Give it time to start
fi

# Start Event Worker
if [ "${ENABLE_EVENT_WORKER}" = "true" ]; then
  echo -e "${BLUE}Starting event-worker...${NC}"
  "${BIN_DIR}/event-worker" --conf "${CONFIG_FILE}" &
  PIDS+=($!)
  echo "  → PID: ${PIDS[-1]}"
fi

# Start Scheduler
if [ "${ENABLE_SCHEDULER}" = "true" ]; then
  echo -e "${BLUE}Starting scheduler...${NC}"
  "${BIN_DIR}/scheduler" --conf "${CONFIG_FILE}" &
  PIDS+=($!)
  echo "  → PID: ${PIDS[-1]}"
fi

# Start Webhook Orchestrator
if [ "${ENABLE_WEBHOOK_ORCHESTRATOR}" = "true" ]; then
  echo -e "${BLUE}Starting webhook-orchestrator...${NC}"
  "${BIN_DIR}/webhook-orchestrator" --conf "${CONFIG_FILE}" &
  PIDS+=($!)
  echo "  → PID: ${PIDS[-1]}"
fi

# Start Event Publisher (only if NATS is enabled)
if [ "${ENABLE_EVENT_PUBLISHER}" = "true" ]; then
  echo -e "${BLUE}Starting event-publisher...${NC}"
  "${BIN_DIR}/event-publisher" --conf "${CONFIG_FILE}" &
  PIDS+=($!)
  echo "  → PID: ${PIDS[-1]}"
fi

echo ""
echo -e "${GREEN}All enabled services started${NC}"
echo "Press Ctrl+C to stop all services"
echo ""

# Service status table
echo "Service Status:"
echo "  VFS Service:            ${ENABLE_VFS_SERVICE}"
echo "  Event Worker:           ${ENABLE_EVENT_WORKER}"
echo "  Scheduler:              ${ENABLE_SCHEDULER}"
echo "  Webhook Orchestrator:   ${ENABLE_WEBHOOK_ORCHESTRATOR}"
echo "  NATS:                   ${ENABLE_NATS}"
echo "  Event Publisher:        ${ENABLE_EVENT_PUBLISHER}"
echo ""

# Wait for any process to exit
wait -n

# If we get here, one process exited unexpectedly
echo -e "${RED}One or more services exited unexpectedly${NC}"
cleanup
