# Perf audit fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the allocation, copy and redundant-work costs found by the 2026-09-29 perf audit on all three platforms, without changing what any screen renders.

**Architecture:** Five independent code tasks (Go backend; Android core → Android UI; iOS core → iOS UI) plus a docs task. Every change is local to the function the audit named; the only semantic change is the slot memo rule (Task 3/5, ruling R1). Runtime output (strings, numbers, layout, animation) must stay byte/pixel identical — conformance and existing tests are the guard.

**Tech Stack:** Go 1.x + pgx v5 (backend); Kotlin + Jetpack Compose, OkHttp (Android, minSdk 33); Swift 6 + SwiftUI, iOS 18 (iOS).

**Spec:** the audit findings below (per task, with evidence) argue from `docs/superpowers/specs/2026-09-27-wishbone-v1-design.md` (runtime semantics), `docs/superpowers/specs/2026-09-28-raffles-slots-host-nav-design.md` §2.3 (slot semantics) and `docs/superpowers/decisions/*.md`. Audit reference numbers were measured on HEAD 236825c (Go: pgx benchmarks on a scratch DB; iOS core: release-mode bench package at `/private/tmp/claude-501/-Users-ivan-projects-wishbone/02100b6b-da42-4c59-985e-9ed44d06643c/scratchpad/perf/ios-core/bench/`).

## Global Constraints

- **Output identical.** No change may alter a rendered string, number, colour, layout, animation curve or timing, response body (except the ETag-hit `document: null` in Task 1 item 3, which clients already handle — verify), or DSL semantics. Conformance tests (`contract/conformance` via each platform's ConformanceTest) must pass unchanged.
- **Parity iOS ↔ Android is mandatory** for every runtime rule touched (number formatting, slot memo, disk-cache cap, timelines). Constants: slot memo freshness **30 s**; disk cache cap **64 entries**.
- iOS: Swift 6 strict concurrency, iOS 18 minimum, no new dependencies. Android: minSdk 33, no new dependencies. Go: no new dependencies.
- Russian commit messages, one commit per logical group, trailer `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.
- Stay inside your task's directories. Never push, never touch Stratoapp, the test server or the VPS.
- Local servers (if needed): Go task port 8085, Android 8086, iOS 8087. Stage upstream env: `WISHBONE_GRAPHQL_URL=https://stage.korputeam.ru/graphql WISHBONE_GRAPHQL_LOGIN=79956791664:133767`. Never log tokens, Authorization headers or response bodies.
- `ponytail:` comment for any deliberate shortcut with a known ceiling.

## Rulings (controller)

- **R1 — slot memo.** Each `Wishbone` instance keeps an in-memory memo per slot cache key (screen id + params): the decoded document, data, etag and the instant of the last *successful network* load. On slot appearance: render from the memo synchronously if present (no disk read/parse), else from disk as today; revalidate over the network only if there is no memo entry or its last network load is ≥ 30 s old. `SCREEN_NOT_FOUND` drops the memo entry together with the disk cache. Spec §2.3 «Загрузка — при появлении вьюхи … Принудительное обновление — сменой identity вьюхи у хоста» is replaced (Task 6): no host relies on identity refresh (Stratoapp's `wishboneSlotID` is the screen id). Plain screens keep revalidating on every open.
- **R2 — disk cache cap.** Both SDKs cap the disk cache directory at 64 entries; after a write, delete the oldest by modification time beyond 64. (Cache keys include params, so `raffle.tickets {tickets, place}` otherwise grows without bound.)
- **R3 — skipped with reason.** iOS O(n²) `reduce`/`append` paging (0.4 ms over a whole 330-row session, needs a DSL list-concat), the 10 000-step budget vs. a 330-row lambda (no shipped screen does it), admin preview data reuse (preview must show fresh data; admin-only, debounced), compiler template JSON round trip (publish-time, linear, ≤ 20 ms worst case).

## Review Focus

1. Numbers at the fast-path edges — negative whole numbers, `-0`, `1e15 - 1`, `1e15`, `≥ 9.2e18`, NaN/±Infinity — must print exactly as before (Tasks 2, 4).
2. Theme switch light ↔ dark after token memoisation must re-resolve every `@color`/`@type`/`@icon`/`@motion` (Tasks 3, 5).
3. Slot memo: `SCREEN_NOT_FOUND` clears memo + disk; different params never share an entry; a memo hit still honours root `when` (Tasks 3, 5).
4. A `repeat: -1` timeline whose target appears later (a `when` flips, a lazy row scrolls in) must start animating; finite timelines and `await play` unchanged (Task 3).
5. Repeat items whose value changes at the same key (leaderboard refresh), reorder, or shrink must re-render correctly after the Locals cache (Task 5).
6. Go ETag-aware read: `If-None-Match` with several ETags, a stale ETag, or none → document present; matching ETag → `document: null` handled by both SDKs as «keep cached» (Task 1).

---

### Task 1: Go backend

**Files:** `backend/internal/store/store.go`, `backend/internal/api/{data.go,public.go,server.go,lifecycle.go}`, `backend/internal/ops/gql.go`, `backend/internal/preview/hub.go`, `backend/cmd/server/main.go`, tests beside them.

**Findings → changes** (audit numbers are raffle.main):

1. **`scanRev` re-validates JSON** (`store.go:206-208`). pgx v5 decodes into `*json.RawMessage` with `json.Unmarshal`; into `*[]byte` it copies. Change the three JSON targets to `(*[]byte)(&r.Source)`, `(*[]byte)(&r.Document)`, `(*[]byte)(&r.Meta)`. Audit: 738 → 174 µs per `Live`/`Revision`. Works for `json` and `jsonb`.
2. **`meta()` loads the whole revision** (`api/data.go:79-90`). Add `func (s *Store) Meta(ctx context.Context, id string, rev int) (json.RawMessage, error)` = `SELECT meta FROM revisions WHERE screen_id=$1 AND rev=$2` (same `ErrNoRevision` on no rows) and use it in `meta()`. Audit: 740 → 21 µs per `/v1/data` and `/v1/call`.
3. **`GET /v1/screens` reads unused columns** (`store.go:217-220`, `public.go:25,48-51`). `Live` stops selecting `r.source` (its other reader, `Recompile` in `lifecycle.go:86-90`, switches to `Revision(ctx, id, *sc.PublishedRev)`). Pass the request's `If-None-Match` ETags into the query and select `CASE WHEN r.etag = ANY($2) THEN NULL ELSE r.document END`. First check what `public.go` returns today on an ETag match and that both SDKs keep their cached document when `document` is null — if either SDK doesn't, keep the document column and only drop `source` (report which).
4. **Upstream connection churn** (`ops/gql.go:70-75`, `cmd/server/main.go:51`). One package-level `*http.Client` used whenever `g.HTTP == nil`: `http.DefaultTransport.(*http.Transport).Clone()` with `MaxIdleConnsPerHost = 32`, `Timeout: 10 * time.Second`. On non-200 responses drain the body (`io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))`) before closing.
5. **Response re-compacts the stored document** (`api/server.go:221-229`, `public.go:52-57`). Encode `{etag, revision, data}` and splice `,"document":` + stored bytes (or `null`) before the closing brace. `TestServedDocumentMatchesETag` must still pass. Audit: 167 → 5 µs.
6. **Upstream response parsed twice** (`ops/gql.go:104-135`). Decode in one pass: the wire struct's `Data` field points at `out`; still treat `"data": null` / missing data as failure.
7. **Hub `rebase` copies ×3 under the global lock** (`preview/hub.go:119-121,327-329`). One `bytes.ReplaceAll` (or equivalent single copy).
8. **`http.Server` without `IdleTimeout`** (`cmd/server/main.go:99`). Add `IdleTimeout: 120 * time.Second`.

- [ ] **Step 1:** Before changing code, write temporary benchmarks `zz_perf_*_test.go` for items 1, 2, 5 (Live/Revision scan, meta, response encode) and record baseline `go test -run '^$' -bench . -benchmem -count 5` numbers to the report. Delete them before the final commit.
- [ ] **Step 2:** Tests first where behaviour is observable: store test for `Meta` (existing rev → bytes equal to `Revision(...).Meta`; unknown rev → `ErrNoRevision`); store/API test for the ETag-aware `Live` (matching ETag → nil document; non-matching / several / none → document); response test that the served document bytes equal the stored bytes and the JSON is valid with `document: null`; gql test that a non-200 upstream response still yields `UPSTREAM_FAILED` and a `"data": null` body yields the same failure as before. Run, see the new ones fail.
- [ ] **Step 3:** Implement items 1–8. Run `cd backend && go test ./... && go vet ./...` (Postgres is running locally; use the repo's existing test DB setup).
- [ ] **Step 4:** Re-run the benchmarks, record after-numbers next to the baseline, delete `zz_perf_*`, confirm `git status --short backend` shows only intended changes.
- [ ] **Step 5:** Commit per group: `Сервер: чтение ревизий без повторной валидации JSON и лишних колонок`, `Сервер: ответ экрана без перекомпакции документа`, `Сервер: пул соединений к upstream GraphQL, один проход по ответу`, `Сервер: IdleTimeout, одна копия в rebase превью`.

### Task 2: Android core (`android/wishbone-core`)

**Files:** `src/main/kotlin/app/wishbone/core/{Format.kt,TimelineSampler.kt,Interpreter.kt,Stdlib.kt}`, tests in `src/test/kotlin/app/wishbone/core/`.

**Interfaces — Produces:** `TimelineSampler.sampleTrack(timeline: Timeline, track: Track, ms: Double, index: Int): Double` made public (Task 3 calls it). Keep `sample()` for conformance tests.

1. **Whole-number fast path** (`Format.kt:31,33,69`): where `BigDecimal(r).toPlainString()` formats a whole `r`, use `if (abs(r) < 9.0e18) r.toLong().toString() else BigDecimal(r).toPlainString()` (keep sign handling identical).
2. **Date/duration formatting** (`Format.kt:101-122`): `formatDuration` → a `pad2` helper instead of `String.format`; `formatDate` → cached `DateTimeFormatter`s (private vals per pattern) and cached `ZoneId` lookups.
3. **Per-frame timeline sampling** (`TimelineSampler.kt:14-19`): expose `sampleTrack` (see Interfaces).
4. **Stdlib call overhead** (`Interpreter.kt:222-232`, `Stdlib.kt:80,99,321`): no new context object / `Stdlib` instance per call — e.g. the existing per-eval `Budget` implements `StdlibContext` and `Stdlib` becomes a stateless `object` with `call(ctx, name, args)`; `interpolate` reads `num(list[i])` directly instead of building boxed lists.
5. **Parameter binding** (`Interpreter.kt:43,107,228,239`; `Stdlib.kt:158,207`): replace `params.zip(args)` with an index loop helper binding into `Locals` (same truncation as `zip`); `map`/`filter`/`find` stop building a `listOf(...)` per element.
6. **String functions** (`Stdlib.kt:131-147,267-285,301-310`): `trim`/`substring`/`num` scan with `codePointAt`/`codePointBefore`/`offsetByCodePoints` and return `substring(i, j)` (no boxed code-point lists); ASCII fast path in `mapCase`.
7. **Error path** (`Interpreter.kt:6-9`, `Stdlib.kt:318-323`): the eval exception skips the stack trace (`Exception(msg, null, false, false)` or the Kotlin equivalent for the existing class); cap the `Value` text embedded in the message at 200 chars.

- [ ] **Step 1:** Tests first: `FormatTest` cases for whole numbers `0, -0.0, 1, -1, 42, 999999999999999 (1e15-1), 1e15, -1e15, 9.2e18, 1e19, NaN, ±Infinity` through `str()`/`formatNumber` asserting the exact current output (run them on the unchanged code first — they must pass before and after); `formatDate` with two different time zones called alternately returns the same as before; `TimelineSamplerTest` asserting `sampleTrack` equals the corresponding `sample()` entry for staggered and repeat cases; Stdlib tests for `trim`/`substring`/`upper`/`lower` on Cyrillic, emoji (surrogate pairs) and ASCII.
- [ ] **Step 2:** Implement 1–7. Run `cd android && ./gradlew :wishbone-core:test`.
- [ ] **Step 3:** Commit per group: `Android core: целые числа без BigDecimal, кэш форматтеров дат`, `Android core: семплер трека без аллокаций на кадр`, `Android core: вызовы stdlib и лямбд без лишних аллокаций`, `Android core: строковые функции без боксинга, ошибки без стектрейса`.

### Task 3: Android UI (`android/wishbone-ui`)

**Files:** `src/main/kotlin/app/wishbone/ui/{Timelines.kt,Screen.kt,Store.kt,Slot.kt,ScreenLoader.kt,ScreenHolder.kt,Client.kt,DiskCache.kt,Animated.kt,Layouts.kt,NodeView.kt,Modifiers.kt,RenderContext.kt}`, tests in `src/test/kotlin/app/wishbone/ui/`.

**Interfaces — Consumes:** `TimelineSampler.sampleTrack(timeline, track, ms, index): Double` (Task 2).

1. **Frame loop never stops** (`Timelines.kt:73-107`, `Screen.kt:323-328`): an infinite (`repeat == -1`) active timeline counts in `running()` only while at least one of its targets has a live registered instance (`live` map); `register()` bumps `version` when a target gets its first live instance so the `snapshotFlow` restarts the loop. Finite timelines unchanged.
2. **Per-frame sampling** (`Timelines.kt:108-116`): `transform` calls `TimelineSampler.sampleTrack(t.timeline, t.timeline.tracks[0], ms, index)` directly (tracks are already single-track copies); remove the `ponytail:` note.
3. **Token records per read** (`Store.kt:236-241`): memoise token values per `(group, name)`; the memo is cleared whenever `store.theme` changes.
4. **Computed re-evaluated on every read** (`Store.kt:232`): one `derivedStateOf` per computed name, created lazily, so reads share one evaluation per state change.
5. **`Store.data` single map** (`Store.kt:160`, `ScreenHolder.kt:57,118`): per-source state cells written only when the value changed (like the env cells).
6. **Slot memo — ruling R1** (`Slot.kt:65-73`, `ScreenLoader.kt:27-48`, `Client.kt:187-189`): per-`Wishbone` memo (key = the slot's cache key) of decoded document + data + etag + last network load instant (monotonic clock); appearance renders from memo synchronously, revalidates only if no entry or ≥ 30 s; `SCREEN_NOT_FOUND` removes the entry with the disk cache. Also: pass the cache entry already read into `ScreenLoader.load` (no second read/parse); skip the disk write when the response's data and etag equal the cached ones.
7. **Blocking HTTP not cancellable** (`Client.kt`): `execute()` → `enqueue` + `suspendCancellableCoroutine` (cancel the call on coroutine cancellation), like `bytes()`.
8. **Main-thread parse** (`Client.kt:127-129`, `ScreenHolder.kt:69-77`): `ScreenResponse.parse` inside the IO block; `Document.decode` on `Dispatchers.Default`.
9. **Disk cache cap — ruling R2** (`DiskCache.kt`): after a write, keep the newest 64 entries by `lastModified`. Hex key encoding via a lookup table instead of per-byte `String.format` (`DiskCache.kt:63`).
10. **Eager Animatables** (`Animated.kt:61-96`): hold the settled value; create the `Animatable` only when a motion spec first applies to that prop.
11. **Presence tracking without animation** (`Animated.kt:117-146`, `RenderContext.kt:135-137`): skip the `HashSet`s/copies when no child animates; evaluate a repeat row's key without a throwaway context/interpreter.
12. **Lazy reuse** (`Layouts.kt:42,122,126`): `contentType = { it.node.id }` on `items(...)`; child keys relative to their container (not including the repeat index), e.g. a `localKey` in `materialize`.
13. **Scroll-driven transforms recompose** (`NodeView.kt:55`, `Modifiers.kt:56-64`): transform props (opacity, offsetX/Y, scale, rotation) without an `anim` are evaluated inside the `graphicsLayer {}` block (draw phase), as timelines and drag already are.
14. **Slot body reads `env.now`** (`Slot.kt:59-64,78`): the visibility decision via `derivedStateOf` so the 1 Hz tick doesn't recompose the slot body.
15. **Per-node garbage** (`Modifiers.kt:113,122-135`): cache the semantics id string per node; compare insets without allocating `Insets(0,0,0,0)`.

- [ ] **Step 1:** Tests first (JVM unit tests beside the existing ones): TimelineRegistry — infinite autoplay timeline with no live instance → `running()` false; after `register` of a target instance → true and `version` bumped; after `unregister` → false; finite timeline behaviour unchanged. Store — token memo re-resolves after a theme change (light → dark colour differs); computed evaluated once for two reads without state change, re-evaluated after a state write. Slot/ScreenLoader — with a fake client counting requests and an injectable clock: second appearance within 30 s → 0 network calls and content shown; at 30 s → 1 call; `SCREEN_NOT_FOUND` → memo entry and disk file gone; different params → separate entries. DiskCache — 65 writes → 64 files, the oldest removed. Layout — container-relative child keys are equal for rows at different repeat indices.
- [ ] **Step 2:** Implement 1–15, running `cd android && ./gradlew :wishbone-ui:testDebugUnitTest` after each group.
- [ ] **Step 3:** Visual/behaviour check on the emulator (`emulator-5554`, Android playground, server `https://wishbone.217-198-9-139.sslip.io`): `raffle.main` (tabs, table paging to the end, countdown), `showcase.promo` (scroll-driven hero), `showcase.catalog` (spinners/shimmer), Feed demo (banner scroll out/in, animates as before). Screenshots before (base commit build) and after for the same screens → `.superpowers/sdd/2026-09-29-perf-audit-fixes/shots/android/`. Confirm with `adb shell dumpsys gfxinfo <pkg>` or frame stats that the idle `raffle.main` tasks tab produces no continuous frames after the fix.
- [ ] **Step 4:** Run `cd android && ./gradlew :wishbone-core:test :wishbone-ui:testDebugUnitTest :playground:compileDebugKotlin`.
- [ ] **Step 5:** Commit per group: `Android: цикл кадров останавливается без видимых целей бесконечных таймлайнов`, `Android: мемо токенов и вычисляемых значений, данные по источникам`, `Android: слот из памяти 30 с, отменяемые запросы, разбор вне главного потока`, `Android: кэш на диске — не больше 64 записей`, `Android: ленивые Animatable, переиспользование строк ленивых списков`, `Android: трансформации от скролла в фазе отрисовки`.

### Task 4: iOS core (`ios/Sources/WishboneCore`)

**Files:** `Interpreter.swift`, `Format.swift`, `JSONValue.swift`, `TimelineSampler.swift`, `Stdlib.swift`, tests in `ios/Tests/WishboneCoreTests/`.

**Interfaces — Produces:** `Interpreter.eval<S: Scope>(_ expr: Expr, scope: S, locals: Locals? = nil) throws -> Value` (generic; call sites unchanged); a public per-track sampler `TimelineSampler.sampleTrack(_ timeline: Timeline, track: Timeline.Track, ms: Double, index: Int) -> Double` (Task 5 calls it; use the real type names from the file).

1. **Existential box + budget per eval** (`Interpreter.swift:73-77,176,257,263`): make `eval` and the internal `evalStep`/`evalBinary`/`callStdlib`/`callLambda` generic over `S: Scope`; reuse one `StepBudget` per interpreter, resetting to 10 000 at each top-level `eval` and saving/restoring around nested evals (a nested eval must not consume or reset the outer budget differently than today — keep «fresh 10 000-step budget per call» semantics). Audit: raffle.main full render 2320 → ~730 allocations.
2. **`String(format:)` for numbers** (`Format.swift:51,55,23,27`): whole-number fast path `String(Int64(x))` for `abs(x) < 1e15` (and the `≥ 1e15` rounded branch while `< 9e18`; same in `fixedPointDigits`). Audit: 490 ns / 8 allocations → 5 ns; identical on 2M random values.
3. **`formatDate` builds a `DateFormatter` per call** (`Format.swift:159-172`): cache one formatter per (style, time zone) — thread-safe for Swift 6 (e.g. a `Mutex`-protected dictionary or a per-actor cache; formatters are used from the main actor today — check).
4. **Document decode** (`JSONValue.swift:30,104,141,167-209`): parse the raw UTF-8 bytes (`Data`/`UnsafeBufferPointer<UInt8>`) instead of copying into `[Unicode.Scalar]`. Output identical (same `JSONValue` for every fixture in `contract/`).
5. **Stdlib dispatch** (`Interpreter.swift:263-303`): move the hot names (`alpha`, `min`, `max`, `str`, `formatNumber`, `plural`, `count`) to the top of the switch; no per-call `Set<String>` construction.
6. **Per-frame sampling** (`TimelineSampler.swift:53-61`): expose `sampleTrack` (Interfaces).
7. **Regex per call** (`Stdlib.swift:125,141`): cache compiled regexes by pattern (bounded, e.g. 64 entries), as Android does.
8. **Small allocations** (`Interpreter.swift:142,383-431`, `Stdlib.swift:77`): record literals `Dictionary(minimumCapacity:)`; lambda calls avoid the per-element args array where possible; ASCII fast path in `upper`/`lower`.

- [ ] **Step 1:** Tests first (`WishboneCoreTests`): the numbers from Review Focus 1 through `str`/`formatNumber` asserting exact current output (pass before and after); `formatDate` alternating two time zones; `sampleTrack` equals the `sample()` entry; nested eval inside a lambda over 200 items still throws the budget error at the same step count as before (write the test against the current behaviour first); JSON decode of every `contract/screens/*.json` and `backend` fixture equals the old decoder's output (keep the old implementation in the test target temporarily only if needed, else compare against `JSONSerialization`-built values).
- [ ] **Step 2:** Implement 1–8. Run `cd ios && swift test --filter WishboneCoreTests`, then `swift test`. Restore `ios/Package.resolved` if a tool rewrites it.
- [ ] **Step 3:** Re-measure with the audit's bench package: copy `/private/tmp/claude-501/-Users-ivan-projects-wishbone/02100b6b-da42-4c59-985e-9ed44d06643c/scratchpad/perf/ios-core/bench/` to your own scratch, point its `.package(path:)` at your worktree's `ios/`, run it in release, report before/after for: literal eval, leaderboard row, raffle.main full render, `str(42)`, `formatDate`, document decode.
- [ ] **Step 4:** Commit per group: `iOS core: eval без экзистенциальной коробки и бюджета на вызов`, `iOS core: целые числа без String(format:), кэш форматтеров дат`, `iOS core: разбор JSON по байтам UTF-8`, `iOS core: семплер трека, регэкспы из кэша, мелкие аллокации`.

### Task 5: iOS UI (`ios/Sources/WishboneUI`)

**Files:** `NodeView.swift`, `Modifiers.swift`, `Events.swift`, `Store.swift`, `Props.swift`, `Motion.swift`, `Timelines.swift`, `WishboneSlot.swift`, `WishboneScreen.swift`, `Client.swift`, `DiskCache.swift`, `Primitives.swift`, `Fonts.swift`, `ImageLoader.swift`, tests in `ios/Tests/WishboneUITests/`.

**Interfaces — Consumes:** generic `Interpreter.eval<S: Scope>` and `TimelineSampler.sampleTrack` (Task 4).

1. **No-op modifier layers** (`Modifiers.swift:52-60,94-103,118-124,214-230`; `Events.swift:13-14`; `NodeView.swift:144-150`): apply each layer only when the node declares the prop (a key in `node.props`, static per node, so view identity stays stable): padding, `BoxFrame` (skip for wrap/wrap with no min/max), background/overlay, the box-prop opacity/scale/rotation/offset; timeline effects only for timeline/drag targets; `.onAppear`/`.onDisappear` only when `node.on` has those handlers; `.accessibilityAddTraits` only when non-empty. Keep `.zIndex`. Mostly an `if present: Bool` on the existing `animated(_:_:)` helper plus conditions at call sites.
2. **Repeat Locals churn** (`NodeView.swift:11-17`, `RepeatItem.items`): cache `Locals` per `node.id + idSuffix + "." + key` in `Store`; reuse the previous pair when the item value is `==`, the index matches and the parent locals are identical (`===`); drop entries for keys no longer present.
3. **Slot memo — ruling R1** (`WishboneSlot.swift:76` → `WishboneScreen.swift:215-240`): same rule as Android Task 3 item 6 (per-`Wishbone` memo, 30 s, `SCREEN_NOT_FOUND` clears, render from memo synchronously on appearance); skip the disk write when data and etag are unchanged.
4. **Load path on main** (`WishboneScreen.swift:215-283`, `Client.swift:145-157`, `DiskCache.swift:27-30`): move parse/decode/encode/SHA-256/disk write into `@concurrent`/nonisolated static helpers (pattern: `ImageLoader.load`); only `Store(...)` creation and the `phase` assignment stay on the main actor. Collapse redundant JSON passes (parse once, keep the parsed value).
5. **Disk cache cap — ruling R2** (`DiskCache.swift`): keep the newest 64 entries by modification date after a write; hex key encoding without 32× `String(format:)`.
6. **Per-body churn** (`Store.swift:131-134,209-216`; `NodeView.swift:85-92,145`; `Motion.swift:21`; `Props.swift:8-10`; `Events.swift:144`): memoise token values per `(group, name)` cleared on theme change; cache computed values per state generation; build transitions/`Implicit`/`ContentAnimation` arrays only when the node has them; a11y id string cached per node; `Children` iterate without copying.
7. **View inits allocate on every parent re-render** (`WishboneSlot.swift:45-56`; `WishboneScreen.swift:77-88,124-129`): create Navigator/ScreenGeometry/ScreenModel/TaskBag once (optional `@State` filled on first appearance, or `@State` with lazy init) — keep the «`onNavigate` read once» contract and the earlier ruling that `forward`/`exit` are wired before the first cached frame.
8. **Timeline sampling per frame** (`Timelines.swift:100-114`): sample only the tracks targeting `id` through `TimelineSampler.sampleTrack`, no dictionary.
9. **Fonts per text body** (`Primitives.swift:132-141,238-247`; `Fonts.swift:47-61`): memoise `(Font, lineSpacing)` per `TypeStyle` (make it `Hashable`).
10. **Image cells on cache hit** (`Primitives.swift:173-178`; `ImageLoader.swift:49-55`): when `ImageLoader.cached(url)` hits, render it directly without the async hop; dedupe concurrent loads of the same URL.
11. **Drag cells created in `body`** (`NodeView.swift:122`, `NodeBody.scoped()`): create them outside the view update (store init / `onAppear` / lazily on first drag), so no observable write happens during `body`.

- [ ] **Step 1:** Tests first (`WishboneUITests`): Locals cache — unchanged items return identical `Locals` (`===`); a changed value at the same key returns a new one; shrink drops entries. Token memo re-resolves after theme change. Slot memo — fake client counting requests + injectable clock: within 30 s → 0 calls, at 30 s → 1 call, `SCREEN_NOT_FOUND` clears memo + disk, different params → separate entries. DiskCache — 65 writes → 64 files. `RenderSmokeTests` stay green.
- [ ] **Step 2:** Implement 1–11. Run `cd ios && swift test --filter WishboneUITests`, then `swift test`.
- [ ] **Step 3:** Visual check in the iOS Playground on a simulator not used by another agent (server `https://wishbone.217-198-9-139.sslip.io`): `raffle.main` (tabs, table paging to the end with fast swipes — no hang, CPU back to idle), `showcase.catalog`, `showcase.onboarding`, `showcase.promo`, `showcase.form`, Feed demo (banner in/out). Screenshots of the same screens before (base commit) and after → `.superpowers/sdd/2026-09-29-perf-audit-fixes/shots/ios/`; they must match visually. Watch the console for «Modifying state during view update».
- [ ] **Step 4:** Commit per group: `iOS: узел без пустых модификаторов`, `iOS: repeat переиспользует Locals неизменённых элементов`, `iOS: слот из памяти 30 с, загрузка экрана вне главного потока`, `iOS: кэш на диске — не больше 64 записей`, `iOS: мемо токенов, шрифтов и вычисляемых значений`, `iOS: семплер трека без словаря, картинки из кэша без async-прыжка, drag-ячейки вне body`.

### Task 6: Docs and rulings (controller)

**Files:** `docs/superpowers/specs/2026-09-28-raffles-slots-host-nav-design.md` §2.3, `docs/superpowers/decisions/renderers.md`, `docs/superpowers/decisions/server.md`, `docs/README.md` status line.

- [ ] **Step 1:** §2.3: replace «Загрузка — при появлении вьюхи (кэш сразу, затем сеть, как у экрана). Принудительное обновление — сменой identity вьюхи у хоста.» with the R1 rule; add the R2 cap to renderers.md (both SDKs); add the server notes (JSON columns scanned as bytes, `Meta` query, ETag-aware `Live`, spliced response, upstream pool) to server.md; add the Android frame-loop rule (infinite timelines tick only while a target is live) to renderers.md.
- [ ] **Step 2:** Commit `Доки: итоги перф-аудита — слот из памяти, лимит кэша, правила таймлайнов и сервера`.
