# Service Discovery with Consul

This package provides seamless integration with Consul service discovery using the `consul+` URL scheme.

## Features

- **Automatic service resolution**: Use `consul+<protocol>://service-name` URLs
- **Transparent pass-through**: Regular URLs work without modification
- **Load balancing**: Weighted random selection from available endpoints
- **Connection pooling**: Built-in support via Consul HTTP transport
- **Caching**: 15-second cache with hash-based validation

## URL Format

```
consul+<protocol>://service-name[/path][?query][&consul.options]
```

### Consul Options (Query Parameters)

- `consul.idc` - IDC/datacenter (e.g., `lf`, `hl`)
- `consul.cluster` - Cluster name (e.g., `prod`, `staging`)
- `consul.limit` - Max endpoints to fetch (default: 10)

## Usage Examples

### NATS Connection

```go
import "github.com/telnet2/mysql-vfs/pkg/discovery"

// Regular URL (no Consul)
conn, err := discovery.NewNATSConnection("nats://localhost:4222")

// Consul service discovery
conn, err := discovery.NewNATSConnection("consul+nats://nats-service")

// With cluster selection
conn, err := discovery.NewNATSConnection("consul+nats://nats-cluster?consul.cluster=prod")

// Optional connection (returns nil if URL is empty, no error)
conn, err := discovery.NewOptionalNATSConnection(os.Getenv("NATS_URL"))
```

### HTTP Client

```go
import "github.com/telnet2/mysql-vfs/pkg/discovery"

// Create HTTP client with Consul support
client := discovery.NewHTTPClient()

// Use service names directly in URLs
resp, err := client.Get("http://api-service/v1/users")

// With cluster selection
resp, err := client.Get("http://api-service/search?consul.cluster=prod")
```

### URL Resolution

```go
import "github.com/telnet2/mysql-vfs/pkg/discovery"

// Parse consul+ URL
cu, err := discovery.ParseConsulURL("consul+nats://nats-service?consul.cluster=prod")
fmt.Println(cu.ServiceName)  // "nats-service"
fmt.Println(cu.Scheme)        // "nats"
fmt.Println(cu.Options.Cluster) // "prod"

// Resolve to concrete endpoint
resolved, err := discovery.ResolveConsulURL("consul+https://api-service/v1/users")
// Returns: "https://10.1.2.3:8080/v1/users" (actual endpoint from Consul)

// Pass-through for regular URLs
resolved, err := discovery.ResolveOrPassthrough("https://api.example.com")
// Returns: "https://api.example.com" (unchanged)
```

## Environment Variables

Configure your services using environment variables:

```bash
# NATS with Consul
NATS_URL=consul+nats://nats-service
NATS_URL=consul+nats://nats-cluster?consul.cluster=prod

# NATS without Consul (traditional)
NATS_URL=nats://localhost:4222

# Database with Consul
DATABASE_DSN=consul+mysql://mysql-db/mydb?consul.cluster=prod

# HTTP endpoints
API_URL=consul+https://backend-api
```

## Integration with Services

### services/vfs/main.go

```go
// Supports both consul+ URLs and regular URLs
natsURL := os.Getenv("NATS_URL")
natsConn, err := discovery.NewOptionalNATSConnection(natsURL)
```

### services/event-publisher/main.go

```go
natsURL := getEnv("NATS_URL", "nats://localhost:4222")
nc, err := discovery.NewNATSConnection(natsURL)
```

## Consul Agent Configuration

The Consul library automatically connects to the local Consul agent at:

1. Unix socket: `/opt/tmp/sock/consul.sock` (if available)
2. TCP: `127.0.0.1:2280` (default)
3. Environment variables:
   - `CONSUL_HTTP_HOST`
   - `CONSUL_HTTP_PORT`
   - `BYTED_SD_UDS_PATH` (Unix socket path)

## Testing

Run tests for the discovery package:

```bash
go test ./pkg/discovery/... -v
```

## How It Works

1. **URL Parsing**: `consul+nats://service` → extracts protocol (`nats`) and service name
2. **Consul Lookup**: Query Consul agent for service endpoints
3. **Load Balancing**: Select one endpoint using weighted random selection
4. **Connection**: Replace service name with actual `host:port` and connect

## Comparison

### Without Consul
```bash
NATS_URL=nats://10.1.2.3:4222
```
❌ Hardcoded IP
❌ No load balancing
❌ Manual failover

### With Consul
```bash
NATS_URL=consul+nats://nats-service
```
✅ Service name resolution
✅ Automatic load balancing
✅ Auto failover
✅ Health checking (via Consul)

## Supported Protocols

- `consul+nats://` - NATS messaging
- `consul+http://` - HTTP services
- `consul+https://` - HTTPS services
- `consul+mysql://` - MySQL databases
- Any protocol supported by Consul

## Notes

- Consul endpoints are cached for 15 seconds to reduce lookup overhead
- The Consul agent must be running locally (sidecar pattern)
- Service names should be registered in Consul beforehand
- Regular URLs pass through unchanged (backward compatible)
