# Production verification: v0.1.8

Observed 22 November 2026, on the physical ARM64 STB and public HTTPS endpoint.

| Check | Observed result |
|---|---|
| ARM64 release checksum | OK |
| Runtime Docker image build | 23.5 seconds |
| Core and gateway health | Both healthy |
| Public `/health`, `/ready` | HTTP 200 |
| `/api/v1/exercises` | 53 catalog entries, pagination returned 20 |
| `/api/v1/gyms` | 9 curated gyms, no synthetic records in returned list |
| `/api/v1/equipment` | 16 equipment types |
| `/api/v1/muscle-groups` | 12 muscle groups |
| Forward calorie estimate | Moderate walking, 70 kg, 15 minutes, MET 3.8: 69.8 kcal |
| Reverse calorie recommendation | 200 kcal, 70 kg, 45-minute cap: four target-reaching options plus one partial option |
| Workout preview | Outdoor walking, 30 minutes; 3 min warm-up, 24 min main, 3 min cooldown; unique item IDs, no warnings, saved=false |
| Idle container memory | Core 8.621 MiB; gateway 7.984 MiB |
| System available memory | 1.3 GiB at idle snapshot |

The production gateway is `127.0.0.1:18081`, matching the existing remotely managed tunnel. The public base URL is `https://nizdri.stei.fun/api/v1`.

Pending live verification: partner-key validity and scope restrictions, authenticated user lifecycle, gym-constrained planning and adaptation, program generation, activity tracking, complete API documentation smoke test, load testing, and restart persistence. This record does not claim those tests passed. No credentials or personal data are included.
