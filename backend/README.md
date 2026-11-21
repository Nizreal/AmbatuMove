# AmbatuMove Core

Single Go HTTP service with SQLite persistence, embedded forward migrations, seeded exercise/cardio and MET reference data, and clearly synthetic gym examples.

## Local development

Requires Go 1.27.x. Copy `.env.example` to `.env` and set `DATABASE_PATH`; export its values in your shell (the binary does not load dotenv files). In production, set `JWT_SIGNING_KEY` to a base64-encoded random value with at least 32 decoded bytes. Use a persistent secret across restarts. Start with `go run ./cmd/server serve`; routes are under `/api/v1`, and `/health`, `/ready`, `/openapi.json`, and `/docs` are at the root.

The service migrates and idempotently seeds on startup. `go run ./cmd/server migrate` applies migrations, `seed` refreshes synthetic reference rows, and `healthcheck` probes local liveness. The local database directory is created automatically.

The bounded reference and gym caches are enabled by default. Set `REFERENCE_CACHE_ENABLED=false` or `GYM_CACHE_ENABLED=false` to run cache-bypassed comparisons; the SQL-backed results are unchanged.

`GET /api/v1/calories/recommendations?target_kcal=250&weight_kg=70&max_duration_minutes=45&category=cardio` returns deterministic, constraint-aware activity options from the seeded MET catalog. Repeat `available_equipment`, `excluded_equipment`, and `excluded_exercise_ids` for multiple IDs; pass `gym_id` to use that gym's verified inventory. Without a gym or explicit equipment IDs, only bodyweight/no-required-equipment activities are eligible. Defaults are 180 minutes, beginner experience, moderate maximum intensity, gross MET estimation, and up to eight results. The endpoint is stateless and public; consumers may also use a partner key scoped for `calories:estimate`. When a target cannot be reached within the time cap, the result includes partial alternatives or an explanation with an empty list. See [`../docs/openapi.yaml`](../docs/openapi.yaml) for complete query and response schemas.

Create partner keys with `go run ./cmd/server admin partner-key create --group <name> --scopes catalog:read,gyms:read,calories:estimate,planner:preview`. The generated secret is printed once. List metadata with `admin partner-key list`, rotate with `admin partner-key rotate --id <id>`, and revoke with `admin partner-key revoke --id <id>`. Bootstrap an administrator by securely piping a password to `go run ./cmd/server admin user create-admin --email <email> --password-stdin`. No admin or partner key is provisioned by default.

Create an online backup with `go run ./cmd/server backup --output <new-file>` and verify it with `go run ./cmd/server verify-backup --path <file>`. Protect backup files as user data. Set `GOMEMLIMIT=180MiB` for the constrained deployment profile; this is a runtime target, not a hard process memory cap.

The `go run ./cmd/server schema-version` operational command prints the current database migration version without applying migrations. The STB deployment script uses it to decide whether it can safely roll the application image back after a failed activation; it never automatically restores database contents.

## Checks

```sh
gofmt -w ./cmd ./internal ./migrations
go vet ./...
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o /tmp/ambatumove-server ./cmd/server
```

Build the ARM64-compatible container with `docker buildx build --platform linux/arm64 -t ambatumove-core:local .` from this directory. Docker must be installed separately.
