# Task 8 — controller decisions (binding, supplement the brief)

Normative: contract/api.md (every endpoint, JSON shape, status code, header), spec §5 (docs/superpowers/specs/2026-09-27-wishbone-v1-design.md), contract/errors.json, contract/operations.json. The compiler API you use is in task-3-report.md / task-2-report.md (same dir): `compile.LoadContract`, `compile.Screen`, `Meta`, `Diag`.

## Process
- `backend/cmd/server/main.go`; flags/env: `-addr` (default `:` + `$PORT` or `:8080`), `-contract` (default `../contract`), `-admin` (default `../admin/dist`), `DATABASE_URL` (default `postgres:///wishbone?sslmode=disable`; the dev DB is local Postgres 17 — create it if missing by connecting to `postgres` db and `CREATE DATABASE`), `WISHBONE_ADMIN_PASSWORD` (default `wishbone`, log a warning when defaulted).
- Dependencies: `github.com/jackc/pgx/v5` (pgxpool), `github.com/coder/websocket`. Nothing else. `log/slog` JSON logs; never log call args/values, draft sources, Authorization.
- Startup: LoadContract (fail fast), migrations (embedded `internal/store/migrations/*.sql`, applied in order, tracked in `schema_migrations`), ops registry must cover every op in operations.json (fail fast otherwise), seed: every `contract/screens/*.json` whose id is not in the DB is created and published (none exist yet — handle an empty/missing dir).

## Store (`internal/store`)
Tables: `screens(id text pk, title text, draft jsonb, created_at, updated_at)`, `revisions(screen_id, rev int, source jsonb, document jsonb, etag text, runtime_min text, meta jsonb /* ops, data decls */, published_at, pk(screen_id, rev))`, `idempotency(key text, op text, request_hash bytea, status text /* in_flight|done */, response jsonb, created_at, pk(key, op))`, `audit(id bigserial, actor text, action text, entity text, at timestamptz, details jsonb)`. Live revision = max(rev). Publish = compile draft → insert rev = max+1 in one transaction (+ audit). Rollback = publish the source of rev N as a new revision and set it as the draft. Screen id must match `^[a-z0-9][a-z0-9._-]{0,63}$` (400 ARGS_INVALID otherwise).

## Public API (`internal/api`, exactly contract/api.md)
- `GET /v1/screens/{id}`: runtime check (semver of `X-Wishbone-Runtime` ≥ runtimeMin, missing header → treat as "1.0.0"), params from `?params=` merged over document param defaults, data resolved (below), ETag `"<rev>-<sha256(document)[:16]>"`, If-None-Match → document null (data still resolved).
- Data resolution: sources in parallel, per-source timeout 500 ms, whole budget 1200 ms; args JSON with `{"$param": name}` substituted from params; a failed `required` source → 503 DATA_REQUIRED_FAILED; non-required → `{ok: false, error}`.
- `POST /v1/data`, `POST /v1/call` per api.md: op must be in the revision's meta ops (call) — else 403 OPERATION_NOT_BOUND; unknown op 400; write ops require idempotencyKey (400), same key + different args hash → 409 IDEMPOTENCY_MISMATCH, in_flight → 409 CALL_IN_FLIGHT, done → stored response replayed without calling the handler; handler failure deletes the reservation. **Validate args against the op's DSL args type at this trust boundary** (reuse dsl.ParseType + a small JSON-vs-type checker) → 400 ARGS_INVALID.
- `GET /v1/theme?brand=` with version = sha256(file)[:16] and If-None-Match → 304; unknown brand → korpu.
- `GET /assets/*` from `contract/assets/`.
- Handlers resolve absolute asset URLs from the request (`scheme://host` honoring `X-Forwarded-Proto`/`Host`), so the Android emulator (10.0.2.2) and the iOS simulator (localhost) both get reachable URLs.

## Demo ops (`internal/ops`) — exactly operations.json types
In-memory, deterministic, per-user state keyed by `Authorization` header value (or "anon"):
- `demo.profile` → `{name: "Ivan", tickets: <counter, starts 12>, avatar: <abs url /assets/avatar.png>, level: 7}`
- `demo.offers` → 6 offers, ids `o1…o6`, Russian titles/subtitles (e.g. "Двойные билеты", "Кешбэк 10%"…), prices, `image`: abs url `/assets/offers/o<N>.jpg`, `until`: now + N hours (ms epoch).
- `demo.plans` → 3 plans `basic` (199), `pro` (499), `max` (999) with 2–4 Russian perks each.
- `demo.checkPromo` → `WB10` → {valid: true, discount: 10, message: "Скидка 10% применена"}; `WB25` → 25; anything else → {valid: false, discount: 0, message: "Промокод не найден"}; simulate 300 ms latency.
- `demo.subscribe` (write) → invalid email → ok:false ARGS_INVALID + fieldErrors.email "Введите корректный email"; `taken@example.com` → fieldErrors.email "Этот email уже занят"; empty name → fieldErrors.name "Как к вам обращаться?"; else ok {id: "sub_<n>", total: plan price × (1 − discount/100) using WB10/WB25}; simulate 600 ms latency.
- `demo.claimOffer` (write) → tickets += 3 → {tickets}.

## Preview (`internal/preview`, exactly api.md + spec §5.4)
Sessions in memory: code = 4 chars from `ABCDEFGHJKMNPQRSTUVWXYZ23456789`, fields screen/title/source/document/data/devices/admins/updatedAt; expire after 2 h without activity (sweeper). `POST /admin/api/preview/{code}/document` compiles the posted source (not stored), resolves its data sources (params = defaults), stores it on the session and broadcasts `{"type":"document",…}`; diags returned either way. New device → immediate last document. Device `log` messages → admins as `{"type":"log","device",…}`; device list changes → admins `{"type":"devices",…}`. Unknown code → 404 (the admin recreates the session on 404).

## Admin API
Bearer password (constant-time compare); WS via `?token=`. Catalog per api.md: templates `[{name, params: [{name, type, default?}], events, slot}]` — `default` = the DSL source text of the default when present (read from the template source file), `tokens.<group>` / `icons` = string arrays, `ops` from operations.json, `brands` from tokens/*.json. `GET /admin/api/templates/{name}` → template source JSON. Static SPA at `/admin/*` from the `-admin` dir with index.html fallback (if the dir is missing, 404 with a hint to run `npm run build` in admin/).

## Tests
httptest against a real Postgres test database (`wishbone_test`, created/dropped in TestMain) with a tiny inline test contract dir or the real `../contract`: screen fetch + ETag/304 + params, UNSUPPORTED_RUNTIME, required data failure 503, call bound/unbound/unknown, idempotency replay/mismatch/in-flight, args validation, publish/rollback/revisions, auth 401, preview: device WS receives the document after POST …/document and gets the last document on join, admin WS gets devices + logs, session 404.
`cd backend && go test ./... && go vet ./...` green; `go run ./cmd/server` starts and `curl localhost:8080/v1/theme?brand=korpu` works.
