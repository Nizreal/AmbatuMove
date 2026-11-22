# AmbatuMove

[Bahasa Indonesia](README.md) | [PDF](README.en.pdf) | [API reference (PDF)](docs/api-reference.en.pdf) | [HTML](docs/api-reference.en.html)

- API: https://nizdri.stei.fun/api/v1
- Swagger UI: https://nizdri.stei.fun/docs
- OpenAPI: [YAML](docs/openapi.yaml) / [JSON](docs/openapi.json) / [live](https://nizdri.stei.fun/openapi.json)
- Release: [v0.1.8](https://github.com/Nizreal/AmbatuMove/releases/tag/v0.1.8)

AmbatuMove is the Health & Wellbeing microservice for the TST and PWAM course project. It generates workouts using available time, experience, equipment, location and fitness goals. Users can save plans, log activities and view their training history. Partner groups can access the exercise catalog, gym directory, calorie estimates and workout previews through the API.

## Architecture and deployment

```text
Mobile app / partner service
    | HTTPS
Cloudflare Tunnel (host)
    | localhost:18081
Nginx gateway (8080)
    | core-service:8080
Go core -> SQLite (/data/ambatumove.sqlite)
```

The backend uses Go and SQLite. Nginx and the backend run in two Docker containers on an ARM64 STB. Cloudflare Tunnel runs on the host. The mobile application and cross-group integrations run outside the STB.

| Container | CPU limit | RAM limit | Idle RAM |
|---|---:|---:|---:|
| Backend | 0.50 | 256 MiB | 8.621 MiB |
| Nginx | 0.25 | 64 MiB | 7.984 MiB |

The measurements were taken on the physical STB after deployment on 22 November 2026. The production port is **18081**, matching the existing tunnel configuration. Data resides in `/mnt/storage/ambatumove`; release artifacts and deployment configuration reside in `/mnt/storage/ambatumove-deployment`.

## API access

Public endpoints require no account. Partner groups can request an individual API key privately from the AmbatuMove team and send it using `X-API-Key`. Users register and log in with email and password; personal data requires `Authorization: Bearer <token>`. Moderation requires an admin JWT.

| Scope | API |
|---|---|
| `catalog:read` | Exercises, equipment, muscle groups, categories |
| `gyms:read` | Gyms, equipment, facilities |
| `calories:estimate` | Calorie estimates and recommendations |
| `planner:preview` | Workout/program previews, alternatives |

Default limits: 60 requests/min per client or user per route group, 120 for partner keys, and 5 for authentication routes. Use timeouts, cache reference data and back off on HTTP 429. Keep partner API keys on the partner server and out of APKs and public repositories. Use synthetic test data.

## Examples

```bash
BASE=https://nizdri.stei.fun
curl -fsS "$BASE/health"
curl -fsS "$BASE/api/v1/gyms"
curl -fsS "$BASE/api/v1/calories/estimate?activity=walking&intensity=moderate&duration_minutes=15&weight_kg=70"
curl -fsS "$BASE/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&max_duration_minutes=45"
```

```bash
curl -fsS -X POST "$BASE/api/v1/workouts/generate" \
  -H 'Content-Type: application/json' \
  -d '{"goal":"endurance","workout_type":"cardio",
       "duration_minutes":30,"experience_level":"beginner",
       "environment":"outdoor","space":"unrestricted",
       "available_equipment":["bodyweight"],
       "preferred_activity_ids":["act_walk_outdoor"],"save":false,"seed":42}'
```


Partner access:

```bash
curl -fsS "$BASE/api/v1/gyms" -H "X-API-Key: $PARTNER_KEY"
```

Workout previews are unsaved. To persist a plan, supply a JWT and `save: true`. Business responses generally contain `data` and `meta.request_id`; list responses may include `meta.pagination`. See OpenAPI for complete parameters and schemas.

## Initial data and verification

At verification time, the service contained 53 exercises, 16 equipment types, 12 muscle groups and 9 Bandung gyms. Gym records have source information and remain `unverified`. Unknown inventory is left unspecified. MET calorie estimates may differ from actual expenditure.

Health, readiness, catalogs, gym directory, calorie estimation, recommendations and workout preview were tested through the public domain. User sessions, API key permissions, saved workouts, programs, tracking, load testing and restart persistence still require live verification. See [results](docs/production-verification.en.md) and [PDF](docs/production-verification.en.pdf).

## Development and maintenance

- [Backend guide](backend/README.en.md) ([Bahasa Indonesia](backend/README.md)) for local configuration and administration.
- [STB runbook](docs/performance/AmbatuMove_STB_Deployment_and_Operations_Runbook_v1.1.md) for deployment and recovery. Retain production port 18081 where examples use 18080.
- `scripts/release.ps1` or `scripts/release.sh` build ARM64 binaries on the laptop. Publish a new tag and release for code changes, then run `bash deploy/deploy-release.sh deploy VERSION` from a matching checkout on the STB.
- Gym, exercise, equipment, facility and MET data can be exported or imported through the CLI. Make a verified backup and dry-run imports. SQLite revision tracking refreshes caches.

## API checks

```bash
python scripts/smoke-test.py --base https://nizdri.stei.fun
```

For local measurements on the STB:

```bash
python scripts/smoke-test.py --base http://127.0.0.1:18081 --benchmark
```

This script has yet to be run against production. `--partner` tests a full-scope partner key; `--demo-email` selects a synthetic account. `--register-demo` and `--write-demo-workout` create test data. See `--help` for usage.
