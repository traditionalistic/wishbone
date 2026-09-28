# Raffles, слоты и стек хоста — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** экран розыгрыша Stratoapp на Wishbone с данными из stage GraphQL, инлайн-слоты (баннер розыгрыша в нативной ленте), навигация в стек хоста; интеграция в ленту Stratoapp.

**Architecture:** Go-сервер получает операции `raffle.*`, проксирующие в GraphQL NewBackend с токеном пользователя (или токеном тестового аккаунта для чтений без `Authorization`); SDK (iOS/Android) получают `onNavigate` у `WishboneScreen` и новый `WishboneSlot`; экраны `raffle.*` и слот `slot.feed.top` — исходники в `contract/screens/`.

**Tech Stack:** Go 1.22+ (net/http, pgx), SwiftUI (iOS 18, Swift 6, Swift Testing), Kotlin/Compose (JUnit 5), React+TS (admin), DSL Wishbone.

**Spec:** `docs/superpowers/specs/2026-09-28-raffles-slots-host-nav-design.md` (читать целиком перед любой задачей). Базовая спека — `docs/superpowers/specs/2026-09-27-wishbone-v1-design.md`, обязательные правила — `docs/superpowers/decisions/*.md`.

## Global Constraints

- Паритет iOS/Android — обязательное правило (decisions/renderers.md): одинаковые имена, одинаковая семантика.
- Без новых зависимостей (Go, Swift, Kotlin, npm).
- Токены, заголовки `Authorization`, тела ответов GraphQL и значения форм не логируются.
- Строки UI — ru; комментарии в коде — английские, как в соседнем коде; коммиты — по-русски, с трейлером `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.
- Stage GraphQL: `https://stage.korputeam.ru/graphql`; тестовый аккаунт: телефон `79956791664`, код `133767` (`authenticate(phoneNumber, code)` без `sendCode`; access-токен живёт 15 минут, JWT с `exp`).
- Stratoapp — только локальная ветка, без пуша. Тест-сервер (`deploy/push.sh`) — разрешён.
- Проверки: `cd backend && go test ./... && go vet ./...`; `cd ios && swift test`; `cd android && ./gradlew :wishbone-core:test :wishbone-ui:testDebugUnitTest`; `cd admin && npm test && npm run build`.
- Backend-тесты API требуют локальный Postgres 17 (как в V1: `wishbone_test` создаётся тестами).

---

### Task 1: Таймаут операции (`timeoutMs`)

**Files:**
- Modify: `backend/internal/dsl/stdlib.go` (тип `Op`, `LoadOps`)
- Modify: `backend/internal/api/data.go` (`resolve`, `source`)
- Modify: `contract/README.md` или `contract/api.md` (одна строка о `timeoutMs`)
- Test: `backend/internal/dsl/types_test.go` (или новый `stdlib_test.go`), `backend/internal/api/public_test.go`

**Interfaces:**
- Produces: `dsl.Op.Timeout time.Duration` (0 = по умолчанию); в `operations.json` — необязательное поле `"timeoutMs": 3000`.

- [ ] **Step 1: Failing test for LoadOps**

```go
func TestLoadOpsTimeout(t *testing.T) {
	m, err := LoadOps([]byte(`{"operations": {"a.x": {"kind": "read", "args": "{}", "result": "number", "timeoutMs": 3000}, "a.y": {"kind": "read", "args": "{}", "result": "number"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["a.x"].Timeout != 3*time.Second || m["a.y"].Timeout != 0 {
		t.Fatalf("timeouts: %v %v", m["a.x"].Timeout, m["a.y"].Timeout)
	}
}
```

- [ ] **Step 2:** `cd backend && go test ./internal/dsl -run TestLoadOpsTimeout` → FAIL (`Timeout` undefined).

- [ ] **Step 3: Implement**

```go
type Op struct {
	ID, Kind     string // Kind: "read" | "write"
	Args, Result *Type
	Timeout      time.Duration // data-source timeout; 0 = the server default
}
// in LoadOps: Operations map[string]struct{ Kind, Args, Result string; TimeoutMs int }
ops[id] = &Op{ID: id, Kind: o.Kind, Args: args, Result: res, Timeout: time.Duration(o.TimeoutMs) * time.Millisecond}
```

In `api/data.go`:

```go
// timeout of one data source: the op's timeoutMs, else sourceTimeout.
func (s *Server) timeout(op string) time.Duration {
	if o := s.Contract.Ops[op]; o != nil && o.Timeout > 0 {
		return o.Timeout
	}
	return sourceTimeout
}
```
`resolve`: budget = `dataBudget`, raised to `s.timeout(d.Op) + 200*time.Millisecond` for the slowest source; `source`: `context.WithTimeout(ctx, s.timeout(d.Op))`. Update the doc comment of `resolve` («500 ms each» → «per-op timeout»).

- [ ] **Step 4: API test** — in `public_test.go`, with a test contract dir (copy of `../contract` in `t.TempDir()` with an extra op `t.slow` `timeoutMs: 900` whose handler sleeps 700 ms, and a screen using `data v = t.slow()`): `GET /v1/screens/<id>` returns `data.v.ok == true`. Without `timeoutMs` the same handler fails with `DATA_SOURCE_FAILED`. Use the existing helpers (`newEnvWith`, `e.do`); register the extra handler by building `Config.Ops` with `maps.Copy`.

- [ ] **Step 5:** `go test ./... && go vet ./...` → PASS. Add to `contract/api.md` under «Публичный API»: «`operations.json` → `timeoutMs` (необязательно) — таймаут источника данных этой операции; по умолчанию 500 мс; бюджет экрана — max(1200 мс, самый долгий + 200 мс)».

- [ ] **Step 6: Commit** — `Бэкенд: таймаут источника данных задаётся операцией (timeoutMs)`.

---

### Task 2: Снятие с публикации

**Files:**
- Create: `backend/internal/store/migrations/002_unpublish.sql`
- Modify: `backend/internal/store/store.go` (`Screen`, `Screens`, `Screen`, `Live`, `Publish`, new `Unpublish`)
- Modify: `backend/internal/api/admin.go` (`screenJSON`/`toJSON`, handler `unpublish`), `backend/internal/api/server.go` (route)
- Modify: `contract/api.md` (admin table)
- Test: `backend/internal/store/store_test.go`, `backend/internal/api/admin_test.go`

**Interfaces:**
- Produces: `POST /admin/api/screens/{id}/unpublish` → 204; `GET /admin/api/screens[/{id}]` → `"unpublished": bool`; `store.(*Store).Unpublish(ctx, id, actor) error`; `store.Screen.Unpublished bool`.

- [ ] **Step 1: Migration**

```sql
-- A screen taken off the air: GET /v1/screens answers SCREEN_NOT_FOUND until the next publish/rollback.
ALTER TABLE screens ADD COLUMN unpublished_at timestamptz;
```

- [ ] **Step 2: Failing API test** (append to `admin_test.go`):

```go
func TestUnpublish(t *testing.T) {
	e := newEnv(t)
	e.admin("POST", "/screens", map[string]string{"id": "t.slot", "title": "Slot"})
	if code, b := e.admin("POST", "/screens/t.slot/publish", nil); code != 200 {
		t.Fatalf("publish: %d %s", code, b)
	}
	if code, _, _ := e.do("GET", "/v1/screens/t.slot", nil); code != 200 {
		t.Fatalf("live before unpublish: %d", code)
	}
	if code, b := e.admin("POST", "/screens/t.slot/unpublish", nil); code != 204 {
		t.Fatalf("unpublish: %d %s", code, b)
	}
	code, _, b := e.do("GET", "/v1/screens/t.slot", nil)
	wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
	_, b = e.admin("GET", "/screens/t.slot", nil)
	if m := obj(t, b); m["unpublished"] != true || m["publishedRev"] == nil {
		t.Fatalf("admin view of an unpublished screen: %s", b)
	}
	if code, b := e.admin("POST", "/screens/t.slot/publish", nil); code != 200 {
		t.Fatalf("republish: %d %s", code, b)
	}
	if code, _, _ := e.do("GET", "/v1/screens/t.slot", nil); code != 200 {
		t.Fatalf("publish puts it back on the air: %d", code)
	}
	code, b = e.admin("POST", "/screens/nope/unpublish", nil)
	wantErr(t, code, b, 404, "SCREEN_NOT_FOUND")
}
```
Also: after unpublish, a restart-like `srv.Seed(ctx)` must NOT republish a seeded screen — add to the existing `TestSeed`: unpublish a seeded id, call `e.srv.Seed(ctx)`, expect still 404.

- [ ] **Step 3:** `go test ./internal/api -run 'TestUnpublish|TestSeed'` → FAIL.

- [ ] **Step 4: Implement store**
  - `Screen` gets `Unpublished bool`; `Screens`/`Screen` select `unpublished_at IS NOT NULL`.
  - `Live`: `SELECT … FROM revisions r JOIN screens s ON s.id = r.screen_id WHERE r.screen_id = $1 AND s.unpublished_at IS NULL ORDER BY rev DESC LIMIT 1` (no row → `ErrNotFound`).
  - `Publish` (both publish and rollback): in the same tx `UPDATE screens SET unpublished_at = NULL WHERE id = $1`.
  - `Unpublish(ctx, id, actor)`: tx `UPDATE screens SET unpublished_at = coalesce(unpublished_at, clock_timestamp()) WHERE id = $1` (0 rows → `ErrNotFound`) + `audit(…, "unpublish", id, nil)`.
  - `Recompile` / `Revision` / `/v1/data` / `/v1/call` stay by revision number (open screens finish).
- [ ] **Step 5: Implement API** — `admin("POST /admin/api/screens/{id}/unpublish", s.unpublish)`; handler → `s.Store.Unpublish(r.Context(), id, "admin")`, errors through `s.screenErr`, success `w.WriteHeader(204)`; `screenJSON` gains `Unpublished bool \`json:"unpublished"\``.
- [ ] **Step 6:** store test `TestUnpublish` in `store_test.go` (Live → ErrNotFound after Unpublish, Publish clears) → `go test ./... && go vet ./...` PASS.
- [ ] **Step 7:** `contract/api.md`: row `POST /admin/api/screens/{id}/unpublish` → 204, «снимает экран с публикации: `GET /v1/screens/{id}` → 404 до следующей публикации/отката»; в описании `GET /admin/api/screens` — поле `unpublished`.
- [ ] **Step 8: Commit** — `Бэкенд: снятие экрана с публикации (unpublish)`.

---

### Task 3: GraphQL-клиент апстрима

**Files:**
- Create: `backend/internal/ops/gql.go`
- Test: `backend/internal/ops/gql_test.go`

**Interfaces:**
- Produces:

```go
// GQL talks to the NewBackend GraphQL upstream (spec 2026-09-28 §3.1).
type GQL struct {
	URL   string       // "" → every call fails with UPSTREAM_FAILED
	Login string       // "phone:code" of a stage test account: the identity of reads without Authorization
	HTTP  *http.Client // nil → a client with a 10 s timeout
	Now   func() time.Time
	// unexported: mu sync.Mutex, token string, exp time.Time
}

// failure is a domain error of an upstream call; handlers turn it into ops.Fail(code, msg).
type failure struct{ code, msg string }
func (f *failure) Error() string

// Do runs one named operation. auth is the caller's Authorization value or "" (the fallback identity, reads only).
func (g *GQL) Do(ctx context.Context, auth string, write bool, name, query string, vars map[string]any, out any) error

// fail converts any error of Do into a Result.
func fail(err error) Result
```

- [ ] **Step 1: Failing tests** (`gql_test.go`), fake upstream with `httptest.NewServer`:

```go
// upstream answers by operationName with a canned body and records the Authorization it saw.
type upstream struct {
	t      *testing.T
	bodies map[string]string // operationName → response body
	auth   []string
	logins int
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	u.auth = append(u.auth, r.Header.Get("Authorization"))
	if req.OperationName == "WishboneLogin" {
		u.logins++
		exp := time.Now().Add(15 * time.Minute).Unix()
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d,"id":1188}`, exp)))
		fmt.Fprintf(w, `{"data":{"authenticate":{"accessToken":"h.%s.s"}}}`, payload)
		return
	}
	b, ok := u.bodies[req.OperationName]
	if !ok {
		w.WriteHeader(500)
		return
	}
	io.WriteString(w, b)
}
```
Tests:
  1. `TestGQLForwardsAuthorization`: `Do(ctx, "Bearer u", false, "Q", "query Q { x }", nil, &out)` → upstream saw `Bearer u`, `out` decoded from `{"data":{"x":1}}`.
  2. `TestGQLFallbackLoginForReadsOnly`: `Login: "79956791664:133767"`; two reads with auth `""` → one login, both carried `Bearer h.<payload>.s`; a write with `""` → `*failure` code `UNAUTHENTICATED`, no upstream call; with `Now` moved past `exp-60s` → second login.
  3. `TestGQLErrors`: `{"errors":[{"extensions":{"code":"UNAUTHENTICATED"}}],"data":null}` → code `UNAUTHENTICATED`; `{"errors":[{"message":"boom"}],"data":null}` → `UPSTREAM_FAILED`; HTTP 500 → `UPSTREAM_FAILED`; `URL: ""` → `UPSTREAM_FAILED`; partial `{"errors":[…],"data":{"x":1}}` → no error, `out` decoded; `UNAUTHENTICATED` on a fallback-token request clears the cached token (next read logs in again).
  4. `TestFailResult`: `fail(&failure{"UNAUTHENTICATED", "…"})` → `Result{Error: {Code: "UNAUTHENTICATED"}}`; `fail(errors.New("x"))` → `UPSTREAM_FAILED`.

- [ ] **Step 2:** `go test ./internal/ops -run 'TestGQL|TestFailResult'` → FAIL.

- [ ] **Step 3: Implement** `gql.go`:

```go
const (
	msgAuth     = "Войдите в аккаунт, чтобы участвовать в розыгрыше"
	msgUpstream = "Не удалось загрузить розыгрыш"
)

func (g *GQL) Do(ctx context.Context, auth string, write bool, name, query string, vars map[string]any, out any) error {
	fallback := auth == ""
	if fallback {
		if write || g.Login == "" {
			return &failure{"UNAUTHENTICATED", msgAuth}
		}
		t, err := g.fallbackToken(ctx)
		if err != nil {
			return err
		}
		auth = "Bearer " + t
	}
	err := g.post(ctx, auth, name, query, vars, out)
	var f *failure
	if fallback && errors.As(err, &f) && f.code == "UNAUTHENTICATED" {
		g.mu.Lock()
		g.token = "" // expired early or revoked: log in again next time
		g.mu.Unlock()
	}
	return err
}
```
  - `post`: `URL == ""` → `UPSTREAM_FAILED`; JSON body `{"operationName": name, "query": query, "variables": vars}`; headers `Content-Type: application/json`, `Authorization` if non-empty, `Accept-Language` is NOT forwarded (ru only, spec ceiling); non-200 → `UPSTREAM_FAILED`; decode `{"data": json.RawMessage, "errors": [{"message","extensions":{"code"}}]}`; any `extensions.code == "UNAUTHENTICATED"` → `UNAUTHENTICATED`; `data` missing/`null` → `UPSTREAM_FAILED`; else `json.Unmarshal(data, out)`.
  - `fallbackToken`: under `g.mu`: cached and `Now() < exp-60s` → return; else `post(ctx, "", "WishboneLogin", "mutation WishboneLogin($p: String!, $c: Int!) { authenticate(phoneNumber: $p, code: $c) { accessToken } }", {"p": phone, "c": code}, &resp)`; parse `exp` from the JWT payload (`strings.Split(tok, ".")[1]`, `base64.RawURLEncoding`), missing exp → cache 5 minutes. `Login` is split once on `:`; code parsed with `strconv.Atoi` (bad format → `UPSTREAM_FAILED` + one `slog.Error` without the value).
  - `fail(err)`: `*failure` → `Fail(code, msg)`; else `slog.Warn("upstream call failed", "err", err)` (no bodies) and `Fail("UPSTREAM_FAILED", msgUpstream)`.
  - Upstream network errors (timeout, refused) → `UPSTREAM_FAILED` (wrap as `*failure`).
- [ ] **Step 4:** tests PASS; `go vet ./...`.
- [ ] **Step 5: Commit** — `Бэкенд: GraphQL-клиент апстрима (прокидывание токена, вход тестовым аккаунтом для чтений)`.

---

### Task 4: Сборка данных розыгрыша (чистые функции)

**Files:**
- Create: `backend/internal/ops/raffle_build.go`
- Test: `backend/internal/ops/raffle_build_test.go` (фикстуры `testdata/raffle/*.json` — ответы stage, уже в репо)

**Interfaces:**
- Consumes: фикстуры `backend/internal/ops/testdata/raffle/{overview,leaderboard,users,tickets,invites,me,unauth}.json` (формат `{"data": …}` ответа GraphQL).
- Produces (все возвращают `map[string]any`/`[]any` — JSON-формы типов из `operations.json`, Task 5):

```go
type gqlTask struct {
	TaskType        string  `json:"taskType"`
	Name            string  `json:"name"`
	Count           int     `json:"count"`
	Priority        int     `json:"priority"`
	Progress        int     `json:"progress"`
	Target          *int    `json:"target"`
	Completed       bool    `json:"completed"`
	NextAvailableAt *string `json:"nextAvailableAt"`
	TicketsEarned   int     `json:"ticketsEarned"`
}
type gqlRule struct {
	TaskType     string `json:"taskType"`
	Name         string `json:"name"`
	TicketsCount int    `json:"ticketsCount"`
	Description  string `json:"description"`
	IconURL      string `json:"iconUrl"`
	IconURLSmall string `json:"iconUrlSmall"`
	Priority     int    `json:"priority"`
	Target       *int   `json:"target"`
	IsPromo      bool   `json:"isPromo"`
}
type gqlPrize struct {
	RaffleID string `json:"raffleId"`
	Prize    struct {
		TicketsCount int    `json:"ticketsCount"`
		Name         string `json:"name"`
		ImageURL     string `json:"imageUrl"`
	} `json:"prize"`
}
type gqlOverview struct {
	DailyBonusRewards []struct{ StreakDay, Amount int } `json:"dailyBonusRewards"`
	CurrentDailyBonus struct {
		StreakDay    int  `json:"streakDay"`
		ClaimedToday bool `json:"claimedToday"`
	} `json:"currentDailyBonus"`
	GetMyProgressRaffle *struct {
		TicketsCount int       `json:"ticketsCount"`
		Hidden       bool      `json:"hidden"`
		Items        []gqlTask `json:"items"`
	} `json:"getMyProgressRaffle"`
	GetPrizes *struct {
		MainPrize *gqlPrize  `json:"mainPrize"`
		Prizes    []gqlPrize `json:"prizes"`
	} `json:"getPrizes"`
	GetRaffleUsersInfo *struct {
		Total               int  `json:"total"`
		CurrentUserPosition *int `json:"currentUserPosition"`
	} `json:"getRaffleUsersInfo"`
	GetRaffleTaskRules []gqlRule `json:"getRaffleTaskRules"`
	GetRaffleTime      *struct{ Start, End string } `json:"getRaffleTime"`
	GetUser            *struct{ MyRefCode *string `json:"myRefCode"` } `json:"getUser"`
}

func buildOverview(d *gqlOverview, now time.Time) (map[string]any, error) // error → UPSTREAM_FAILED if required parts are nil
func buildPromo(d *gqlOverview, now time.Time) map[string]any              // uses GetRaffleTime, progress items, rules, position
func displayName(name *string, isMe, hidden bool) string
func grouped(n int) string        // 50427 → "50 427", -1200 → "-1 200"
func msTime(iso string) any       // RFC 3339 (with or without fraction) → float64 ms; "" or bad → nil
func shareText(code string) string
```

- [ ] **Step 1: Failing tests** (expected values computed from the fixtures):

```go
func loadOverview(t *testing.T) *gqlOverview {
	t.Helper()
	b, err := os.ReadFile("testdata/raffle/overview.json")
	if err != nil {
		t.Fatal(err)
	}
	var env struct{ Data gqlOverview }
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	me, _ := os.ReadFile("testdata/raffle/me.json")
	var m struct{ Data gqlOverview }
	_ = json.Unmarshal(me, &m)
	env.Data.GetUser = m.Data.GetUser
	return &env.Data
}

var fixtureNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func TestBuildOverview(t *testing.T) {
	o, err := buildOverview(loadOverview(t), fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if o["tickets"] != 1250.0 || o["place"] != 11.0 || o["participants"] != 330.0 || o["hidden"] != false {
		t.Fatalf("header: %v %v %v %v", o["tickets"], o["place"], o["participants"], o["hidden"])
	}
	if o["endsAt"] != float64(time.Date(2026, 10, 10, 18, 0, 0, 0, time.UTC).UnixMilli()) {
		t.Fatalf("endsAt: %v", o["endsAt"])
	}
	daily := o["daily"].(map[string]any)
	days := daily["days"].([]any)
	if len(days) != 7 || days[0].(map[string]any)["state"] != "available" || days[1].(map[string]any)["state"] != "waiting" || daily["claimAmount"] != 2.0 {
		t.Fatalf("daily: %v", daily)
	}
	if mp := o["mainPrize"].(map[string]any); mp["title"] != "Автомобиль" {
		t.Fatalf("main prize: %v", mp)
	}
	prizes := o["prizes"].([]any)
	first := prizes[0].(map[string]any)
	if len(prizes) != 16 || first["rank"] != 1.0 || first["title"] != "Автомобиль" || first["tickets"] != 12000.0 {
		t.Fatalf("prizes: %d %v", len(prizes), first)
	}
	// Acoin prizes share one image and keep it ("coin" exception); nothing else repeats in this fixture
	if prizes[15].(map[string]any)["image"] == nil {
		t.Fatal("coin prizes keep their shared image")
	}
	tasks := o["tasks"].([]any)
	if len(tasks) != 15 || tasks[0].(map[string]any)["id"] != "SELF_EVENT_CREATED" {
		t.Fatalf("tasks: %d %v", len(tasks), tasks[0])
	}
	if o["tasksDone"] != 5.0 || o["tasksTotal"] != 15.0 || len(o["rules"].([]any)) != 6 {
		t.Fatalf("summary: %v/%v rules %d", o["tasksDone"], o["tasksTotal"], len(o["rules"].([]any)))
	}
	if !strings.HasPrefix(o["share"].(string), "📱 Скачай приложение: https://korpu.app/ref?uid=1188\n\n") {
		t.Fatalf("share: %q", o["share"])
	}
}

func TestTaskProgressRows(t *testing.T) {
	o, _ := buildOverview(loadOverview(t), fixtureNow)
	byID := map[string][]any{}
	for _, x := range o["tasks"].([]any) {
		m := x.(map[string]any)
		byID[m["id"].(string)] = m["progress"].([]any)
	}
	row := func(id string, i int) map[string]any { return byID[id][i].(map[string]any) }
	// hours task: hours + tickets received
	if r := row("SELF_HOUR_IN_APP", 0); r["label"] != "Проведено часов:" || r["value"] != "100" {
		t.Fatalf("hours: %v", r)
	}
	if r := row("SELF_HOUR_IN_APP", 1); r["label"] != "Получено билетов:" || r["value"] != "1 000" || r["ticket"] != true {
		t.Fatalf("hours tickets: %v", r)
	}
	// steps task: walked + remaining, grouped by thousands
	if r := row("SELF_STEPS_GOAL", 0); r["label"] != "Пройдено шагов:" || r["value"] != "50 427" {
		t.Fatalf("steps: %v", r)
	}
	if r := row("SELF_STEPS_GOAL", 1); r["label"] != "Осталось:" || r["value"] != "149 573" {
		t.Fatalf("remaining: %v", r)
	}
	// target > 1: "a/b", no tickets row while nothing earned
	if len(byID["SELF_COMMENT_POSTED"]) != 1 || row("SELF_COMMENT_POSTED", 0)["value"] != "1/5" {
		t.Fatalf("comment: %v", byID["SELF_COMMENT_POSTED"])
	}
	// nothing done yet and no target: no rows
	if len(byID["SELF_EVENT_CREATED"]) != 0 {
		t.Fatalf("event: %v", byID["SELF_EVENT_CREATED"])
	}
}

func TestDisplayName(t *testing.T) {
	n := "Иван Петров"
	cases := []struct {
		name         *string
		isMe, hidden bool
		want         string
	}{
		{&n, false, false, "Иван Петров"},
		{nil, false, false, "**********"},
		{&n, true, false, "Я Иван Петров"},
		{&n, true, true, "Я **********"},
		{nil, true, false, "Я"},
		{nil, true, true, "**********"},
	}
	for _, c := range cases {
		if got := displayName(c.name, c.isMe, c.hidden); got != c.want {
			t.Errorf("displayName(%v, %v, %v) = %q, want %q", c.name, c.isMe, c.hidden, got, c.want)
		}
	}
}

func TestGroupedAndTime(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1 000", 149573: "149 573", -1200: "-1 200", 1000000: "1 000 000"} {
		if got := grouped(n); got != want {
			t.Errorf("grouped(%d) = %q", n, got)
		}
	}
	if msTime("2026-09-10T15:09:41.60322Z") != float64(time.Date(2026, 9, 10, 15, 9, 41, 603220000, time.UTC).UnixMilli()) || msTime("") != nil || msTime("nope") != nil {
		t.Fatal("msTime")
	}
}

func TestBuildPromo(t *testing.T) {
	d := loadOverview(t)
	p := buildPromo(d, fixtureNow)
	if p["active"] != true || p["place"] != 11.0 || p["tasksDone"] != 5.0 || p["tasksTotal"] != 15.0 {
		t.Fatalf("promo: %v", p)
	}
	if buildPromo(d, time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))["active"] != false {
		t.Fatal("inactive after the end")
	}
	d.GetRaffleTime = nil
	if buildPromo(d, fixtureNow)["active"] != false {
		t.Fatal("no raffle time → inactive")
	}
}
```

- [ ] **Step 2:** `go test ./internal/ops -run 'TestBuild|TestTask|TestDisplay|TestGrouped'` → FAIL.

- [ ] **Step 3: Implement** — faithful port of Stratoapp `Modules/Raffle/Domain/Sources/Models/RaffleOverviewAssembler.swift` and `Modules/CoreNetworking/Sources/RaffleResponseMappers.swift` (read both):
  - **Rules:** dedupe by `(isPromo, taskType, name)` keeping the first; `tasks` = non-promo, sorted by `priority` desc (stable); `rules` = promo rules in upstream order: `{name, description, icon: iconUrlSmall|iconUrl|nil (trimmed, "" → nil), tickets: ticketsCount}`.
  - **Task join:** progress items grouped by `taskType`; `ticketsEarned` = Σ, `count` = Σ, `progress` = max, `target` = first item's target ?? rule target; `cooldownUntil` = max `nextAvailableAt` if it is after `now` (ms), else nil. Progress rows: cooldown → only the tickets row; no items → `[]`; steps tasks (`FRIEND_STEPS_RATING`, `SELF_STEPS_GOAL`): target ≤ 0 → `[]`, else «Пройдено шагов:» `grouped(progress)` + «Осталось:» `grouped(target − progress)` when target > progress; hours task (`SELF_HOUR_IN_APP`): «Проведено часов:» `grouped(count)` + tickets row; target > 1: «Выполнено:» `"a/b"` with `a = min(count, target)` + tickets row; else if count > 0 || tickets > 0: «Выполнено:» `grouped(count)` + tickets row; tickets row = «Получено билетов:» `grouped(ticketsEarned)`, `ticket: true`, only when ticketsEarned > 0. Task: `{id: taskType, name: rule.name, description: rule.description, icon: rule icon (small first), tickets: rule.ticketsCount, cooldownUntil, progress}`.
  - **Summary:** active types = non-promo rule types; `tasksDone` = |completed item types ∩ active|, `tasksTotal` = |active|.
  - **Prizes:** group by `raffleId`, take the lexicographically smallest id's group (all prizes if none); sort by tickets desc, then name asc; `rank = i+1`; image = trimmed URL or nil; a repeated URL becomes nil unless the name contains "coin" (case-insensitive). `mainPrize` = `{title, image}` from `getPrizes.mainPrize` or nil.
  - **Daily:** sort by day; `day < streakDay` → claimed; `== streakDay` → claimedToday ? claimed : available; else waiting. `claimAmount` = amount of the streak day when not claimedToday, else nil.
  - **Header:** `tickets` = progress ticketsCount, `hidden`, `place` = currentUserPosition (nil → nil), `participants` = users-info total, `startsAt`/`endsAt` = `msTime`, `share` = `shareText(myRefCode)` when non-empty, else nil.
  - `buildOverview` errors (→ `UPSTREAM_FAILED` in Task 5) when `GetMyProgressRaffle`, `GetPrizes` or `GetRaffleUsersInfo` is nil.
  - `buildPromo`: `active` = raffle time parses and `start ≤ now < end`; `endsAt`; `place`; `tasksDone/Total` as above.
  - `shareText(code)` = `"📱 Скачай приложение: " + code + "\n\n" + shareBody` where `shareBody` = `"Залетай по моей ссылке, скачивай приложение 📲 и участвуй в розыгрыше!\n\nСреди призов — автомобиль Geely Coolray 🚗, iPhone 17 Pro Max 📱, MacBook 💻, PlayStation 5 🎮 и многое другое 🎁✨"` (Stratoapp `RaffleShare.xcstrings`).
  - Numbers in results are `float64` (JSON numbers); optional fields present with `nil` value.
- [ ] **Step 4:** tests PASS.
- [ ] **Step 5: Commit** — `Бэкенд: сборка данных розыгрыша (задания, призы, дейли, маскирование) по правилам Stratoapp`.

---

### Task 5: Операции `raffle.*`

**Files:**
- Create: `backend/internal/ops/raffle.go`
- Modify: `contract/operations.json`, `contract/errors.json`
- Modify: `backend/cmd/server/main.go`, `backend/internal/api/api_test.go` (`newEnvWith` handlers), `backend/internal/ops/ops_test.go` (`TestCheckCoversContract`, result-type check)
- Test: `backend/internal/ops/raffle_test.go`

**Interfaces:**
- Consumes: `GQL.Do`, `fail`, `build*` (Tasks 3–4).
- Produces: `type Raffle struct{ G *GQL; Now func() time.Time }`, `func (r *Raffle) Handlers() map[string]Handler`; env `WISHBONE_GRAPHQL_URL`, `WISHBONE_GRAPHQL_LOGIN`.

- [ ] **Step 1: Contract.** `operations.json` — add (types on one line each, exactly):

| id | kind | timeoutMs | args | result |
|---|---|---|---|---|
| `raffle.overview` | read | 3000 | `{}` | `{tickets: number, place: number?, participants: number, hidden: bool, startsAt: number?, endsAt: number?, daily: {streakDay: number, claimedToday: bool, claimAmount: number?, days: list<{day: number, amount: number, state: "claimed" \| "available" \| "waiting"}>}, mainPrize: {title: string, image: string?}?, prizes: list<{rank: number, title: string, tickets: number, image: string?}>, tasks: list<{id: string, name: string, description: string, icon: string?, tickets: number, cooldownUntil: number?, progress: list<{label: string, value: string, ticket: bool}>}>, rules: list<{name: string, description: string, icon: string?, tickets: number}>, tasksDone: number, tasksTotal: number, share: string?}` |
| `raffle.promo` | read | 3000 | `{}` | `{active: bool, endsAt: number?, place: number?, tasksDone: number, tasksTotal: number}` |
| `raffle.leaderboard` | read | 3000 | `{offset: number, limit: number}` | `{total: number, myPosition: number?, items: list<{key: string, position: number, name: string, tickets: number, isMe: bool}>}` |
| `raffle.tickets` | read | 3000 | `{offset: number, limit: number}` | `{total: number, items: list<{id: string, createdAt: number}>}` |
| `raffle.invites` | read | 3000 | `{}` | `{total: number, maxTicketsPerInvite: number, items: list<{key: string, name: string, tickets: number}>}` |
| `raffle.claimDaily` | write | — | `{}` | `{streakDay: number}` |
| `raffle.toggleHidden` | write | — | `{}` | `{hidden: bool}` |

`errors.json`: `"UPSTREAM_FAILED": {"http": 502, "retryable": true, "client": "per-source/call {ok:false}; the screen shows its error state"}` (keep the file's key order style; `TestNewRequiresErrorCodes` must stay green).

- [ ] **Step 2: Failing tests** (`raffle_test.go`) — the fake upstream from Task 3 (move `upstream` into a shared `testutil_test.go` in package `ops`), bodies from fixtures by operationName:

```go
func raffleEnv(t *testing.T) (*Raffle, *upstream) {
	t.Helper()
	fx := func(name string) string {
		b, err := os.ReadFile("testdata/raffle/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	u := &upstream{t: t, bodies: map[string]string{
		"RaffleOverview":     mergeData(t, fx("overview"), fx("me")),
		"RafflePromo":        fx("overview"),
		"RaffleLeaderboard":  mergeData(t, fx("leaderboard"), `{"data":{"getMyProgressRaffle":{"hidden":false}}}`),
		"RaffleUsers":        fx("users"),
		"RaffleTickets":      fx("tickets"),
		"RaffleInvites":      mergeData(t, fx("invites"), fx("me")),
		"RaffleClaimDaily":   `{"data":{"claimDailyReward":{"streakDay":2}}}`,
		"RaffleToggleHidden": `{"data":{"toggleMyVisibilityInRaffle":true}}`,
	}}
	ts := httptest.NewServer(u)
	t.Cleanup(ts.Close)
	return &Raffle{G: &GQL{URL: ts.URL, Login: "79956791664:133767"}, Now: func() time.Time { return fixtureNow }}, u
}
// mergeData merges the "data" objects of GraphQL responses.
func mergeData(t *testing.T, bodies ...string) string
```
Tests:
  1. `TestRaffleResultsMatchContract`: every handler with `Req{User: "Bearer u", Args: <valid args>}` returns `OK`, and `Validate(contractOps(t)[id].Result, jsonRoundTrip(res.Value)) == nil` (round-trip through `json.Marshal/Unmarshal` so Go ints become float64).
  2. `TestRaffleLeaderboard`: `{offset: 20, limit: 20}` → 20 items, positions 21…40, `key` = `"<position>"`, nil userIds → `"**********"`, user names «Имя Фамилия» from `users.json`, `myPosition` = 11, no `isMe` in this page; the upstream saw `limit: 20, offset: 20` variables.
  3. `TestRaffleAnonymousReadsUseTheTestAccount`: `Req{User: "anon"}` for `raffle.promo` → OK, upstream saw `Bearer h.…` after one `WishboneLogin`; `raffle.claimDaily` with `anon` → `UNAUTHENTICATED` and no upstream mutation.
  4. `TestRaffleUnauthenticated`: body `unauth.json` for `RaffleOverview` → `Result.Error.Code == "UNAUTHENTICATED"`.
  5. `TestRaffleInvites`: empty fixture → `{total: 0, maxTicketsPerInvite: 30, items: []}`.
  6. `TestRaffleWrites`: claim → `{streakDay: 2}`, toggle → `{hidden: true}`.

- [ ] **Step 3:** `go test ./internal/ops -run TestRaffle` → FAIL.

- [ ] **Step 4: Implement** `raffle.go`. `auth := r.User; if auth == "anon" { auth = "" }`. Queries (named, as consts):

```graphql
query RaffleOverview { dailyBonusRewards { streakDay amount } currentDailyBonus { streakDay claimedToday } getMyProgressRaffle { ticketsCount hidden items { taskType name count priority progress target completed nextAvailableAt ticketsEarned } } getPrizes { mainPrize { raffleId prize { ticketsCount name imageUrl } } prizes { raffleId prize { ticketsCount name imageUrl } } } getRaffleUsersInfo(limit: 1, offset: 0) { total currentUserPosition } getRaffleTaskRules { taskType name ticketsCount description iconUrl iconUrlSmall priority target isPromo } getRaffleTime { start end } getUser { id myRefCode } }
query RafflePromo { getRaffleTime { start end } getMyProgressRaffle { ticketsCount hidden items { taskType completed } } getRaffleTaskRules { taskType name isPromo } getRaffleUsersInfo(limit: 1, offset: 0) { total currentUserPosition } }
query RaffleLeaderboard($limit: Int!, $offset: Int!) { getRaffleUsersInfo(limit: $limit, offset: $offset) { total currentUserPosition items { userId ticketsCount } } getMyProgressRaffle { hidden } }
query RaffleUsers($ids: [Int!]!) { getUsersForRaffle(userIds: $ids) { id firstname lastname } }
query RaffleTickets($limit: Int!, $offset: Int!) { getMyTickets(limit: $limit, offset: $offset) { total items { ticketId createdAt } } }
query RaffleInvites { getMyReferralInvites(limit: 100, offset: 0) { total maxTicketsPerInvite items { userId totalTickets } } getUser { id } }
mutation RaffleClaimDaily { claimDailyReward { streakDay } }
mutation RaffleToggleHidden { toggleMyVisibilityInRaffle }
```
  - leaderboard: `limit` clamped to 1…50, `offset` ≥ 0 (ints from float64 args); positions `offset + i + 1`; `isMe` = position == currentUserPosition; names via `RaffleUsers` for unique positive ids (skip the call when none) — `strings.TrimSpace(firstname + " " + lastname)`, empty → nil; `displayName(name, isMe, hidden)`.
  - tickets: clamp as above; `{id: ticketId, createdAt: msTime}`.
  - invites: drop the current user's own row (`getUser.id`), name = users-for-raffle name ?? `str(userId)`, `tickets` = `totalTickets`, `key` = `str(userId)`; `total` = api total, or the row count when the api total is 0.
  - any `Do` error → `return fail(err), nil`; `buildOverview` error → `fail(&failure{"UPSTREAM_FAILED", msgUpstream})`.
- [ ] **Step 5: Wire.** `main.go`:

```go
handlers := (&ops.Demo{}).Handlers()
maps.Copy(handlers, (&ops.Raffle{G: &ops.GQL{URL: os.Getenv("WISHBONE_GRAPHQL_URL"), Login: os.Getenv("WISHBONE_GRAPHQL_LOGIN")}}).Handlers())
```
(log once at start whether the upstream is configured — URL host only, never the login). `api_test.go` `newEnvWith` and `ops_test.go` `TestCheckCoversContract`: merge `(&Raffle{G: &GQL{}}).Handlers()` into the demo handlers.
- [ ] **Step 6:** `go test ./... && go vet ./...` PASS.
- [ ] **Step 7: Smoke against stage** (manual, not committed): `cd backend && WISHBONE_GRAPHQL_URL=https://stage.korputeam.ru/graphql WISHBONE_GRAPHQL_LOGIN=79956791664:133767 go run ./cmd/server` + a throwaway screen published through the admin API with `data o = raffle.overview()` → `GET /v1/screens/<id>` shows `data.o.ok: true`, tickets ≈ 1250.
- [ ] **Step 8: Commit** — `Бэкенд: операции raffle.* поверх GraphQL NewBackend`.

---

### Task 6: Админка — «Снять с публикации»

**Files:**
- Modify: `admin/src/api.ts`, `admin/src/mock.ts`, `admin/src/ui/TopBar.tsx`, the component that wires TopBar callbacks (`admin/src/ui/App.tsx` or `Workspace.tsx` — where `onPublish` is defined)
- Test: `npm test` (existing suites) + `npm run build`

**Interfaces:**
- Consumes: `POST /admin/api/screens/{id}/unpublish` → 204; `unpublished: boolean` in screen JSON (Task 2).

- [ ] **Step 1:** `api.ts`: `unpublished: boolean` in both screen types; `unpublish: (id: string) => req<void>('POST', \`/screens/${enc(id)}/unpublish\`)` (make `req` accept 204 with an empty body if it does not already).
- [ ] **Step 2:** `mock.ts`: keep an `unpublished` flag per screen; `…/unpublish` sets it (204), publish/rollback clear it; list/get return it.
- [ ] **Step 3:** `TopBar.tsx`: next to «Publish» a button «Снять с публикации» (`disabled` when not published or already unpublished or busy; `confirm('Снять экран с публикации? Клиенты получат SCREEN_NOT_FOUND до следующей публикации.')`); the screen picker label shows ` (снят)` instead of ` (rN)` when `unpublished`; the revisions select's first option reads `Снят с публикации` then. Wire `onUnpublish` like `onPublish` (busy flag, refresh the screen list and current screen afterwards, error → console panel as other admin errors do).
- [ ] **Step 4:** `cd admin && npm test && npm run build` → PASS; check in the dev server with the mock (`npm run dev`, mock mode as documented in `admin/`) that the button toggles the label.
- [ ] **Step 5: Commit** — `Админка: снятие экрана с публикации`.

---

### Task 7: iOS — навигация в стек хоста

**Files:**
- Modify: `ios/Sources/WishboneUI/Navigator.swift` (public `WishboneRoute`, `forward`)
- Modify: `ios/Sources/WishboneUI/WishboneScreen.swift` (`onNavigate`, `init(route:)`, `StackHost` without its own stack)
- Test: `ios/Tests/WishboneUITests/NavigatorTests.swift`

**Interfaces:**
- Produces:

```swift
public struct WishboneRoute: Hashable, Identifiable, Sendable {
    public let id: UUID
    public let screen: String
    public let params: [String: Value]
    /// "push" | "fade" | "present" (slots only) | nil — a hint for the host.
    public let transition: String?
    public init(screen: String, params: [String: Value] = [:], transition: String? = nil)
}
WishboneScreen(_ wishbone: Wishbone, id: String, params: [String: Value] = [:], onExit: (() -> Void)? = nil, onNavigate: ((WishboneRoute) -> Void)? = nil)
WishboneScreen(_ wishbone: Wishbone, route: WishboneRoute, onExit: (() -> Void)? = nil, onNavigate: ((WishboneRoute) -> Void)? = nil)
// Navigator:
@ObservationIgnored var forward: ((WishboneRoute) -> Void)?
@ObservationIgnored var forwardsPresent = false // slots: `present` also goes to the host
```

- [ ] **Step 1: Failing tests** (append to `NavigatorTests`):

```swift
@Test func hostStackReceivesPushes() {
    let nav = Navigator()
    var pushed: [WishboneRoute] = []
    var exited = false
    nav.forward = { pushed.append($0) }
    nav.exit = { exited = true }
    nav.navigate(Route(screen: "raffle.tickets", params: ["n": .number(1)], fade: true))
    #expect(nav.path.isEmpty)
    #expect(pushed.map(\.screen) == ["raffle.tickets"])
    #expect(pushed.first?.transition == "fade")
    #expect(pushed.first?.params["n"] == .number(1))
    nav.back()
    #expect(exited) // the host pops
}

@Test func hostStackKeepsSheetsInTheSDK() {
    let root = Navigator()
    var pushed = 0
    root.forward = { _ in pushed += 1 }
    root.present(Route(screen: "raffle.rules", params: [:]))
    #expect(root.sheet?.screen == "raffle.rules")
    let sheet = Navigator(presenter: root)
    sheet.navigate(Route(screen: "inner", params: [:])) // a sheet's navigator has no forward: pushes inside
    #expect(sheet.path.map(\.screen) == ["inner"] && pushed == 0)
    sheet.dismiss()
    #expect(root.sheet == nil)
}

@Test func slotForwardsPresent() {
    let nav = Navigator()
    var pushed: [WishboneRoute] = []
    nav.forward = { pushed.append($0) }
    nav.forwardsPresent = true
    nav.present(Route(screen: "raffle.rules", params: [:]))
    #expect(nav.sheet == nil && pushed.first?.transition == "present")
}
```
(Check `Value`'s case names in `WishboneCore/Value.swift` and adjust `.number(1)` if the case is named differently.)

- [ ] **Step 2:** `cd ios && swift test --filter NavigatorTests` → FAIL.

- [ ] **Step 3: Implement.** `Navigator.navigate`: `if let forward { return forward(WishboneRoute(route)) }` (a sheet's navigator never gets `forward`, so pushes inside a sheet stay inside); `present`: `if forwardsPresent, let forward { return forward(WishboneRoute(route, transition: "present")) }`. `WishboneRoute(_ route: Route, transition:)` maps `fade` → `"fade"`, else `"push"`; `Route(_ r: WishboneRoute)` maps back (`fade = r.transition == "fade"`). `WishboneScreen`: store `onNavigate`; `init(route:)` builds `Route(route)`; body `.id` = `route.id.uuidString` for route-based screens (keep the current id/params key for `init(id:)`). `StackHost` gets `onNavigate`; when non-nil its body is

```swift
RouteView(wishbone: wishbone, route: root, navigator: navigator)
    .sheet(item: $navigator.sheet) { … same as today … }
    .modifier(EnvReader(env: wishbone.env, geometry: navigator.geometry))
    .onAppear {
        navigator.exit = onExit ?? { dismiss() }
        navigator.forward = onNavigate
    }
```
(no `NavigationStack`); when nil — unchanged. Doc comment of `WishboneScreen`: explain `onNavigate` (spec §1.2 table in one paragraph).
- [ ] **Step 4:** `swift test` → PASS (whole package).
- [ ] **Step 5: Commit** — `iOS: навигация в стек хоста (onNavigate, WishboneRoute)`.

---

### Task 8: iOS — `WishboneSlot`

**Files:**
- Create: `ios/Sources/WishboneUI/WishboneSlot.swift`
- Modify: `ios/Sources/WishboneUI/WishboneScreen.swift` (`ScreenModel`: slot mode + cache drop on 404), `ios/Sources/WishboneUI/DiskCache.swift` (`remove`), `ios/Sources/WishboneUI/Environment.swift` (`EnvReader` inline geometry)
- Test: `ios/Tests/WishboneUITests/StoreTests.swift` or new `SlotTests.swift`

**Interfaces:**
- Consumes: `Navigator.forward`, `forwardsPresent`, `WishboneRoute` (Task 7).
- Produces: `public struct WishboneSlot: View { public init(_ wishbone: Wishbone, id: String, params: [String: Value] = [:], onNavigate: @escaping (WishboneRoute) -> Void) }`; `DiskCache.remove(_ key: String)`; `ScreenModel(…, slot: Bool)`.

- [ ] **Step 1: Failing tests** (`SlotTests.swift`; reuse `Support.swift` helpers — read it for how `Wishbone` and stub networking are built in `ClientTests`/`StoreTests`):
  1. `notFoundDropsTheCachedSlot`: write a cached response for `slot.x` under `DiskCache.screenKey(scope:id:params:)`, stub the network to answer 404 `SCREEN_NOT_FOUND`, `await model.load()` on a slot `ScreenModel` → `phase` is `.failed`, `cache.read(key) == nil`. The same with a non-slot model keeps the cache and stays `.ready` (current behaviour).
  2. `DiskCache.remove` removes the file and a later `read` returns nil.
  If `Support.swift` has no URL stubbing, stub via `URLProtocol` subclass registered on the `URLSession` the `API.send` uses — follow whatever `ClientTests` already does; do not add a new mechanism when one exists.
- [ ] **Step 2:** `swift test --filter SlotTests` → FAIL.
- [ ] **Step 3: Implement.**
  - `DiskCache.remove(_:)` — delete the file, ignore errors.
  - `ScreenModel` gets `let slot: Bool` (default false). In `load()`'s `catch`: `if slot, case WishboneError.server(_, "SCREEN_NOT_FOUND", _, _) = error { wishbone.cache.remove(key); cached = nil; phase = .failed(…) ; return }` (match the actual `WishboneError` case shape in `Client.swift`).
  - `EnvReader` gets `inline: Bool = false`; inline → `geometry.safeTop = 0`, `safeBottom = 0`, `screenSize = CGSize(width: g.size.width, height: 0)` (spec §2.3).
  - `WishboneSlot.swift`:

```swift
/// Inline server-driven content inside a host screen (spec 2026-09-28 §2): sized by its content, nothing at all
/// while loading, unpublished (the cache is dropped), failed without a cache, or when the root's `when` is false.
/// `navigate` and `present` go to `onNavigate` (`present` with transition "present"); `back`/`dismiss` do nothing.
public struct WishboneSlot: View {
    let wishbone: Wishbone
    let route: Route
    let onNavigate: (WishboneRoute) -> Void

    public init(_ wishbone: Wishbone, id: String, params: [String: Value] = [:], onNavigate: @escaping (WishboneRoute) -> Void) {
        self.wishbone = wishbone
        route = Route(screen: id, params: params)
        self.onNavigate = onNavigate
    }

    public var body: some View {
        SlotHost(wishbone: wishbone, route: route, onNavigate: onNavigate)
            .id("\(route.screen)|\(API.jsonString(.record(route.params)))")
    }
}

struct SlotHost: View {
    let wishbone: Wishbone
    let route: Route
    let onNavigate: (WishboneRoute) -> Void
    @State private var navigator = Navigator()
    @State private var model: ScreenModel?
    @Namespace private var namespace

    var body: some View {
        Group {
            if let store = model?.store {
                var ctx = RenderContext(store: store)
                // not isRoot: a slot's background stays inside the slot
                let _ = (ctx.namespace = namespace)
                NodeView(node: store.document.root, ctx: ctx)
            }
        }
        .modifier(EnvReader(env: wishbone.env, geometry: navigator.geometry, inline: true))
        .task {
            navigator.forward = onNavigate
            navigator.forwardsPresent = true
            navigator.exit = {}
            let m = model ?? ScreenModel(wishbone: wishbone, route: route, navigator: navigator, slot: true)
            model = m
            await m.load()
        }
        .onAppear { wishbone.clock.retain() }
        .onDisappear { wishbone.clock.release() }
    }
}
```
  (Write the `ctx` setup the way `ScreenContent.body` does — a `var ctx` built before returning the view; the `let _ =` above is only a sketch.)
- [ ] **Step 4:** `swift test` → PASS.
- [ ] **Step 5: Commit** — `iOS: WishboneSlot — инлайн-слот, пустой без контента, кэш сбрасывается при снятии`.

---

### Task 9: Android — навигация в стек хоста

**Files:**
- Modify: `android/wishbone-ui/src/main/kotlin/app/wishbone/ui/Navigator.kt`, `Screen.kt`
- Test: `android/wishbone-ui/src/test/kotlin/app/wishbone/ui/NavigatorTest.kt`

**Interfaces:**
- Produces:

```kotlin
/** A screen for the host's own back stack (spec 2026-09-28 §1). transition: "push" | "fade" | "present" (slots only). */
data class WishboneRoute(val screen: String, val params: Map<String, Value> = emptyMap(), val transition: String? = null)
// Navigator:
var forward: ((WishboneRoute) -> Unit)? = null
var forwardsPresent = false
// Screen.kt:
@Composable fun WishboneScreen(wishbone: Wishbone, id: String, params: Map<String, Value> = emptyMap(), modifier: Modifier = Modifier, onExit: () -> Unit = {}, onNavigate: ((WishboneRoute) -> Unit)? = null)
@Composable fun WishboneScreen(wishbone: Wishbone, route: WishboneRoute, modifier: Modifier = Modifier, onExit: () -> Unit = {}, onNavigate: ((WishboneRoute) -> Unit)? = null)
```

- [ ] **Step 1: Failing tests** (append to `NavigatorTest`):

```kotlin
@Test
fun hostStackReceivesPushesAndBackExits() {
    val n = nav()
    val pushed = mutableListOf<WishboneRoute>()
    var exited = false
    n.forward = { pushed += it }
    n.exit = { exited = true }
    n.navigate("raffle.tickets", mapOf("n" to Value.Num(1.0)), "fade")
    assertEquals(listOf("root"), n.ids())
    assertEquals(listOf(WishboneRoute("raffle.tickets", mapOf("n" to Value.Num(1.0)), "fade")), pushed)
    assertFalse(n.back())
    assertTrue(exited)
}

@Test
fun hostStackKeepsSheetsInTheSdk() {
    val n = nav()
    var pushed = 0
    n.forward = { pushed++ }
    n.present("raffle.rules", emptyMap())
    n.navigate("inner", emptyMap(), null) // inside the sheet: stays in the SDK
    assertEquals(listOf("root", "raffle.rules", "inner"), n.ids())
    assertEquals(0, pushed)
}

@Test
fun slotForwardsPresent() {
    val n = nav()
    val pushed = mutableListOf<WishboneRoute>()
    n.forward = { pushed += it }
    n.forwardsPresent = true
    n.present("raffle.rules", emptyMap())
    assertEquals(listOf("root"), n.ids())
    assertEquals("present", pushed.single().transition)
}
```
(Use the real `Value` number constructor from `:wishbone-core`.) Transition mapping: `null`/other → `"push"`, `"fade"` → `"fade"`.

- [ ] **Step 2:** `cd android && ./gradlew :wishbone-ui:testDebugUnitTest --tests '*NavigatorTest*'` → FAIL.
- [ ] **Step 3: Implement.** `navigate`: `forward?.takeIf { sheetStart == entries.size }?.let { it(WishboneRoute(screen, params, if (transition == "fade") "fade" else "push")); return }`; `present`: `if (forwardsPresent) forward?.let { it(WishboneRoute(screen, params, "present")); return }`. `WishboneRoot` gets `onNavigate`, kept fresh with `rememberUpdatedState` and assigned `navigator.forward = onNavigate?.let { f -> { r: WishboneRoute -> nav(r) } }` where `nav` is the updated state (mirror how `exit` is done). The route overload calls `WishboneRoot(wishbone, route.screen, route.params, …)`. KDoc on `WishboneScreen` explains `onNavigate` (spec §1.2).
- [ ] **Step 4:** `./gradlew :wishbone-core:test :wishbone-ui:testDebugUnitTest` → PASS.
- [ ] **Step 5: Commit** — `Android: навигация в стек хоста (onNavigate, WishboneRoute)`.

---

### Task 10: Android — `WishboneSlot`

**Files:**
- Create: `android/wishbone-ui/src/main/kotlin/app/wishbone/ui/Slot.kt`
- Modify: `ScreenLoader.kt` / `ScreenHolder.kt` (slot flag, cache drop on 404), `DiskCache.kt` (`remove`), wherever env geometry (`screenWidth`, `safeTop`) is computed for `ScreenContent` (read `Screen.kt` bottom part)
- Test: `android/wishbone-ui/src/test/kotlin/app/wishbone/ui/ScreenLoaderTest.kt`, `DiskCacheTest.kt`

**Interfaces:**
- Consumes: `Navigator.forward`, `forwardsPresent`, `WishboneRoute` (Task 9).
- Produces: `@Composable fun WishboneSlot(wishbone: Wishbone, id: String, params: Map<String, Value> = emptyMap(), onNavigate: (WishboneRoute) -> Unit, modifier: Modifier = Modifier)`; `DiskCache.remove(key)`.

- [ ] **Step 1: Failing tests:** `ScreenLoaderTest` — a slot loader whose `fetch` throws the client's 404 `SCREEN_NOT_FOUND` error with a cached screen present: `load` returns false, the cache entry is gone, `show` was called with the cached screen first (current SWR behaviour) — then the slot hides because the holder state becomes `Failed` (assert on the loader's contract the way existing `ScreenLoaderTest` cases do). Non-slot: cache kept. `DiskCacheTest`: `remove` deletes. Read `ScreenLoaderTest.kt` first and follow its structure and fakes.
- [ ] **Step 2:** run the tests → FAIL.
- [ ] **Step 3: Implement.**
  - `DiskCache.remove(key)`; `ScreenLoader(…, slot: Boolean = false)`: on a `SCREEN_NOT_FOUND` failure and `slot` → remove the cache entry, return false even if content was shown (the holder then sets `Failed`, which a slot renders as nothing).
  - `ScreenHolder(…, slot: Boolean = false)` passes it through.
  - `Slot.kt`:

```kotlin
/**
 * Inline server-driven content inside a host screen (spec 2026-09-28 §2): sized by its content, nothing at all while
 * loading, unpublished (the cache is dropped), failed without a cache, or when the root's `when` is false. `navigate`
 * and `present` go to [onNavigate] (`present` with transition "present"); `back`/`dismiss` do nothing.
 */
@Composable
fun WishboneSlot(wishbone: Wishbone, id: String, params: Map<String, Value> = emptyMap(), onNavigate: (WishboneRoute) -> Unit, modifier: Modifier = Modifier) {
    val nav by rememberUpdatedState(onNavigate)
    val navigator = remember(wishbone, id, params) {
        Navigator(id, params).apply {
            forward = { nav(it) }
            forwardsPresent = true
            exit = {}
        }
    }
    val holder = remember(navigator) { ScreenHolder(wishbone, navigator, navigator.entries.first(), slot = true).also { it.load() } }
    DisposableEffect(holder) { onDispose { holder.dispose() } }
    // env: like WishboneRoot (brand, scheme, clock) but safeTop/safeBottom = 0, screenWidth = slot width, screenHeight = 0
    …
    val store = holder.store ?: return
    CompositionLocalProvider(LocalWishbone provides wishbone) {
        Box(modifier) { NodeView(store.doc.root, /* root ctx as ScreenContent builds it, without the root background extension */) }
    }
}
```
  Reuse the env/clock setup from `WishboneRoot` by extracting the smallest shared helper (e.g. a `@Composable rememberScreenEnv(wishbone)` plus the ticking-clock `LaunchedEffect`) instead of copying it; measure the slot width with `Modifier.onSizeChanged` or `BoxWithConstraints` and feed it into the env. The slot never fills max size.
- [ ] **Step 4:** `./gradlew :wishbone-core:test :wishbone-ui:testDebugUnitTest` → PASS.
- [ ] **Step 5: Commit** — `Android: WishboneSlot — инлайн-слот, пустой без контента, кэш сбрасывается при снятии`.

---

### Task 11: Экраны розыгрыша и слот (контракт)

**Files:**
- Create: `contract/screens/raffle.main.json`, `raffle.tickets.json`, `raffle.invites.json`, `raffle.rules.json`, `slot.feed.top.json`
- Create: `contract/assets/raffle/*.png` (artwork copied from Stratoapp)
- Test: existing seed/golden tests (`go test ./...` compiles every `contract/screens/*.json`); a local server against stage

**Interfaces:**
- Consumes: ops of Task 5 (types exactly as in the table there), templates in `contract/templates/` (read their `param`/`event` declarations before use: `SegmentedTabs`, `Card`, `Button`, `ProgressBar`, `Toggle`, `ListRow`, `Badge`, `EmptyState`, `Loader`, `Divider`, `IconButton`), `host("share", {text})`.
- Produces: screen ids `raffle.main`, `raffle.tickets` (params `tickets: number`, `place: number`), `raffle.invites`, `raffle.rules` (params `share: string`), `slot.feed.top`.

The source format is spec V1 §3.1; look at `contract/screens/showcase.promo.json` and `showcase.form.json` for idioms (headers, `data`, `refresh`, `await call`, `repeat`, countdown from `env.now`, `plural`). Colors/typography only through tokens (`contract/tokens/korpu.json`); spacing via `@space.*`; icons from `contract/icons.json`. Every node has a unique `id`. Strings — from the Stratoapp catalogue (spec §3.3; exact texts below).

- [ ] **Step 1: Artwork.** Copy from Stratoapp (`find ~/projects/Stratoapp/Modules -name '*.imageset' -path '*aff*'` — `raffle_super_gift`, `raffle_coin`, laurels, cup, `raffle_car` fallback; take the `@3x` PNG of each) into `contract/assets/raffle/` with short names (`gift.png`, `coin.png`, `laurel-left.png`, `laurel-right.png`, `cup.png`, `car.png`). Keep each ≤ 300 KB (downscale with `sips -Z 600` if larger). Referenced as `/assets/raffle/<name>.png`.
- [ ] **Step 2: `raffle.rules`** (present): `param share: string = ""`; `data o = raffle.overview()`; header «Как получить билеты» (`@type.title2` or the closest title token); `scroll` `repeat` over `o.rules ?? []`: row = `image` icon 40×40 (`when: item.icon != null`), name, «+N билетов» (`plural`), description; bottom `Button` «Пригласить друзей» → `host("share", {text: share != "" ? share : (o.share ?? "")})`, `when` the text is non-empty.
- [ ] **Step 3: `raffle.tickets`** (push): `param tickets: number = 0`, `param place: number = 0`; `data page = raffle.tickets({offset: 0, limit: 20})`; `state extra: list<{id: string, createdAt: number}> = []`, `state loading = false`, `state done = false`; header with back (`IconButton` chevron → `back()`) and «Мои билеты»; summary card: «N билетов» (`plural(tickets, {one: "# билет", few: "# билета", many: "# билетов", other: "# билета"})`) + «N место» or «Места пока нет» when `place == 0`; card «Id билетов»: `repeat` over `(page.items ?? []) + extra` (use `append`/list concat available in stdlib — check `stdlib.json`; else keep one state list seeded on `appear`) rows `#i id`; footer node with `on.appear` → `if !loading && !done { loading = true; let r = await call raffle.tickets({offset: count, limit: 20}); … append r.value.items; done = r.value.items.count < 20; loading = false }`; `page == null` → «Не удалось загрузить билеты» + «Повторить» (`refresh(page)`).
- [ ] **Step 4: `raffle.invites`** (push): `data inv = raffle.invites()`; header back + «Мои приглашения»; card: «Мои приглашения» + `"${inv.total} пользователей"`; rows «name — N билетов»; empty «Начислений пока нет»; `inv == null` → «Не удалось загрузить историю начислений» + «Повторить».
- [ ] **Step 5: `raffle.main`**:
  - `data o = raffle.overview()`, `data board = raffle.leaderboard({offset: 0, limit: 20})`; `state tab = "tasks"`, `state hiddenOverride: bool? = null`, `state claiming = false`, `state rows: list<…> = []` (leaderboard rows beyond the first page), `state loadingMore = false`; `let hidden = hiddenOverride ?? o.hidden ?? false`.
  - Header: `IconButton` back → `back()`; title «Розыгрыш»; `IconButton` info → `present("raffle.rules", {share: o.share ?? ""})`.
  - `o == null` → `EmptyState`-like block «Не удалось загрузить розыгрыш» + `Button` «Повторить» → `refresh(o)`; rest hidden.
  - `SegmentedTabs` «Задания / Призы / Таблица» bound to `tab` (values `tasks`, `prizes`, `table`).
  - Progress card (when `tab != "table"`): «Мои билеты», `plural(o.tickets …)`, «N место»/«Места пока нет», «Подробнее» → `navigate("raffle.tickets", {tickets: o.tickets, place: o.place ?? 0})`.
  - Main prize card: laurels + «Главный приз» + `o.mainPrize.title`, «Итоги розыгрыша проведем ${formatDate(o.endsAt, "date")}» when `endsAt != null`, `image` `o.mainPrize.image ?? "/assets/raffle/car.png"` (`fit: "fit"`).
  - Tab «Задания»:
    - Daily card «Заходи каждый день и получай ACoins»: horizontal `scroll` `repeat` over `o.daily.days`, cell «День N» + amount + state style (claimed — `@color.success`-tinted check, available — accent border, waiting — muted); `Button` «Забрать ${o.daily.claimAmount}» `when: o.daily.claimAmount != null`, loading while `claiming`: `claiming = true; let r = await call raffle.claimDaily(); if r.ok { haptic("success"); refresh(o) } else { host("toast", {message: "Не удалось получить ежедневную награду"}) }; claiming = false`.
    - Card «Как выиграть главный приз?» / «Заработай больше всех билетов!» → `present("raffle.rules", {share: o.share ?? ""})`.
    - Tasks card: gift art, «Получай билеты за задания», «Осталось: Xдн:Yч:Zм» from `o.endsAt - env.now` (clamp at 0; days = `floor(ms / 86400000)`, hours, minutes), rows per `o.tasks`: icon `image` (fallback `@icon.star`), name, «+N билетов», description, progress rows «label value» (with a ticket icon when `ticket`), «Доступно через HH:MM:SS» from `cooldownUntil - env.now` (`formatDuration`) while positive; empty «Заданий пока нет».
    - Invitations card «Мои приглашения» → `navigate("raffle.invites")`.
    - Referral card «Получи 30 билетов за каждого друга» / «Награда начислится, когда друг зарегистрируется» / «Количество билетов не ограничено.» + `Button` «Пригласить друзей» → `host("share", {text: o.share ?? ""})` (`when: o.share != null`).
    - Settings card «Настройки розыгрыша»: `Toggle` «Показывать ФИО в таблице» with value `!hidden`; on change: `let was = hidden; hiddenOverride = !was; let r = await call raffle.toggleHidden(); if r.ok { hiddenOverride = r.value.hidden } else { hiddenOverride = was; host("toast", {message: "Не удалось сохранить настройку"}) }`.
  - Tab «Призы»: «Призы» header; `grid` columns 2 over `o.prizes`: card with `image` (or coin art fallback), rank badge `#N`, `Badge` «от N», title (2 lines max).
  - Tab «Таблица»: cup art + «Таблица участников» + «Условия начисления» link → rules; rows over `(board.items ?? []) + rows`: «#position name — N» (`isMe` → accent background); footer `on.appear` loads the next page with `await call raffle.leaderboard({offset: count, limit: 20})` (guard with `loadingMore` and stop when `count >= board.total`).
- [ ] **Step 6: `slot.feed.top`**: `data promo = raffle.promo() required`; root `when: promo.active && env.now < (promo.endsAt ?? 0)`, `width: "fill"`, padding `@space.lg` horizontally (feed gutter), `on.tap` → `navigate("raffle.main")`; card (accent gradient or `@color.bg2`, corner 20): gift art, «Розыгрыш» title + «Выполняй задания и выигрывай призы», place badge «#N место» (`when: promo.place != null`), ticking countdown «Осталось: Xд Yч Zм» from `promo.endsAt - env.now`, «Задания ${promo.tasksDone}/${promo.tasksTotal}» (`when: promo.tasksTotal > 0`). `a11y` label «Розыгрыш», role button.
- [ ] **Step 7: Compile.** `cd backend && go test ./...` (the seed/golden tests compile every screen; fix diagnostics until green). For a quick loop: `go run ./cmd/wbc` if it supports compiling a file, else `POST /admin/api/compile` against a local server.
- [ ] **Step 8: Run against stage.** Local server: `WISHBONE_GRAPHQL_URL=https://stage.korputeam.ru/graphql WISHBONE_GRAPHQL_LOGIN=79956791664:133767 go run ./cmd/server` (fresh local DB so the new screens seed: `dropdb wishbone && createdb wishbone`, or unpublish/recreate via admin). Open each screen in the iOS Playground on a simulator (anonymous → test account), check data, tab switching, pushes, rules sheet, claim/toggle return `UNAUTHENTICATED` toast (anonymous writes) — writes get verified in Stratoapp (Task 14). Screenshot every screen/tab; fix layout issues.
- [ ] **Step 9: Commit** — `Шоукейс: экраны розыгрыша на stage GraphQL и баннер-слот ленты` (screens + assets).

---

### Task 12: Playgrounds — стек хоста и лента со слотом

**Files:**
- Modify: `ios/Playground/Sources/HomeView.swift` (+ `PlaygroundModel` if routing lives there), create `ios/Playground/Sources/FeedDemoView.swift`
- Modify: `android/playground/src/main/kotlin/app/wishbone/playground/Playground.kt`, create `FeedDemo.kt` in the same package
- Test: manual on simulator/emulator + screenshots

**Interfaces:**
- Consumes: `WishboneScreen(…, route:, onExit:, onNavigate:)`, `WishboneSlot` (Tasks 7–10).

- [ ] **Step 1: iOS.** `HomeView`: `NavigationStack(path: $path)` with `@State private var path: [WishboneRoute] = []` and `.navigationDestination(for: WishboneRoute.self) { r in WishboneScreen(model.wishbone, route: r, onExit: { path.removeLast() }, onNavigate: { path.append($0) }) }`; the showcase list and the custom-id field push `WishboneRoute(screen: id)` (add `raffle.main` and a «Feed demo (slot.feed.top)» row); live preview keeps its current full-screen presentation. `FeedDemoView`: native `ScrollView` + `LazyVStack` of 6 fake post cards (plain SwiftUI: gray rect 200 pt + two text lines), with `WishboneSlot(model.wishbone, id: "slot.feed.top") { path.append($0) }` as the first row; a route with `transition == "present"` is appended the same way (the playground has no modals of its own). The playground host's `action("share")` logs and shows the existing host-action toast.
- [ ] **Step 2: Android.** Replace the single `route` with `val stack = remember { mutableStateListOf(Route(start ?: "home")) }`; `BackHandler(enabled = stack.size > 1) { stack.removeLast() }`; `"screen"` → `WishboneScreen(wb, WishboneRoute(route.arg), onExit = { stack.removeLast() }, onNavigate = { stack += Route("route", it) })` — store the `WishboneRoute` in the playground's `Route` (add a nullable field); add `"feed"` → `FeedDemo(wb) { stack += Route("route", it) }` (`LazyColumn`, first item `WishboneSlot(wb, "slot.feed.top", onNavigate = …)`, then 6 fake cards); Home lists `raffle.main` and «Feed demo».
- [ ] **Step 3: Verify both** against the local server with stage upstream (Task 11 Step 8): feed demo shows the banner; tap pushes `raffle.main` onto the playground stack (native back returns to the feed; iOS swipe-back works); inside, «Подробнее» pushes `raffle.tickets` onto the same stack; «i» opens the rules sheet (SDK). Unpublish `slot.feed.top` in the admin → reopen feed demo → no banner, no gap; publish again → banner back. Screenshots: feed with banner, raffle.main (3 tabs), tickets, invites, rules — iOS and Android.
- [ ] **Step 4: Commit** — `Playground: пуш в стек плейграунда и лента со слотом (iOS, Android)`.

---

### Task 13: Выкатка тест-сервера

**Files:**
- Modify: `deploy/Dockerfile`, `deploy/docker-compose.yml`, `deploy/push.sh`

- [ ] **Step 1: CA bundle.** `push.sh`: after copying the Dockerfile — `cp /etc/ssl/cert.pem "$STAGE/ca-certificates.crt"` (macOS system bundle; fail with a clear message if missing). `Dockerfile`: `COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt` (Go's default Linux root path).
- [ ] **Step 2: Env.** `docker-compose.yml` server environment: `WISHBONE_GRAPHQL_URL: ${WISHBONE_GRAPHQL_URL:-}` and `WISHBONE_GRAPHQL_LOGIN: ${WISHBONE_GRAPHQL_LOGIN:-}`. `push.sh` remote script: if `.env` lacks `WISHBONE_GRAPHQL_URL`, append `WISHBONE_GRAPHQL_URL=https://stage.korputeam.ru/graphql` and `WISHBONE_GRAPHQL_LOGIN=79956791664:133767` (test-only stage creds, spec §3.1) — keep the umask 077 file mode.
- [ ] **Step 3: Seed on the live DB.** New screens seed automatically on start (missing ids only). Nothing else changes on the VPS: never touch Hermes (`/opt/hermes`, its nginx site, ports 8080/8081).
- [ ] **Step 4: Deploy + check.** `deploy/push.sh`; then `curl -s https://wishbone.217-198-9-139.sslip.io/v1/screens/slot.feed.top` shows `data.promo.ok: true` and `active: true` (resolve the name from the VPS if this Mac's DNS is proxied — see memory), `raffle.main` data ok; admin shows the 5 new screens and the unpublish button.
- [ ] **Step 5: Commit** — `Выкатка: CA-сертификаты в образе и апстрим GraphQL stage`.

---

### Task 14: Stratoapp — слот в ленте, оверлей, шаринг

Repo `~/projects/Stratoapp`, branch `dev` has uncommitted changes of the owner in `App/Sources/AppEntry.swift` and `App/Sources/BrandFeatures*.swift`: **do not commit them**. Work in a new local branch `feat/wishbone-raffle` from `dev` via `git worktree add ../Stratoapp-wishbone-raffle -b feat/wishbone-raffle dev` so the owner's working tree stays untouched. Read `Stratoapp/CLAUDE.md` and `docs/Architecture/*.md` rules that apply (module boundaries, Tuist, tests) before editing.

**Files (paths relative to the Stratoapp worktree):**
- Modify: `Vendor/WishboneSDK/**` via `Scripts/sync-wishbone.sh`
- Modify: `Modules/WishboneAdapter/Sources/WishboneAdapter.swift` (`slot(id:onNavigate:)`), `Modules/WishboneAdapter/Sources/WishboneAppHost.swift` (`share`)
- Modify: `Modules/Feed/Domain/Sources/Render/FeedRenderItem.swift`, `FeedRenderSnapshotBuilder.swift`, `FeedState` (slot flag), `Modules/Feed/Feature/Sources/Assembly/FeedFeatureModule.swift`, `FeedStore`/state init plumbing, `Modules/Feed/Feature/Sources/UI/FeedTextureTimelineView.swift` (row)
- Modify: `App/Sources/BrandFeatures.swift` (pass the slot factory) — only the lines needed, in the worktree
- Modify: `Modules/AppCore/Sources/UI/RootWishboneSheet.swift` / `RootView.swift` / `RootScreen.swift` (overlay instead of sheet)
- Test: the Feed domain tests for `FeedRenderSnapshotBuilder` (find the existing test file), WishboneAdapter tests

- [ ] **Step 1: Sync SDK.** `Scripts/sync-wishbone.sh` (from `../wishbone` at the commit after Task 12); build the adapter module.
- [ ] **Step 2: Feed item (TDD).** Test in the existing snapshot-builder tests: with `state.wishboneSlotID = "slot.feed.top"` and scope `.main(false)` the snapshot contains `.wishboneSlot` right after `.rafflePromo` (or after stories when no promo); nil → absent; other scopes → absent. Implement `case wishboneSlot(FeedWishboneSlotRenderModel)` (`id: "feed.wishbone.<slotID>"`, `slotID`), `postID` nil, the builder rule, and `FeedState.wishboneSlotID: String?` set at store init from a new `FeedFeatureModule(wishboneSlot: (@MainActor (String) -> AnyView)? = nil)` (slot id constant `"slot.feed.top"` lives in the App wiring). Row: `AnyView(wishboneSlot?(model.slotID) ?? AnyView(EmptyView()))`.
- [ ] **Step 3: Adapter.** `WishboneAdapter.slot(_ id: String) -> AnyView` → `WishboneSlot(wishbone, id: id) { route in host.routeFirstPartyLink(<first-party URL for bdui/route.screen>) }` — build the URL in the format `RootExternalRouteParser` accepts for the `bdui` section (read the parser; e.g. `korpu://bdui/<id>`), params dropped (spec §4 ceiling, add a `ponytail:` comment). `WishboneAppHost.action`: `"share"` with `args["text"] as? String` → present `UIActivityViewController` from the key window's top-most view controller (follow any existing share helper in Stratoapp — `FeedSharing`/`ShareLink` usage — before writing a new one).
- [ ] **Step 4: Wire.** `BrandFeatures.swift`: pass `wishboneSlot: adapter.map { a in { id in a.slot(id) } }` only when the adapter is available (debug/TF); in the worktree the owner's uncommitted edits are absent — keep the diff minimal.
- [ ] **Step 5: Overlay.** Present `state.presentedWishboneScreen` with `ManualSlidingOverlay(edge: .trailing, onInteractiveDismiss: { send(.wishboneDismissed) })` like the raffle pane (own z-index constant next to `RootOverlayZIndex.raffle`), content = the existing `RootWishboneScreenFactory(screenID) { send(.wishboneDismissed) }` + `.environment(\.routeFirstPartyLink, …)`; remove the `.sheet` path of `wishboneSheet` but keep `openWishboneDiagnostics`. The bdui deeplink and the slot both land here.
- [ ] **Step 6: Build and run.** Tuist generate + build the debug scheme for a simulator (the project's documented commands); log in on the stage contour with the test account (dev menu contour = stage; phone `79956791664`, Telegram code `133767`); the feed shows the banner under the native raffle promo; tap → Wishbone `raffle.main` slides in; «Подробнее» pushes inside the SDK stack; «Забрать» / toggle work with the real token (then restore the toggle); «Пригласить друзей» opens the share sheet; swipe/back closes the overlay. Screenshots.
- [ ] **Step 7: Tests** — the module tests touched (Feed domain, WishboneAdapter) green.
- [ ] **Step 8: Commit** in the worktree branch (no push) — `Wishbone: баннер-слот розыгрыша в ленте, экраны Wishbone открываются оверлеем, шаринг из хоста`.

---

### Task 15: Доки и память

**Files:**
- Modify: `docs/superpowers/decisions/renderers.md` (host stack + slot rulings), `docs/superpowers/decisions/server.md` (unpublish, timeoutMs, upstream), `docs/README.md` (status line), spec §8.3 of V1 (placement slots → «см. спеку 2026-09-28»)
- Memory: `wishbone-v1-status.md` (new milestone), `wishbone-test-server.md` (upstream env)

- [ ] **Step 1:** decisions — add sections «Host stack & slots (2026-09-28)» (renderers) and «Upstream, unpublish, op timeouts (2026-09-28)» (server) with the binding rules actually implemented (including every ruling taken during Tasks 1–14).
- [ ] **Step 2:** README status: V1 + Raffles on stage, slots, host stack; Stratoapp branch name.
- [ ] **Step 3:** memory files updated; `MEMORY.md` index lines adjusted.
- [ ] **Step 4: Commit** — `Доки: решения по слотам, стеку хоста и апстриму`.
