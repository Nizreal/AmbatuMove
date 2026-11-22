# Backend

[Bahasa Indonesia](README.md) | [Project README](../README.en.md)

## Local development

Requires Go 1.27.x. Run the commands from `backend/`. Copy `.env.example` to `.env`, set `DATABASE_PATH` and export the environment variables in your shell. The binary does not read dotenv files. For production use a persistent base64 JWT signing secret of at least 32 decoded bytes.

```bash
go run ./cmd/server serve
```

Default service port: 8080. Business routes start with `/api/v1`; `/health`, `/ready`, `/openapi.json` and `/docs` are root routes. Startup applies migrations and inserts missing reference rows. Existing administrator edits survive restarts. Synthetic gyms require `APP_ENV=development` or `test` and `SEED_SYNTHETIC_GYMS=true`; production rejects this setting and deactivates old synthetic records without deleting their references.

## Reference data

Back up the database, validate the import and apply it:

```bash
go run ./cmd/server backup --output data/pre-import.sqlite
go run ./cmd/server verify-backup --path data/pre-import.sqlite
go run ./cmd/server admin gym import --file data/gyms.json --dry-run
go run ./cmd/server admin gym import --file data/gyms.json
```

`admin gym|exercise|equipment|facility|met` supports `import --file PATH [--dry-run]` and `export --file PATH`. Imports validate the full JSON array, identifiers, enum values and related records before writing a transaction. Omitted record IDs remain intact. Exports can be edited and reimported. SQLite cache revisions allow external CLI imports to become visible to the server without a restart.

The curated dataset is `data/gyms.json`. The survey queue is documentation only. Unknown equipment quantities remain unknown. Use `REFERENCE_CACHE_ENABLED=false` or `GYM_CACHE_ENABLED=false` for cache-bypassed comparisons.

## Access management

```bash
go run ./cmd/server admin partner-key create \
  --group TEAM --scopes catalog:read,gyms:read
go run ./cmd/server admin partner-key list
```

The secret is printed once. Use `admin partner-key rotate --id ID` or `admin partner-key revoke --id ID` to manage existing keys. To create an administrator, securely pipe a password to `admin user create-admin --email EMAIL --password-stdin`. No admin or partner key is provided by default.

## Checks and operations

```bash
go vet ./...
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -o /tmp/ambatumove-server ./cmd/server
```

The CLI also supports `migrate`, `seed`, `healthcheck` and `schema-version`. Use the schema version when assessing binary rollback. A backup is never automatically restored by the release deployment script. `GOMEMLIMIT=180MiB` is a soft runtime target; Compose enforces the hard container limit. See the [deployment runbook](../docs/performance/AmbatuMove_STB_Deployment_and_Operations_Runbook_v1.1.md) for release and recovery procedures. On the deployed STB, use port 18081.

For the calorie recommendation query parameters and all other API schemas, see [OpenAPI](../docs/openapi.yaml) or the [API reference](../docs/api-reference.en.pdf).

### P2 and AI coach

Migration 0005 adds equipment presets, manual logs/corrections, audit history, workout revisions, gym reviews/moderation, favorites, and coach rate counters. Create and verify a database backup before upgrading the binary. Rolling back to an older binary requires checking schema and migration compatibility; migrations are not automatically reversed.

Gemini coaching is enabled only when `AI_ENABLED=true` and `GEMINI_API_KEY` is supplied through a deployment secret or untracked environment file. The examples stay disabled with a blank key. Each request must include explicit external-processing consent; chat history is not persisted. See the [AI coach operations guide](../docs/ai-coach-operations.md). Password recovery remains disabled until a safe mail provider is configured.
