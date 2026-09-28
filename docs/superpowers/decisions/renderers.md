# Renderer — controller decisions (binding for iOS WishboneUI and Android :wishbone-ui)

Normative: spec §2 (primitives, modifiers, tokens), §4 (motion), §5.2/§5.4 + contract/api.md (network, preview), §6 (SDK, host API, theme), contract/primitives.json (props, defaults), contract/stdlib.json (env), contract/README.md (timeline sampling). The runtime core (WishboneCore / :wishbone-core) already exists: Value, AST, Scope, Interpreter, EffectSink, TimelineSampler — build on it, don't re-implement. Both platforms mirror the same structure and names (Store, RenderContext, NodeView, Modifiers, Theme, Navigator, Client, PreviewClient, Playground).

Work is split: **phase a** = store, client, cache, theme, fonts, renderer of every primitive + modifiers + a11y, events, effects, navigation, preview, playground; **phase b** = motion (§4: implicit animate, transactions, transitions, matchId, timelines, drivers, drag, self.pressed, reduceMotion) + Lottie/Rive adapters.

## Store (one per rendered screen)
- Holds document, params, data values, **state cells** (one observable cell per state key: iOS `@Observable final class Cell`, Android `mutableStateOf`), driver cells (per scroll/drag id and axis), timeline registry, pending transaction (Android), error log.
- Computed values are evaluated on read (no cache). Expressions are evaluated inside SwiftUI `body` / composables so the platform's observation tracks exactly the cells read → only dependent nodes re-render.
- Implements core `Scope` together with a `RenderContext` (repeat locals, instance scope chain, self-pressed state, event payload). Instance scopes: a node carrying `inst` creates an InstanceScope (cells keyed by qualified name, parent pointer) owned by the view identity (SwiftUI `@State`, Compose `remember` keyed by node id + repeat key path); `instanceState(name)` walks up the chain.
- Handler runs: core Interpreter + an EffectSink implemented by the store (navigation, network, host, haptics, timelines, preview logs). Eval/handler errors never crash: they are logged (preview `log` message when a preview session is attached; `assertionFailure`/`Log.w` in debug), and the failing node renders nothing.
- On a new document for the same screen (preview hot-reload / SWR refresh with a new revision): rebuild the store but **keep values of state keys that still exist** (same name) so editing doesn't reset the screen.

## Rendering
- Recursive `NodeView(node, ctx)`; unknown `t` → nothing (+ log). `when` false → not in the tree. `repeat` → one child per item, identity = `key` value (else index), locals `as`/`index`.
- **Box model (canonical, both platforms):** content → padding (inside) → size (`width`/`height`: number = fixed, `"fill"` = take all available space along that axis / share remaining space equally with other fill siblings in a stack, `"wrap"` = intrinsic; min/max; aspect) → background (fills the box, shaped by `corner`) / material blur → border (shaped by `corner`) → clip (`corner`) → shadow → transform (opacity, offsetX/Y, scale, rotation — visual only) → zIndex. SwiftUI: `.padding().frame().background(shape).overlay(stroke).clipShape().shadow().opacity().scaleEffect().rotationEffect().offset().zIndex()`; Compose (outer → inner): `zIndex → graphicsLayer(alpha, scale, rotation, translation) → shadow → size/fill → clip → background → border → padding`. `corner` ≥ half the smaller side renders as a capsule (pill = 9999). A numeric `width`/`height` is exact (SwiftUI `frame(width:)`; Compose `requiredWidth/requiredHeight`, not `width`), so a child wider than its clipped parent keeps its size. `border`/`stroke` with `width` 0 draws nothing (Compose `border(0.dp)` would be a hairline — skip it); strokes are drawn inside the bounds (SwiftUI `strokeBorder`, Compose inset by width/2). Every node, including `spacer`, supports the common events (`appear`, `disappear`, `tap`, `longPress`).
- Primitives: `stack` v/h/z (VStack/HStack/ZStack ↔ Column/Row/Box) with `spacing`, `align` per spec §2.1; `spacer` (`Spacer(minLength:)` ↔ `Spacer(Modifier.weight(1f))` inside Row/Column — pass parent axis through ctx); `scroll` (ScrollView ↔ verticalScroll/horizontalScroll; `lazy` → LazyVStack/LazyColumn; `paging` → `.scrollTargetBehavior(.paging)` ↔ pager-like snap fling; `indicators`); `grid` (LazyVGrid fixed columns ↔ rows of `columns` cells); `text` (theme type role: family+weight → registered font, size, lineHeight, tracking; scales with Dynamic Type / font scale; color token; textAlign; maxLines 0 = unlimited); `image` (AsyncImage ↔ Coil `AsyncImage`, `fit`, placeholder color); `icon` (icons table: SF Symbol ↔ `wb_ic_*` drawable, tinted, size); `shape` (rect/circle/capsule, fill color or gradient, stroke); `textField` (TextField/SecureField ↔ BasicTextField/TextField styled by tokens; edits fire `change` with `event.value`; `submit` on return; keyboard types; **controlled when the `value` prop is present** — it always shows the evaluated `value`, so if the screen doesn't update its state the edit is reverted; **uncontrolled when `value` is absent** — the field keeps its own text); `lottie`/`rive` via a renderer registry (phase b adapters register themselves; missing → nothing); `native` → `host.nativeView(name, props)`.
- URLs in `image`/`lottie`/`rive` (and `open`) that start with `/` are resolved against the server base URL (screens reference bundled assets as `/assets/...`).
- Colors: token → theme color for the current scheme; `alpha` multiplies opacity; gradient = linear with CSS angle semantics (0 = to top, 90 = to right).
- a11y: identifier `wb.<screen>.<nodeId>` (+ `.<repeatKey>` inside repeat); `a11y.label`, role button → button trait/`Role.Button`, header → header trait/`heading()`, image; `hidden`; a container with an identifier uses `.accessibilityElement(children: .contain)` so children stay visible to UI tests.
- Tap handling (phase a: plain; phase b adds pressed): a node with `on.tap` is tappable without blocking scroll (iOS: `Button` with a custom `ButtonStyle` — gives pressed state and a11y; Android: `clickable(interactionSource, indication = null)`), `longPress` too.

## Theme & fonts
- `GET /v1/theme?brand=` with ETag, cached on disk; bundled fallback = copy of contract/tokens/korpu.json inside the SDK (a unit test asserts the copy equals the contract file). Fonts from contract/assets/fonts bundled in the SDK and registered at startup (iOS `CTFontManagerRegisterFontsForURL`; Android `res/font` + FontFamily by family name and weight; missing weight → nearest). Scheme follows the system unless the host/playground overrides it.

## Network & cache (`Client`)
- Exactly contract/api.md; headers `X-Wishbone-Runtime: 1.0.0`, platform, brand, `Accept-Language`, plus `host.authHeaders()`. iOS URLSession, Android OkHttp 4.12 + kotlinx.serialization.
- Screen: show the cached response immediately (disk cache per screen+params: document + etag + data), fetch with `If-None-Match`, swap in the fresh one; no cache + error → built-in fallback view (message + Retry, strings ru/en in the SDK).
- `call`: POST /v1/call with a fresh UUID idempotencyKey per invocation; transport failure → `{ok: false, error: {code: "NETWORK", message}}`. `refresh(x)` → POST /v1/data.

## Navigation & effects
- SDK-owned stack inside `WishboneScreen`: iOS `NavigationStack(path:)`; Android own back stack + `AnimatedContent` + `BackHandler`. `navigate(id, params, "push"|"fade")`, `back()`, `present(id, params)` = modal sheet (iOS `.sheet` large detent + drag indicator; Android `ModalBottomSheet`), `dismiss()` closes the sheet or pops. `open(url)`: https or a scheme the host allows → `host.open`, else ignored. `haptic` → UIFeedbackGenerator / `performHapticFeedback`. `track` → host. `host(...)` → `host.action`. `wait` → delay.
- Env: `now` from a shared 1 s ticking clock (only ticks while screens are visible), platform, locale (preferred language, lowercased), brand, colorScheme, reduceMotion (system setting), screen size, safe area, timeZone.

## Preview (`PreviewClient`)
- `GET /v1/preview/sessions`, WS `/v1/preview/ws?session=`, `hello` on connect, `document` → render that document (with its data) in place, `play` → start the timeline, runtime errors → `log` messages. Reconnect with backoff.

## Playground app
- Home: server base URL (default iOS `http://localhost:8080`, Android emulator `http://10.0.2.2:8080`), brand picker (from theme brands: korpu, kz), scheme picker (system/light/dark), list of live preview sessions (tap → live preview screen), list of screens `showcase.onboarding`, `showcase.promo`, `showcase.form`, `showcase.catalog` + a field to open any id.
- The playground's host implementation logs `track`/`host`/`open` to the console and shows a toast/banner for host actions.

## Motion (phase b) — spec §4 made precise
- Motion value → platform animation: spring `{response ms, damping}` → SwiftUI `.spring(response: s, dampingFraction:)` / Compose `spring(dampingRatio = damping, stiffness = (2π / response_s)²)`; curve `{duration ms, bezier}` → `.timingCurve(x1,y1,x2,y2, duration:)` / `tween(duration, CubicBezierEasing(...))`; token names resolved through the theme.
- Implicit `animate[prop]` (or `*`): iOS scoped `.animation(anim) { $0.modifier(...) }` (iOS 17 API) per animated prop group; Android `animateFloatAsState`/`animateColorAsState`/`animateDpAsState` with that spec.
- Transactions `animate(m) { … }`: iOS `withAnimation(m)` around each state write made inside; Android: the store records the transaction spec with the write; props without an implicit spec animate with it on their next target change.
- `transition` on nodes entering/leaving (`when`, repeat items): iOS `.transition(.asymmetric(...))`; Android `AnimatedVisibility(enter, exit)` (fade/slide(edge)/scale(from)/offset(x,y)); the motion is the current transaction's, else the node's `animate["*"]`, else `@motion.smooth`.
- `matchId` between `when` branches: iOS `matchedGeometryEffect(id:in:)` (namespace per screen); Android `SharedTransitionLayout` at the screen root + `sharedBounds(rememberSharedContentState(key), animatedVisibilityScope)` from the node's AnimatedVisibility.
- Timelines: registry of active timelines `{name → start time}`; `autoplay` starts at first render; `play` restarts; `stop` removes; values from core `TimelineSampler` composed onto the transform layer (opacity/scale ×, offsets/rotation +) of every materialized instance of the target id (stagger by repeat index); frames via `TimelineView(.animation)` (iOS) / `withFrameNanos` loop (Android) only while a timeline is active; `await play` resumes after `duration(timeline, instanceCount)`.
- Drivers: `scroll.<id>.y|x` = content offset from the resting top/left (iOS `onScrollGeometryChange`, negative when overscrolled; Android scroll state value / lazy list offset), written into driver cells every frame; `drag` nodes: translation follows the finger along `axis` (clamped to `bounds`), `drag.<id>.x|y` cells; release → spring (`@motion.interactive`) to the nearest `snap` point (or back to 0), then `dragEnd` with `{x, y, vx, vy}` (translation at release, velocity pt/s).
- `self.pressed` = pressed state of the node's tap gesture (ButtonStyle `isPressed` / `collectIsPressedAsState`).
- `env.reduceMotion`: timelines render their final state, slide/scale/offset transitions become fades, drags still work.
- Lottie (`WishboneLottie`: lottie-ios `LottieView` loading from URL; Android `lottie-compose` `LottieCompositionSpec.Url`): `playing`, `loop`, `speed`, `progress` (drives the frame when set), `end` event. Rive (`WishboneRive`: RiveRuntime `RiveViewModel(webURL:stateMachineName:)`; Android rive-android `RiveAnimationView` in `AndroidView`): `inputs` map → state-machine inputs (bool/number) updated on change, Rive events → `event` with `{name}`.

## Parity rulings from phase a (binding for both platforms)
- Safe area: the root node is laid out inside the safe area; its `background` extends behind the system bars. The root's `ignoreSafeArea` ("none" | "top" | "bottom" | "all", contract/primitives.json common) lets content go edge-to-edge on those edges; `env.safeTop/safeBottom` report the insets.
- A root node with `wrap` size is centered on the screen.
- `shape`, `image`, `scroll` with `wrap` size take all offered space (flexible, like SwiftUI); `grid` and `textField` take the offered width only.
- A stack larger than its content places children at the START of its main axis (top for v, leading for h); cross axis per `align`; z-stack per `align` (default center). iOS must pass the matching frame alignment (e.g. `.frame(maxHeight: .infinity, alignment: .top)`).
- Grid cells are centered in their cell.
- `textField` placeholder = the field's text color at 0.4 alpha.
- Template instance state lives in the screen Store keyed by node id + repeat key path (survives push/pop), not in view-local state. Known ceiling: state of removed repeat items stays for the screen's lifetime.
- `material`: iOS real material blur; Android translucent `bg1` fill (no backdrop blur in Compose) — accepted divergence.
- `indicators` is ignored on Android. `paging` = pager on Android, `.scrollTargetBehavior(.paging)` on iOS.
- Inside a presented sheet: DSL `back()` pops within the sheet's own stack; the system back gesture/button closes the whole sheet.
- `WishboneHost.allowedSchemes: Set<String>` (default empty) — extra URL schemes `open()` may pass to the host besides https.
- The SDK's bundled theme copy must equal contract/tokens/korpu.json (test); re-copy when tokens change.

## Notes from the showcase screens (binding)
- Timeline `stagger` uses the index of the innermost `repeat` that materializes the target node (e.g. `perk` inside a nested repeat staggers by the inner index).
- Drag state (`drag.<id>.x|y`) is kept per materialized instance (node id + repeat key); `dragEnd` `event.x/y` = translation at release (before snapping).
- A `drag.<id>.x|y` read resolves to the cell at `id` + the reader's repeat key path, else the nearest shorter prefix of that path that has a cell, else 0 (Android rule, iOS aligned; reading never creates a cell).
- Each materialized instance of a `when` node gets its own enter/exit transition (onboarding pages are one repeated node with `when: i == page`).
- `rive` views do not intercept taps (V1): pointer input passes to ancestors (the enclosing card handles the tap).
- `matchId` pairs may be sibling `when` branches of the root — the shared-transition scope must cover the whole screen.
- `env.safeTop` / `env.safeBottom` = system bars + display cutout ONLY, never the keyboard (iOS: read insets outside keyboard avoidance, e.g. `.ignoresSafeArea(.keyboard)`; Android: `WindowInsets.systemBars.union(WindowInsets.displayCutout)`). The keyboard is handled by the platform's normal content avoidance.
- Root `ignoreSafeArea` must be honored on BOTH platforms (Android: skip the system-bar padding for the chosen edges).
- A repeat instance's drag offset resets to 0 when the instance leaves the tree (re-added items with the same key start at rest).
- `scroll` padding pads the SCROLLED CONTENT (iOS `contentMargins(.scrollContent)` / padding inside the ScrollView; Android `contentPadding` / padding inside `verticalScroll`), not the viewport — content and shadows scroll edge-to-edge instead of being clipped at the padding.
- `shape` `kind: circle` on a non-square box = a centered circle with diameter min(w, h) (not a capsule).
- `shadow` on a node without `background` follows the content (iOS default); on Android best effort (shape of the node) — accepted divergence.
- `dragEnd` fires at RELEASE (with the release translation and velocity), while the snap spring animates in parallel — handlers that remove the item make it disappear immediately instead of after the spring settles.

## Host integration (binding, added with the Stratoapp/most adapters)
- `WishboneScreen(…, onExit)`: DSL `back()`/`dismiss()` at the SDK root call `onExit` on both platforms. Without it iOS falls back to SwiftUI `dismiss` and Android does nothing. iOS reads `onExit` once when the screen appears, Android always calls the latest one. System back at the root is the host's own (the SDK's Android `BackHandler` covers only its inner pushes); a sheet's dismiss request is handled only while a sheet is open.
- Screen ids are checked against the server's id rule (`[a-z0-9][a-z0-9._-]{0,63}`) before any request or cache access; anything else (e.g. `..` from a deeplink) fails as `SCREEN_NOT_FOUND` locally.
- Android `Wishbone.colorScheme` is Compose state: a host with its own light/dark setting assigns it (iOS takes the scheme from the SwiftUI environment, e.g. the host's `preferredColorScheme`).
