# Wishbone HTTP / WebSocket API (нормативно)

Базовый адрес dev — `http://localhost:8080` (эмулятор Android — `http://10.0.2.2:8080`). Все тела — JSON UTF-8. Ошибка любого эндпоинта — HTTP-статус из `errors.json` и тело `{"error": {"code": "…", "message": "…", "retryable": bool}}`.

## Публичный API (SDK)

Заголовки каждого запроса SDK: `X-Wishbone-Runtime: 1.1.0`, `X-Wishbone-Platform: ios|android`, `X-Wishbone-Brand: korpu`, `Accept-Language: ru`, плюс заголовки хоста (`Authorization` и т.п.).

### `GET /v1/screens/{id}?params=<urlencoded JSON object>`
`If-None-Match: <etag>` — опционально.
- 200 `{"etag": "…", "revision": 4, "document": {…} | null, "data": {"<name>": {"ok": true, "value": …} | {"ok": false, "error": {code, message, retryable}}}}` — `document: null`, когда `If-None-Match` совпал (данные всё равно резолвятся и отдаются). `etag` = `"<rev>-<sha256(document)[:16]>"`.
- 404 `SCREEN_NOT_FOUND`; 409 `UNSUPPORTED_RUNTIME` (runtimeMin ревизии > X-Wishbone-Runtime); 503 `DATA_REQUIRED_FAILED`.

`operations.json` → `timeoutMs` (необязательно) — таймаут источника данных этой операции; по умолчанию 500 мс; бюджет экрана — max(1200 мс, самый долгий + 200 мс).

### `POST /v1/data`
`{"screen": "id", "revision": 4, "params": {…}, "sources": ["me", "offers"]}` → 200 `{"<name>": {ok, value?|error?}}`. Аргументы источников сервер берёт из ревизии (с подстановкой `params`). 409 `REVISION_NOT_FOUND`.

### `POST /v1/call`
`{"screen": "id", "revision": 4, "op": "demo.subscribe", "args": {…}, "idempotencyKey": "uuid"}` → 200 `{"ok": true, "value": …}` | `{"ok": false, "error": {code, message, retryable}, "fieldErrors": {"email": "…"}?}`. Ошибки протокола (HTTP ≠ 200): 400 `OPERATION_UNKNOWN`, 403 `OPERATION_NOT_BOUND` (op не используется ревизией), 409 `REVISION_NOT_FOUND`, 400 `IDEMPOTENCY_REQUIRED` (write без ключа), 409 `IDEMPOTENCY_MISMATCH` (тот же ключ, другие args), 409 `CALL_IN_FLIGHT`. Повтор write с тем же ключом и args отдаёт сохранённый ответ, handler не вызывается. Доменная ошибка handler'а (`ok: false`) — это 200.

### `GET /v1/theme?brand=korpu`
`If-None-Match` поддерживается → 304. 200 — содержимое `contract/tokens/<brand>.json` + `"version": "<sha256[:16]>"`; неизвестный бренд → тема `korpu`.

### `GET /`
302 на `/admin/`: голый адрес сервера открывает админку.

### `GET /assets/<path>`
Статика из `contract/assets/`. Шрифты (`contract/fonts/`) вне этого дерева и не раздаются — 404 `NOT_FOUND`.

### Превью (устройства)

Документы, пришедшие по превью, несут `revision: 0`: `/v1/call` и `/v1/data` с `revision: 0` адресуются к последнему документу живой превью-сессии этого экрана (операции и источники берутся из него).

- `GET /v1/preview/sessions` → `[{"code": "K7Q2", "screen": "showcase.promo", "title": "Промо", "devices": 1, "updatedAt": "RFC3339"}]` (только живые сессии: последняя активность < 2 ч).
- `GET /v1/preview/ws?session=K7Q2` — WebSocket. Устройство первым шлёт `{"type": "hello", "platform": "ios", "device": "iPhone 17 Pro", "runtime": "1.1.0", "render": true}`; сервер сразу отвечает последним документом сессии, если он есть. Сервер → устройство: `{"type": "document", "screen": "id", "document": {…}, "data": {…}}` (формы как в `/v1/screens`), `{"type": "play", "timeline": "intro"}`, `{"type": "render", "job": 17, "screen": "raffle.tickets", "document": {…}, "data": {…}}`. Устройство → сервер: `{"type": "log", "level": "info"|"warn"|"error", "message": "…"}`, `{"type": "shot", "job": 17, "image": "<base64 JPEG>", "width": 402, "height": 874, "settled": true}` или `{"type": "shot", "job": 17, "error": "app is in the background"}`.

#### Снимки для админки

Устройство с `render: true` снимает экраны доски админки (спека превью §1, §2): на задание `render` отвечает ровно одним `shot`. Плейграунд без `render: true` получает живое превью, но не задания.

- Задания раздаются, только пока сессию смотрит хотя бы одна админка. У устройства одно задание в работе; следующее сервер шлёт после ответа. Порядок — экраны доски (фокус первым), у которых нет снимка этого устройства для текущего содержимого (`sha256(document, data)`); неизменившийся экран повторно не снимается.
- `document` и `data` в `render` — в тех же формах, что в `document`; параметры экрана — дефолты документа. Снимок не ходит в сеть: `call`/`refresh` не уходят, навигация и прочие эффекты не выполняются.
- `shot`: `image` — base64 JPEG (начинается с `FF D8 FF`) не больше 1 МиБ после декодирования; `width`/`height` — размер в pt/dp, `1…2000` (дробные админка получает округлёнными); `settled: false` — снято, не дождавшись покоя. `error` обрезается до 200 символов. Иначе снимок считается проваленным с `error: "invalid shot"`.
- Нет ответа 15 с — снимок проваливается с `error: "timeout"`. Проваленный снимок считается сделанным до `retry` или нового содержимого экрана.
- `shot` не на задание в работе (по `job`) отбрасывается.
- Лимит сообщения устройства — 4 МиБ; больше — сервер закрывает сокет.
- Уход устройства удаляет его снимки.

## Admin API

Все запросы — `Authorization: Bearer <WISHBONE_ADMIN_PASSWORD>` (WebSocket — `?token=`). 401 `UNAUTHENTICATED` без него.

| Метод и путь | Тело → ответ |
|---|---|
| `GET /admin/api/screens` | → `[{"id", "title", "updatedAt", "publishedRev": 4 \| null, "unpublished": bool}]` |
| `POST /admin/api/screens` | `{"id", "title"}` → 201 `{"id", "title", "draft": <исходник>}`; черновик по умолчанию — корень `stack` с одним `text`; 409 если id занят |
| `GET /admin/api/screens/{id}` | → `{"id", "title", "draft": <исходник>, "publishedRev": n \| null, "updatedAt", "unpublished": bool}` |
| `PUT /admin/api/screens/{id}` | `{"title"?, "draft"}` → `{"updatedAt"}` (черновик сохраняется даже с ошибками компиляции) |
| `POST /admin/api/screens/{id}/publish` | → 200 `{"revision", "etag"}` \| 422 `{"diags": [Diag]}` |
| `POST /admin/api/screens/{id}/unpublish` | → 204 — снимает экран с публикации: `GET /v1/screens/{id}` → 404 `SCREEN_NOT_FOUND` до следующей публикации/отката |
| `GET /admin/api/screens/{id}/revisions` | → `[{"revision", "etag", "publishedAt", "runtimeMin"}]` (новые сверху) |
| `POST /admin/api/screens/{id}/rollback/{rev}` | → `{"revision", "etag"}` — исходник ревизии `rev` публикуется новой ревизией и становится черновиком |
| `POST /admin/api/compile` | `{"source": <исходник>}` → `{"ok": bool, "diags": [Diag], "document": {…} \| null}` |
| `GET /admin/api/catalog` | (`templates[].params[].default` — в формате значения исходника §3.1: JSON-литерал, `@token` или `=выражение`) → `{"primitives": <primitives.json>, "stdlib": <stdlib.json>, "templates": [{"name", "params": [{"name", "type", "default"?}], "events": ["tap"], "slot": bool}], "tokens": {"color": [..], "type": [..], "space": [..], "radius": [..], "motion": [..]}, "icons": [..], "ops": [{"id", "kind", "args", "result"}], "brands": ["korpu", "kz"]}` |
| `GET /admin/api/templates/{name}` | → исходник шаблона (только чтение) |
| `POST /admin/api/preview` | `{"screen": "id"?}` → `{"code": "K7Q2"}` |
| `POST /admin/api/preview/{code}/document` | `{"source": <исходник>}` → `{"ok", "diags": [Diag]}`; при `ok` документ (с данными) рассылается устройствам сессии, а админкам — доска этого экрана (ниже); сессия переходит на `screen` документа |
| `POST /admin/api/preview/{code}/play` | `{"timeline": "intro"}` → 204 |
| `POST /admin/api/preview/{code}/retry` | → 204 — сбрасывает проваленные снимки сессии, их экраны рендерятся заново |
| `GET /admin/api/preview/{code}/ws?token=` | WebSocket, сервер → админка: `devices` при каждом изменении, `log`, `board`, `frame`, `frames` (ниже) |

**WebSocket админки.** Сообщения сервера:

```jsonc
{"type": "devices", "list": [{"id": "d1", "platform": "ios", "device": "iPhone 17 Pro", "render": true}]}
{"type": "log", "device": "d1", "level": "warn", "message": "…"}
{"type": "board", "focus": "raffle.main", "screens": [
  {"id": "raffle.main", "title": "Розыгрыш", "links": [{"to": "raffle.tickets", "kind": "navigate", "via": "node:progress"}]},
  {"id": "raffle.gone", "title": "", "links": [], "error": "screen not found"}]}
{"type": "frame", "screen": "raffle.main", "device": "d1", "status": "rendering"}
{"type": "frame", "screen": "raffle.main", "device": "d1", "status": "ready", "src": "data:image/jpeg;base64,…", "width": 402, "height": 874, "settled": true}
{"type": "frame", "screen": "raffle.main", "device": "d2", "status": "failed", "error": "timeout"}
{"type": "frames", "list": [ /* объекты frame без "type" */ ]}
```

- `board` — открытый экран (`focus`, первым в `screens`, скомпилирован из присланного исходника) и сохранённые черновики экранов, связанных с ним через `navigate`/`present` с литералом-целью, в обе стороны: компонента связности, обход в ширину от фокуса, не больше 16 экранов. `links` — переходы экрана без дублей, по `(to, kind, via)`; `via` — `node:<id>` или `handler:<name>`. Экран с `error` не рендерится: цели нет среди экранов (`"screen not found"`) или черновик не компилируется (`"draft does not compile: <первая диагностика>"`, `links: []`). Доска приходит, когда меняется её состав, заголовки или ссылки.
- `frame` — снимок экрана `screen` с устройства `device`: `rendering` (задание выдано; прежний снимок, если был, остаётся актуальным до ответа), `ready` (`src` — только `data:image/jpeg;base64,…`, `width`/`height` в pt/dp, `settled`) или `failed` (`error`).
- При подключении админки сервер шлёт `devices`, затем, если доска есть, `board` и одним сообщением `frames` все хранимые снимки (`ready`/`failed`) и задания в работе (`rendering`, после снимка того же экрана и устройства). Очередь пира — 32 сообщения, поэтому снимки не повторяются по одному.

`Diag` = `{"path": "root/children/2/props/text" | "script" | "timelines/0/tracks/1", "line": 1, "col": 5, "message": "…"}` — `line`/`col` относительно DSL-фрагмента по `path` (1-based), `0` если к фрагменту не относится. Сегменты `path` — ключи исходного JSON (§3.1 спеки): `root/children/<i>/props/<prop>`, `…/on/<event>`, `…/when`, `…/repeat/over`, `…/repeat/key`, `…/animate/<prop>`, `…/transition/in`, `…/matchId`, `…/drag`, `…/a11y/label`, `…/id`, `script`, `timelines/<i>`, `timelines/<i>/tracks/<j>/keys/<k>`; узел целиком — путь самого узла (`root/children/2`).

В `catalog`: `tokens.<group>` и `icons` — массивы строк-имён (без `@group.`).

Статика админки — `GET /admin/*` (SPA, `index.html` для неизвестных путей).
