## CITest Architecture

### Overview
The `citest` package hosts the end-to-end suite that boots real service binaries against ephemeral infrastructure to validate policy enforcement and content storage flows. Tests are written with Ginkgo/Gomega and exercise the HTTP APIs via `httpexpect`.

### Infrastructure Lifecycle
- **Dockerized dependencies**: MySQL and LocalStack (S3) are provisioned with `testcontainers-go`, ensuring isolated, reproducible backing services per run.
- **Configuration**: Each run generates a temporary YAML config that points services at the freshly created containers and S3 bucket.
- **Binary execution**: `gexec.Build` compiles the metadata and content services, and the suite runs the resulting binaries directly so signals (SIGTERM/SIGKILL) land on the actual servers, enabling fast teardown.

### Test Flow
1. Start containers, wait for readiness, and create the S3 bucket used during tests.
2. Launch metadata and content binaries with the generated configuration and poll their `/ping` endpoints until healthy.
3. Execute specs (e.g., policy enforcement) by exercising the services through `httpexpect`.
4. On teardown, terminate the services, escalate to SIGKILL only if needed, and remove temporary resources.

### Extending the Suite
- Add scenario-focused specs under `citest` and keep them self-contained.
- Reuse helper functions in `setup_test.go` for HTTP interactions and session management.
- Run `go test -count=1 ./citest -run TestCITest` locally to verify new specs against fresh infrastructure each time.
