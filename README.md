# GK

English · [简体中文](README.zh_CN.md)

GK is a Go + React full-stack starter for building web services with a practical foundation: PostgreSQL, type-safe SQL, OpenAPI contracts, structured logging, and OpenTelemetry. The frontend builds into the Go binary, so you can deploy the application as one executable alongside your database.

## Why GK

- **Start with a working feature.** The removable task example connects a React screen to HTTP handlers, business logic, SQL queries, and database migrations. Use it as a reference when adding your own features.
- **Keep changes close to the business.** Code lives in `internal/<domain>`, with dependencies assembled in one place. Domain models stay separate from HTTP payloads and database rows, making features easier to understand and test.
- **Keep contracts explicit.** OpenAPI generates Go HTTP bindings and TypeScript client code; the frontend uses generated domain types. sqlc generates typed Go queries from SQL you can read and tune directly.
- **Ship a single application binary.** Go embeds the built React app and Goose migrations. Production does not need a Node.js server to serve the frontend; PostgreSQL remains a separate dependency.
- **Diagnose requests across layers.** Structured logs carry request IDs and, when tracing is enabled, trace and span IDs. OpenTelemetry exports HTTP traces and metrics, Go runtime metrics, and PostgreSQL connection-pool metrics.
- **Use one development workflow.** mise pins tools and provides commands for generation, development, builds, tests, and migrations. A GitHub Actions workflow runs the verification pipeline.

GK suits internal tools, dashboards, and small-to-medium web applications that need a Go backend and a React UI. Authentication and authorization are not included; add them before exposing private data or protected operations.

## Stack

| Area | Tools |
| --- | --- |
| Backend | Go, standard-library `net/http` |
| Frontend | React, TypeScript, Vite, Tailwind CSS, TanStack Query |
| Database | PostgreSQL, pgx, sqlc, Goose |
| Contracts | OpenAPI, oapi-codegen, @hey-api/openapi-ts |
| Operations | slog, OpenTelemetry, health checks, graceful shutdown |
| Development | mise, pnpm, Go tests, Vitest, Docker Compose |

## Quick start

Requirements: mise and Docker with Compose. Start Docker before running the development environment.

```bash
git clone https://github.com/lucasdevco/gk.git
cd gk
cp .env.example .env
mise trust
mise install
mise run setup
mise run dev
```

Open <http://localhost:5173>. The development command starts PostgreSQL, the Go backend, and Vite. Vite proxies `/api` and `/health` to the backend on port 8080. Frontend edits reload through Vite; restart the backend after Go changes.

## Commands

Interactive API documentation is available at <http://localhost:8080/api/docs>, with the raw contract at <http://localhost:8080/api/openapi.yaml>. Both paths also work through the Vite development server on port 5173. Swagger UI supports trying requests against the same origin. Its pinned JavaScript and CSS assets load from unpkg and require internet access; the OpenAPI contract is embedded in the binary and served locally.

```bash
mise run generate                 # Generate SQL queries, Go bindings, and TypeScript SDK
mise run build                    # Build the UI and dist/bin/gkd
mise run test                     # Run Go and frontend unit tests
mise run lint                     # Run go vet and TypeScript checks
mise run format                   # Format Go and frontend code
mise run ci                       # Generate, lint, test, and build
mise run db:migrate:new -- add_x   # Create a SQL migration
mise run docker:down              # Stop containers; retain database volume
```

## Project layout

```text
cmd/gkd                  Process entry point
internal/app             Dependency assembly and application lifecycle
internal/platform        Configuration, logging, HTTP, database, telemetry
internal/task            Removable example business module
api                      OpenAPI contract and generated Go bindings
db/migrations            Embedded Goose migrations
db/queries               SQL source for sqlc
db/sqlc                  Generated Go query code
web                      React app and generated TypeScript client
deploy                   Docker and local observability configuration
```

Each business module owns its model, logic, and HTTP/storage adapters. A small repository interface separates business logic from persistence; tests can exercise it through an in-memory adapter. `internal/app` constructs dependencies and mounts routes.

## Configuration and observability

Text-mode startup displays a GK banner with the version, environment, listen address, and API documentation paths. Set `APP_BANNER=false` to hide it. The banner follows `LOG_COLOR` and is always hidden when `LOG_FORMAT=json`. It announces startup, not readiness; use `/health/ready` to check readiness.

Configuration comes from environment variables. mise loads `.env` for local commands; when running the binary directly, supply environment variables through your shell or deployment platform. See [.env.example](.env.example) for defaults. Database migrations run when the backend starts.

Logs go to stdout. Set `LOG_FORMAT=json` for structured output and `LOG_LEVEL` to control verbosity. Text log levels are colored by default (debug: cyan, info: green, warn: yellow, error: red); set `LOG_COLOR=false` to disable colors. JSON output ignores this setting; when enabled, ANSI colors are also written to redirected output. Avoid logging credentials or sensitive payloads.

Observability is optional: run `mise run docker:observability`, enable the commented local `OTEL_*` settings in `.env.example` in your `.env`, and restart the API. `mise run dev` starts only PostgreSQL alongside the API and Vite.

The bundled exporter uses OTLP over gRPC. View traces in Jaeger at <http://localhost:16686>, metrics in Prometheus at <http://localhost:9090>, and Grafana at <http://localhost:3000>. Local Grafana allows anonymous organization administration without login and listens only on 127.0.0.1. Grafana automatically provisions Prometheus and Jaeger data sources and the GK Observability Demo dashboard. Logs remain on stdout; they are not exported through OTLP.

In development, use [API docs](http://localhost:8080/api/docs) to run `baseline`, `slow-dependency`, or `retry` with `{}`. View metrics in the [Grafana dashboard](http://localhost:3000/d/gk-observability) and find the response's `traceId` in Jaeger. Allow 20–30 seconds for metrics to appear.

Responses include `Server-Timing: app;dur=<milliseconds>`, measuring backend time until final response headers are sent, excluding body transmission. Existing timing metrics are preserved. The configured CORS origin can read the header and access browser performance timings.

- `GET /health/live` reports process liveness.
- `GET /health/ready` checks database connectivity.

## Error handling

Define domain errors in `internal/<domain>/errors.go` using `errors.New`. Preserve causes with `%w`; map expected errors with `errors.Is/As` in the HTTP adapter. Business logic does not depend on HTTP status codes.

The shared HTTP helpers return `{ "error": { "code": "task_not_found", "message": "task not found" } }`, with optional `requestId` and safe `details`. HTTP status codes describe transport-level outcomes; `code` is a stable application error identifier in snake_case that clients can branch on. Multiple application codes can share an HTTP status. Unknown errors are logged with their cause and returned as HTTP 500 with `code: "internal_error"`. Never expose raw internal errors to clients.

## Add a business module

1. Define the contract in `api/openapi.yaml`.
2. Add a migration and SQL queries under `db/`.
3. Run `mise run generate`.
4. Implement the model, business logic, and HTTP/storage adapters in `internal/<domain>`.
5. Assemble dependencies and register routes in `internal/app/app.go`.
6. Add the frontend feature and tests for its behavior.

Keep sqlc row types inside the storage adapter. Return domain types to business callers and map them to the generated contract in the HTTP adapter.

The [GK Service Overview](http://localhost:3000/d/gk-service) monitors HTTP throughput, 4xx/5xx ratios, P95/P99, Go memory, and database pools, filtered by service, environment, and instance. HTTP panels exclude health probes and demo traffic; rate windows are at least four minutes to accommodate default 60-second exports. Each process generates `service.instance.id`, overridable through `OTEL_RESOURCE_ATTRIBUTES`. When using your own Collector, retain the service/environment label mapping in `deploy/otel-collector.yaml` and enable Prometheus `honor_labels`. The dashboard is reusable for deployed services; the anonymous local Grafana Compose configuration is not a production deployment.

Generate traffic after starting the API and enabling telemetry: `python3 scripts/traffic.py --duration 180 --rate 10`. The script sends real requests through normal → 4xx errors → recovery phases without creating or modifying task data. Use `--url` for another origin, `--concurrency` to bound in-flight requests, or `--demo` to also exercise development scenarios. With default 60-second exports, use `--duration 600` so each phase spans multiple exports. It does not fabricate 5xx; real server failures such as database outages raise that ratio.
