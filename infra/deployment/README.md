# Development image builds and deployment boundary

Build separate API and engine images from the monorepo root:

```bash
docker build -f infra/docker/api.Dockerfile -t ledgermeadow-api .
docker build -f infra/docker/engine.Dockerfile -t ledgermeadow-engine .
```

Both runtime images use non-root distroless bases. Build contexts exclude environment files, encryption keys, local secret directories, datasets, logs, and test fixtures. The API runtime contains the API and migration binaries plus schema migrations. The engine runtime contains only its binary and runtime libraries.

The current gRPC implementation is loopback-only and assumes a trusted shared host/network namespace. Separate containers cannot communicate over their individual loopback interfaces. Supply a deliberately shared trusted network namespace for local evaluation, or implement authenticated encrypted transport before distributed deployment. Do not change the bind check or expose the engine port as a shortcut.

Only the web application and authorized API routes may eventually be public. The database and engine remain private. Runtime secrets must be injected privately and excluded from image layers, build arguments, build contexts, source, and logs.

The migration runner uses `DATABASE_URL`, so supply a direct database connection for migrations. The API's listener uses `DATABASE_DIRECT_URL`. Local scripts target private development configuration; this repository contains no hosted project identifiers, deployment URLs, managed database credentials, or automatic cloud deployment workflow.

Read [security status](../../docs/SECURITY_STATUS.md) for the work required before production. CI builds and tests native applications; container definitions also require validation with a Docker daemon in your environment.
