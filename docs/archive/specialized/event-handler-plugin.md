# Event Handler Plugin Design

## Goals
- Allow teams to extend lifecycle events with custom logic without modifying the core VFS service.
- Support both synchronous (veto-capable) and asynchronous handlers through a pluggable contract.
- Provide language/runtime flexibility while maintaining sandboxing and security boundaries.
- Keep configuration and deployment simple enough for ops teams that already manage `.events` files.

## Constraints & Principles
1. **Backward compatible**: Existing built-in handlers (webhook, log, metrics) remain first-class. Plugins are additive.
2. **Isolation first**: Plugins may run arbitrary code; we should isolate them with well-defined IPC and resource limits.
3. **Observability**: Execution metadata (duration, success/failure, veto, error messages) must flow back for logging/metrics.
4. **Operational safety**: Pluggable components must support hot reloads, version pinning, and graceful shutdown.

## Architecture Overview
```
                     ┌─────────────────────────────┐
                     │     VFS Lifecycle Trigger    │
                     │  (existing domain layer)     │
                     └────────────┬─────────────────┘
                                  │
                                  ▼
                ┌────────────────────────────────────┐
                │      Handler Registry (extended)   │
                │  - Built-in adapters               │
                │  - Plugin adapter shim             │
                └────────────┬───────────────────────┘
                                 │
         ┌───────────────────────┴────────────────────────┐
         │                                                │
┌────────▼────────┐                              ┌────────▼────────┐
│ Built-in Handler│                              │  Plugin Sandbox │
│ (webhook/log/   │   gRPC / UNIX socket RPC     │  Runtime        │
│ metrics)        │ <──────────────────────────> │  (per language) │
└─────────────────┘                              └─────────────────┘
```

### Key Components
- **Plugin Descriptor**: JSON or YAML alongside the binary/script specifying name, version, runtime, entrypoint, health checks, timeout, max concurrency, and whether synchronous/veto is enabled.
- **Plugin Host**: Lightweight process (managed by VFS) that spawns/monitors plugin runtimes and exposes a uniform RPC interface (gRPC/Unix domain socket).
- **Plugin SDKs**: Per language libraries (Go, Python first) to simplify implementing the required RPC contract.
- **Handler Registry Extension**: Registers plugin handlers at bootstrap, mapping handler type `"plugin:<name>"` in `.events` to the appropriate plugin host.

## Plugin Lifecycle
1. **Discovery**
   - Plugins live under `plugins/<plugin-name>/` with `plugin.json` descriptor.
   - On service start or on-demand reload (admin API), the VFS service scans descriptors.
2. **Validation**
   - Validate schema, required fields, supported runtime, signature (optional for signed plugins).
3. **Startup**
   - For each enabled plugin, spawn the runtime (e.g., execute `./plugin-binary` or `python main.py`) within a supervised sandbox.
   - Establish RPC channel (preferring Unix domain socket to avoid network exposure).
   - Run plugin health check before marking it ready.
4. **Execution**
   - When a lifecycle event matches an `.events` handler of type `plugin:<name>`, the trigger marshals the payload into the RPC request and awaits the response.
   - For async handlers, calls are fire-and-forget with retries handled by VFS; for sync handlers, VFS waits for the plugin response (with timeout) and translates success/veto/error.
5. **Shutdown & Reload**
   - On graceful shutdown or plugin upgrade, the host sends a termination RPC (`Shutdown`), waits for in-flight work to finish, and then stops the process.
   - Hot reload swaps binaries atomically: new process starts, health check passes, registry updates handler mapping, old process drained and terminated.

## RPC Contract
```
message HandleRequest {
  string event_type = 1;
  string handler_name = 2;
  bool synchronous = 3;
  bytes payload_json = 4;      // same structure used by built-in handlers
  map<string,string> metadata = 5; // request ID, directory ID, etc.
}

message HandleResponse {
  bool success = 1;
  bool veto = 2;
  string message = 3;
  string code = 4;           // machine-readable error code
  map<string,string> metrics = 5; // optional; folded into observability
}

service EventHandler {
  rpc Handle(HandleRequest) returns (HandleResponse);
  rpc Health(google.protobuf.Empty) returns (HealthStatus);
  rpc Shutdown(google.protobuf.Empty) returns (google.protobuf.Empty);
}
```

## Configuration Changes
- `.events` handlers gain a new type: `"type": "plugin"` plus `"plugin_name": "my_plugin"`.
- Optional plugin-specific config blob passed through unchanged to the handler.
- Global config gains `PLUGINS_DIR`, `PLUGIN_SANDBOX_MODE`, default timeouts, and resource limits.

## Security Considerations
- **Sandboxing**: Run plugins in separate OS processes with limited privileges. For untrusted plugins consider containerized/external runtimes (Firecracker, gVisor).
- **Resource quotas**: Enforce CPU/memory limits per plugin host; track metrics to detect runaway handlers.
- **Code signing**: Optional but recommended for marketplaces; verify plugin binaries before loading.
- **Audit logging**: Record plugin version, execution time, outcome, veto reasons.

## Testing Strategy
- Provide contract tests in `citest` that spin up a sample plugin (e.g., Go executable that writes to tmp) and validate sync/async behavior.
- Unit tests for registry/loader to ensure descriptors are parsed and errors surfaced clearly.
- Chaos tests that crash the plugin process mid-call to ensure VFS retries/marks it unhealthy.

## Rollout Plan
1. **Phase 1**: Implement core host, gRPC contract, Go SDK, and documentation. Support only async plugin handlers at first.
2. **Phase 2**: Add synchronous/veto support with proper timeout handling. Release Python SDK.
3. **Phase 3**: Harden security—container isolation, signature verification, dynamic reload admin API.
4. **Phase 4**: Publish curated plugin examples (auditing, analytics) and monitoring dashboards.

## Open Questions
- Should we allow in-process plugins (Go interfaces) for performance at cost of isolation?
- How to distribute plugins—private repo, OCI images, or package registry?
- How to surface plugin-specific metrics? (Proposal: structured logs + optional Prometheus exporter.)
- Governance: do we need a plugin manifest lock file to pin versions in environments?

