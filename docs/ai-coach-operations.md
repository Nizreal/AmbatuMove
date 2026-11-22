# AI exercise coach operations

The coach is an optional, server-side feature. It is disabled by default (`AI_ENABLED=false`) and only accepts user JWTs; partner API keys cannot call it. The service does not store chat transcripts. Every request that can reach a provider must explicitly set `consent_external_processing: true`.

## Configure Gemini

The current default model is `gemini-3.5-flash-lite`; the model name is configurable through `GEMINI_MODEL`. Google model availability, project quotas, and pricing can change, so verify the provider console before enabling the feature. Free-tier eligibility does not guarantee quota for a particular project or region.

Provide `GEMINI_API_KEY` through the deployment secret mechanism or a permission-restricted, untracked environment file. Do not put a key in `.env.example`, source control, Docker build args, or the image. Restrict file permissions and rotate/revoke the key in Google AI Studio if it is exposed. The HTTP client sends the key in the `x-goog-api-key` header and does not log it.

Environment settings:

```dotenv
AI_ENABLED=true
AI_PROVIDER=gemini
GEMINI_MODEL=gemini-3.5-flash-lite
GEMINI_API_KEY=<provided outside source control>
AI_TIMEOUT_SECONDS=20
AI_MAX_OUTPUT_TOKENS=512
AI_RATE_LIMIT_RPM=5
AI_MAX_CONCURRENT_REQUESTS=1
```

The server validates bounded timeout, output-token, per-user request-rate, and in-process concurrency values. General coaching requires explicit consent; otherwise the API returns a consent error. Current discomfort (caution) is sent to Gemini only when the user consents and the provider is available. The response always appends deterministic safety guidance; without consent or when the provider is unavailable, rate-limited, or times out, a local safety response is returned. Current red-flag symptoms (urgent) are handled locally regardless of consent and are never sent to Gemini. Negated, hypothetical, quoted, and clearly historical red-flag mentions are not treated as current emergencies. Provider HTTP 429 remains a rate-limit response with `Retry-After` when supplied; transient provider errors are not retried automatically.

The provider receives the user's question, explicitly supplied prior turns, selected canonical exercise details, an owned saved workout summary, and only profile or recent-history summaries when the corresponding include flags are true. User and catalog content are sent as untrusted data, not system instructions. No user identifier, email, raw workout-log notes, password, or transcript is persisted in coach storage. The rate counter stores only a user ID, minute window, and count. Use the provider's current privacy/data-use settings appropriate to your organization; external data processing requires user consent.

## Test without a real API key

Run the normal tests; they use a local mock provider and make no external requests:

```bash
cd backend
go test ./internal/coach ./internal/httpapi
```

Production API keys are not needed for CI or local mock tests. If you intentionally run a separate live smoke test, use synthetic prompts and a short-lived, restricted key supplied only through the environment. Never paste personal health information into that test. Revoke the key after use if it was created for the smoke test.

## Example

```bash
curl -fsS -X POST "$BASE/api/v1/coach/chat" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"message":"Why do I feel my biceps instead of my back?","exercise_id":"ex_lat_pulldown","consent_external_processing":true}'
```

Responses include a request ID, provider/model when external inference was used, a safety classification (`general`, `caution`, or `urgent`), suggested actions, and a short limitation statement. For example, consented wrist discomfort may receive grounded exercise-specific guidance plus a deterministic instruction to stop painful movement. Without consent, the same caution request receives local guidance only. Current urgent warning signs receive local escalation advice without calling Gemini. The coach is not a clinician, does not observe exercise form, and cannot diagnose or clear someone to continue exercising.
