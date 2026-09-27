# Wishbone HTTP / WebSocket API (нормативно)

Базовый адрес dev — `http://localhost:8080` (эмулятор Android — `http://10.0.2.2:8080`). Все тела — JSON UTF-8. Ошибка любого эндпоинта — HTTP-статус из `errors.json` и тело `{"error": {"code": "…", "message": "…", "retryable": bool}}`.

## Публичный API (SDK)

Заголовки каждого запроса SDK: `X-Wishbone-Runtime: 1.0.0`, `X-Wishbone-Platform: ios|android`, `X-Wishbone-Brand: korpu`, `Accept-Language: ru`, плюс заголовки хоста (`Authorization` и т.п.).

### `GET /v1/screens/{id}?params=<urlencoded JSON object>`
`If-None-Match: <etag>` — опционально.
- 200 `{"etag": "…", "revision": 4, "document": {…} | null, "data": {"<name>": {"ok": true, "value": …} | {"ok": false, "error": {code, message, retryable}}}}` — `document: null`, когда `If-None-Match` совпал (данные всё равно резолвятся и отдаются). `etag` = `"<rev>-<sha256(document)[:16]>"`.
- 404 `SCREEN_NOT_FOUND`; 409 `UNSUPPORTED_RUNTIME` (runtimeMin ревизии > X-Wishbone-Runtime); 503 `DATA_REQUIRED_FAILED`.

### `POST /v1/data`
`{"screen": "id", "revision": 4, "params": {…}, "sources": ["me", "offers"]}` → 200 `{"<name>": {ok, value?|error?}}`. Аргументы источников сервер берёт из ревизии (с подстановкой `params`). 409 `REVISION_NOT_FOUND`.

### `POST /v1/call`
`{"screen": "id", "revision": 4, "op": "demo.subscribe", "args": {…}, "idempotencyKey": "uuid"}` → 200 `{"ok": true, "value": …}` | `{"ok": false, "error": {code, message, retryable}, "fieldErrors": {"email": "…"}?}`. Ошибки протокола (HTTP ≠ 200): 400 `OPERATION_UNKNOWN`, 403 `OPERATION_NOT_BOUND` (op не используется ревизией), 409 `REVISION_NOT_FOUND`, 400 `IDEMPOTENCY_REQUIRED` (write без ключа), 409 `IDEMPOTENCY_MISMATCH` (тот же ключ, другие args), 409 `CALL_IN_FLIGHT`. Повтор write с тем же ключом и args отдаёт сохранённый ответ, handler не вызывается. Доменная ошибка handler'а (`ok: false`) — это 200.

### `GET /v1/theme?brand=korpu`
`If-None-Match` поддерживается → 304. 200 — содержимое `contract/tokens/<brand>.json` + `"version": "<sha256[:16]>"`; неизвестный бренд → тема `korpu`.

### `GET /assets/<path>`
Статика из `contract/assets/`.

### Превью (устройства)
- `GET /v1/preview/sessions` → `[{"code": "K7Q2", "screen": "showcase.promo", "title": "Промо", "devices": 1, "updatedAt": "RFC3339"}]` (только живые сессии: последняя активность < 2 ч).
- `GET /v1/preview/ws?session=K7Q2` — WebSocket. Устройство первым шлёт `{"type": "hello", "platform": "ios", "device": "iPhone 17 Pro", "runtime": "1.0.0"}`; сервер сразу отвечает последним документом сессии, если он есть. Сервер → устройство: `{"type": "document", "screen": "id", "document": {…}, "data": {…}}` (формы как в `/v1/screens`), `{"type": "play", "timeline": "intro"}`. Устройство → сервер: `{"type": "log", "level": "info"|"warn"|"error", "message": "…"}`.

## Admin API

Все запросы — `Authorization: Bearer <WISHBONE_ADMIN_PASSWORD>` (WebSocket — `?token=`). 401 `UNAUTHENTICATED` без него.

| Метод и путь | Тело → ответ |
|---|---|
| `GET /admin/api/screens` | → `[{"id", "title", "updatedAt", "publishedRev": 4 \| null}]` |
| `POST /admin/api/screens` | `{"id", "title"}` → 201 `{"id", "title", "draft": <исходник>}`; черновик по умолчанию — корень `stack` с одним `text`; 409 если id занят |
| `GET /admin/api/screens/{id}` | → `{"id", "title", "draft": <исходник>, "publishedRev": n \| null, "updatedAt"}` |
| `PUT /admin/api/screens/{id}` | `{"title"?, "draft"}` → `{"updatedAt"}` (черновик сохраняется даже с ошибками компиляции) |
| `POST /admin/api/screens/{id}/publish` | → 200 `{"revision", "etag"}` \| 422 `{"diags": [Diag]}` |
| `GET /admin/api/screens/{id}/revisions` | → `[{"revision", "etag", "publishedAt", "runtimeMin"}]` (новые сверху) |
| `POST /admin/api/screens/{id}/rollback/{rev}` | → `{"revision", "etag"}` — исходник ревизии `rev` публикуется новой ревизией и становится черновиком |
| `POST /admin/api/compile` | `{"source": <исходник>}` → `{"ok": bool, "diags": [Diag], "document": {…} \| null}` |
| `GET /admin/api/catalog` | → `{"primitives": <primitives.json>, "stdlib": <stdlib.json>, "templates": [{"name", "params": [{"name", "type", "default"?}], "events": ["tap"], "slot": bool}], "tokens": {"color": [..], "type": [..], "space": [..], "radius": [..], "motion": [..]}, "icons": [..], "ops": [{"id", "kind", "args", "result"}], "brands": ["korpu", "kz"]}` |
| `GET /admin/api/templates/{name}` | → исходник шаблона (только чтение) |
| `POST /admin/api/preview` | `{"screen": "id"?}` → `{"code": "K7Q2"}` |
| `POST /admin/api/preview/{code}/document` | `{"source": <исходник>}` → `{"ok", "diags": [Diag]}`; при `ok` документ (с данными) рассылается устройствам сессии |
| `POST /admin/api/preview/{code}/play` | `{"timeline": "intro"}` → 204 |
| `GET /admin/api/preview/{code}/ws?token=` | WebSocket, сервер → админка: `{"type": "devices", "list": [{"id", "platform", "device"}]}` при каждом изменении, `{"type": "log", "device", "level", "message"}` |

`Diag` = `{"path": "root/children/2/props/text" | "script" | "timelines/0/tracks/1", "line": 1, "col": 5, "message": "…"}` — `line`/`col` относительно DSL-фрагмента по `path` (1-based), `0` если к фрагменту не относится.

Статика админки — `GET /admin/*` (SPA, `index.html` для неизвестных путей).
