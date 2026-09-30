import Observation
import SwiftUI
import WishboneCore

/// A server-driven screen (spec §6.3, §1.2). Two navigation modes, chosen by whether `onNavigate` is
/// passed: without it, `WishboneScreen(wishbone, id: "showcase.promo", params: [:])` owns its own
/// `NavigationStack` and DSL `navigate` pushes into it. With `onNavigate`, DSL `navigate` instead calls
/// `onNavigate(route)` and the SDK does not wrap the screen in a stack of its own — the host is expected
/// to push `WishboneScreen(wishbone, route:)` into ITS stack (a `NavigationStack`, a reducer's own
/// destination stack, whatever the host uses). `present`/`dismiss` are unaffected: a sheet always keeps
/// its own stack inside the SDK, in both modes. `onExit` runs when the screen asks to leave the SDK —
/// `back()`/`dismiss()` at the root; without it the SDK calls SwiftUI's `dismiss`. Both `onExit` and
/// `onNavigate` are read once, when the screen is created (pass closures over a stable router, not ones that
/// change between renders).
public struct WishboneScreen: View {
    let wishbone: Wishbone
    let root: Route
    let onExit: (() -> Void)?
    let onNavigate: ((WishboneRoute) -> Void)?
    /// `init(id:)`: identity = id + params (re-navigating to the same id/params reuses the store).
    /// `init(route:)`: identity = the route's own `id` — each host push is its own stack record, even to
    /// the same screen id (e.g. `raffle.tickets` pushed twice with different `offset`).
    let identity: String

    public init(_ wishbone: Wishbone, id: String, params: [String: Value] = [:], onExit: (() -> Void)? = nil,
                onNavigate: ((WishboneRoute) -> Void)? = nil) {
        self.wishbone = wishbone
        root = Route(screen: id, params: params)
        self.onExit = onExit
        self.onNavigate = onNavigate
        identity = "\(id)|\(API.jsonString(.record(params)))"
    }

    /// Rebuilds the screen the host just pushed from the `WishboneRoute` its own `onNavigate` received.
    public init(_ wishbone: Wishbone, route: WishboneRoute, onExit: (() -> Void)? = nil,
                onNavigate: ((WishboneRoute) -> Void)? = nil) {
        self.wishbone = wishbone
        root = Route(route)
        self.onExit = onExit
        self.onNavigate = onNavigate
        identity = route.id.uuidString
    }

    public var body: some View {
        // A different id/params (or a different host push) is a different screen: fresh stack, model and store.
        StackHost(wishbone: wishbone, root: root, presenter: nil, onExit: onExit, onNavigate: onNavigate)
            .id(identity)
    }
}

/// Live preview of an admin session: renders every document the session pushes, in place. A render job (spec
/// 2026-09-30 §2) is drawn in a capture layer underneath — same window, size, safe area and scheme — that the user never
/// sees or touches; an opaque cover keeps it hidden under a live root without a background.
public struct WishbonePreviewScreen: View {
    let wishbone: Wishbone
    let root: Route

    public init(_ wishbone: Wishbone, preview: PreviewClient) {
        self.wishbone = wishbone
        root = Route(screen: "preview", params: [:], preview: preview)
    }

    public var body: some View {
        let preview = root.preview!
        ZStack {
            #if canImport(UIKit)
            if let job = preview.job {
                CaptureLayer(wishbone: wishbone, job: job) { preview.finish(job.id, $0) }
                    .id(job.id)
                    .ignoresSafeArea() // the full window, as the live screen: UIKit then hands it the same safe area
            }
            #endif
            Rectangle().fill(.background).ignoresSafeArea()
            StackHost(wishbone: wishbone, root: root, presenter: nil)
        }
        .id(ObjectIdentifier(preview))
    }
}

struct StackHost: View {
    let wishbone: Wishbone
    let root: Route
    let onExit: (() -> Void)?
    /// Host-stack mode (spec §1.2): non-nil only at the top of a screen's own stack, never passed down to
    /// a presented sheet (`.sheet` below omits it) — a sheet always keeps its own stack inside the SDK.
    let onNavigate: ((WishboneRoute) -> Void)?
    /// Made once per stack (not on every re-render of the host).
    @StateObject private var once: Once<Navigator>
    @Environment(\.dismiss) private var dismiss

    init(wishbone: Wishbone, root: Route, presenter: Navigator?, onExit: (() -> Void)? = nil,
         onNavigate: ((WishboneRoute) -> Void)? = nil) {
        self.wishbone = wishbone
        self.root = root
        self.onExit = onExit
        self.onNavigate = onNavigate
        _once = StateObject(wrappedValue: Once(Self.navigator(presenter: presenter, onExit: onExit, onNavigate: onNavigate)))
    }

    private static func navigator(presenter: Navigator?, onExit: (() -> Void)?, onNavigate: ((WishboneRoute) -> Void)?) -> Navigator {
        let navigator = Navigator(presenter: presenter)
        // Wired before the first frame: a cached root's `appear` may navigate or leave at once.
        navigator.forward = onNavigate
        navigator.exit = onExit
        return navigator
    }

    private var navigator: Navigator { once.value }

    /// Host-stack mode: just the route, no stack of its own. Plain mode: the SDK's own `NavigationStack`.
    @ViewBuilder private var content: some View {
        if onNavigate != nil {
            RouteView(wishbone: wishbone, route: root, navigator: navigator)
        } else {
            NavigationStack(path: Bindable(navigator).path) {
                RouteView(wishbone: wishbone, route: root, navigator: navigator)
                    .navigationDestination(for: Route.self) { RouteView(wishbone: wishbone, route: $0, navigator: navigator) }
            }
        }
    }

    var body: some View {
        content
            .sheet(item: Bindable(navigator).sheet) { route in
                StackHost(wishbone: wishbone, root: route, presenter: navigator)
                    .presentationDetents([.large])
                    .presentationDragIndicator(.visible)
            }
            .modifier(EnvReader(env: wishbone.env, geometry: navigator.geometry))
            .onAppear {
                // SwiftUI's `dismiss` is read from the environment, which only an installed view has.
                if navigator.exit == nil { navigator.exit = { dismiss() } }
            }
    }
}

/// One screen of the stack: loads its document (cache first, then network), renders it.
struct RouteView: View {
    let wishbone: Wishbone
    let route: Route
    /// Made once per screen (not on every re-render of its stack).
    @StateObject private var once: Once<ScreenModel>
    @State private var shown: Bool

    init(wishbone: Wishbone, route: Route, navigator: Navigator) {
        self.wishbone = wishbone
        self.route = route
        _once = StateObject(wrappedValue: Once(ScreenModel(wishbone: wishbone, route: route, navigator: navigator)))
        _shown = State(initialValue: !route.fade)
    }

    private var model: ScreenModel { once.value }

    var body: some View {
        Group {
            switch model.phase {
            case .loading:
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            case let .ready(store):
                ScreenContent(store: store)
            case let .failed(message):
                FallbackView(message: message) { Task { await model.load() } }
            }
        }
        .opacity(shown ? 1 : 0)
        .task(id: route.preview?.revision) {
            if let preview = route.preview {
                if let doc = preview.latest { model.apply(doc) }
            } else {
                await model.load()
            }
        }
        .onAppear {
            wishbone.clock.retain()
            // A warm cache is on screen in the first frame (`load()` then only revalidates).
            if route.preview == nil, model.store == nil { model.showCached() }
            if !shown { withAnimation(.easeOut(duration: 0.3)) { shown = true } }
        }
        .onDisappear { wishbone.clock.release() }
        .onChange(of: route.preview?.play?.seq) {
            if let name = route.preview?.play?.timeline { model.store?.timelines.play(name) }
        }
        #if os(iOS)
        .toolbar(.hidden, for: .navigationBar)
        .background(SwipeBackEnabler(allowsContentPop: !(model.store?.hasDrag ?? false)).frame(width: 0, height: 0))
        #endif
    }
}

struct ScreenContent: View {
    let store: Store
    /// `matchId` scope: the whole screen.
    @Namespace private var namespace

    var body: some View {
        var ctx = RenderContext(store: store)
        ctx.isRoot = true
        ctx.namespace = namespace
        // The root's `ignoreSafeArea` widens the whole screen area (not just the root inside a centered frame).
        let edges = store.document.root.props["ignoreSafeArea"].flatMap { ctx.eval($0) }.flatMap { v -> String? in
            if case let .string(s) = v { s } else { nil }
        }
        return NodeView(node: store.document.root, ctx: ctx)
            .frame(maxWidth: .infinity, maxHeight: .infinity) // a wrap-sized root is centered
            .ignoresSafeArea(.container, edges: NodeBody.edges(edges))
    }
}

/// A screen answer with its document decoded: what a store renders, what the disk cache keeps, the next etag.
struct LoadedScreen: Sendable {
    var response: ScreenResponse
    let document: Document
}

/// Screen loading with stale-while-revalidate: the cached answer renders at once, the fresh one
/// replaces it (keeping state values), `document: null` (etag matched) only refreshes data.
@MainActor @Observable
final class ScreenModel {
    enum Phase {
        case loading
        case ready(Store)
        case failed(String)
    }

    private(set) var phase: Phase = .loading
    @ObservationIgnored let wishbone: Wishbone
    @ObservationIgnored let route: Route
    @ObservationIgnored let navigator: Navigator
    /// `WishboneSlot`: an unpublished screen drops its cache (see `dropsCache`); its answers go to the slot memo.
    @ObservationIgnored let slot: Bool
    /// The answer on screen (cache, memo or network): the etag and data of the next revalidation.
    @ObservationIgnored private var cached: LoadedScreen?
    /// Handler runs of this screen's stores; cancelled when the screen goes away.
    @ObservationIgnored let tasks = TaskBag()

    init(wishbone: Wishbone, route: Route, navigator: Navigator, slot: Bool = false) {
        self.wishbone = wishbone
        self.route = route
        self.navigator = navigator
        self.slot = slot
    }

    deinit { tasks.cancelAll() }

    var store: Store? { if case let .ready(s) = phase { s } else { nil } }

    /// The network answer is parsed, decoded, encoded and written off the main actor; only the store and `phase` are
    /// made on it.
    func load() async {
        if case .failed = phase { phase = .loading }
        guard let key = showCached() else { return } // shown on appearance already, unless dropped since
        if slot, wishbone.slotMemo.isFresh(key) { return } // R1: the network answered less than 30 s ago
        do {
            let req = API.screen(await wishbone.requestInfo(), id: route.screen, params: route.params, etag: cached?.response.etag)
            let (response, document) = try await Self.fetch(req, send: wishbone.send)
            let previous = cached, fresh: LoadedScreen
            if let document {
                fresh = LoadedScreen(response: response, document: document)
                show(fresh)
            } else if var c = previous {
                (c.response.etag, c.response.data, c.response.rawData) = (response.etag, response.data, response.rawData)
                store?.update(data: c.response.data)
                fresh = c
                cached = c
            } else {
                throw WishboneError.decode("no document")
            }
            if slot { wishbone.slotMemo.put(key, fresh) }
            if previous?.response.etag != fresh.response.etag || previous?.response.data != fresh.response.data {
                await Self.write(wishbone.cache, key, fresh.response)
            } else {
                await Self.keep(wishbone.cache, key, fresh.response) // the disk has this answer: only mark it used
            }
        } catch {
            wishbone.log("warn", "screen \(route.screen): \(error)")
            if Self.dropsCache(after: error, slot: slot) {
                wishbone.slotMemo.remove(key)
                cached = nil
                fail(error)
                await Self.remove(wishbone.cache, key)
                return
            }
            if cached == nil { fail(error) }
        }
    }

    /// The disk cache key of a network screen; nil for a local screen (shown at once, nothing to fetch) or an
    /// invalid id (failed).
    private func cacheKey() -> String? {
        if let local = wishbone.localScreens[route.screen] {
            do { try apply(throwing: API.parseScreen(local)) } catch { fail(error) }
            return nil
        }
        // the server's id rule: anything else (e.g. `..` from a deeplink) would resolve to another path
        guard API.isScreenID(route.screen) else {
            fail(WishboneError.server(status: 404, code: "SCREEN_NOT_FOUND", message: "invalid screen id \"\(route.screen)\"", retryable: false))
            return nil
        }
        return DiskCache.screenKey(scope: wishbone.cacheScope, id: route.screen, params: route.params)
    }

    /// The synchronous part of `load()`, run on appearance so a warm slot or screen is there in its first frame: a
    /// local screen, else unless something is shown already a slot's memo answer (R1), else the disk cache (ponytail:
    /// a main-actor read and decode of one file, ~1 ms for raffle.main, once per model) → the cache key.
    @discardableResult
    func showCached() -> String? {
        guard let key = cacheKey() else { return nil }
        if cached == nil, let entry = (slot ? wishbone.slotMemo[key] : nil) ?? Self.decode(wishbone.cache.read(key)) {
            show(entry)
        }
        return key
    }

    /// A slot's `SCREEN_NOT_FOUND` (unpublished or removed) drops its cache and shows nothing, even over
    /// the cached content — otherwise a removed banner would live on in the cache (spec 2026-09-28 §2.3).
    /// A screen keeps showing its cache on any error.
    nonisolated static func dropsCache(after error: Error, slot: Bool) -> Bool {
        guard slot, case let .server(_, code, _, _)? = error as? WishboneError else { return false }
        return code == "SCREEN_NOT_FOUND"
    }

    /// Renders an answer (preview document, local screen); state of the previous store survives.
    func apply(_ r: ScreenResponse) {
        do { try apply(throwing: r) } catch { fail(error) }
    }

    private func apply(throwing r: ScreenResponse) throws {
        guard let json = r.document else { throw WishboneError.decode("no document") }
        show(LoadedScreen(response: r, document: try Document(json: json)), caching: false)
    }

    /// In-flight handlers of the replaced store may finish (their effects still run); runs are only cancelled when the
    /// screen itself goes away (deinit). `caching` — the answer is the next revalidation's base.
    private func show(_ screen: LoadedScreen, caching: Bool = true) {
        if caching { cached = screen }
        // Values carry over by name between revisions of the same screen only: a preview session that switched screens
        // (spec 2026-09-30 §1.1) starts the new one fresh.
        let previous = store?.document.screen == screen.document.screen ? store : nil
        phase = .ready(Store(document: screen.document, params: route.params, data: screen.response.data, wishbone: wishbone,
                             navigator: navigator, previous: previous, tasks: tasks))
    }

    private func fail(_ error: Error) {
        wishbone.log("error", "screen \(route.screen): \(error)")
        phase = .failed(String(describing: error))
    }

    // MARK: off the main actor

    /// A cache entry → its answer with the document decoded (nil: none, unreadable, or a document that doesn't decode).
    nonisolated static func decode(_ bytes: Data?) -> LoadedScreen? {
        guard let bytes, let r = try? API.parseScreen(bytes), let json = r.document, let doc = try? Document(json: json) else { return nil }
        return LoadedScreen(response: r, document: doc)
    }

    /// The network answer, with its document decoded (nil: the etag matched).
    @concurrent nonisolated static func fetch(_ req: URLRequest, send: @Sendable (URLRequest) async throws -> (Data?, HTTPURLResponse))
        async throws -> (ScreenResponse, Document?) {
        let (body, _) = try await send(req)
        let r = try API.parseScreen(body ?? Data())
        return (r, try r.document.map { try Document(json: $0) })
    }

    @concurrent nonisolated static func write(_ cache: DiskCache, _ key: String, _ r: ScreenResponse) async {
        if let bytes = try? API.encodeScreen(r) { cache.write(key, bytes) }
    }

    /// Touches the entry (the cap keeps the most recently revalidated), or writes it when it went meanwhile.
    @concurrent nonisolated static func keep(_ cache: DiskCache, _ key: String, _ r: ScreenResponse) async {
        if !cache.touch(key), let bytes = try? API.encodeScreen(r) { cache.write(key, bytes) }
    }

    @concurrent nonisolated static func remove(_ cache: DiskCache, _ key: String) async {
        cache.remove(key)
    }
}

/// No cache and no network: message + Retry (ru/en).
struct FallbackView: View {
    let message: String
    let retry: () -> Void

    var body: some View {
        let ru = Locale.preferredLanguages.first?.lowercased().hasPrefix("ru") ?? false
        VStack(spacing: 12) {
            Image(systemName: "exclamationmark.triangle").font(.largeTitle).foregroundStyle(.secondary)
            Text(verbatim: ru ? "Не удалось загрузить экран" : "Couldn't load the screen").font(.headline)
            Text(verbatim: message).font(.footnote).foregroundStyle(.secondary).multilineTextAlignment(.center)
            Button(action: retry) { Text(verbatim: ru ? "Повторить" : "Retry") }.buttonStyle(.borderedProminent)
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
