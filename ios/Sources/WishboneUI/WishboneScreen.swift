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
/// `onNavigate` are read once, when the screen appears (pass closures over a stable router, not ones that
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

/// Live preview of an admin session: renders every document the session pushes, in place.
public struct WishbonePreviewScreen: View {
    let wishbone: Wishbone
    let root: Route

    public init(_ wishbone: Wishbone, preview: PreviewClient) {
        self.wishbone = wishbone
        root = Route(screen: "preview", params: [:], preview: preview)
    }

    public var body: some View {
        StackHost(wishbone: wishbone, root: root, presenter: nil)
            .id(ObjectIdentifier(root.preview!))
    }
}

struct StackHost: View {
    let wishbone: Wishbone
    let root: Route
    let onExit: (() -> Void)?
    /// Host-stack mode (spec §1.2): non-nil only at the top of a screen's own stack, never passed down to
    /// a presented sheet (`.sheet` below omits it) — a sheet always keeps its own stack inside the SDK.
    let onNavigate: ((WishboneRoute) -> Void)?
    @State private var navigator: Navigator
    @Environment(\.dismiss) private var dismiss

    init(wishbone: Wishbone, root: Route, presenter: Navigator?, onExit: (() -> Void)? = nil,
         onNavigate: ((WishboneRoute) -> Void)? = nil) {
        self.wishbone = wishbone
        self.root = root
        self.onExit = onExit
        self.onNavigate = onNavigate
        _navigator = State(initialValue: Navigator(presenter: presenter))
    }

    var body: some View {
        Group {
            if let onNavigate {
                RouteView(wishbone: wishbone, route: root, navigator: navigator)
                    .sheet(item: $navigator.sheet) { route in
                        StackHost(wishbone: wishbone, root: route, presenter: navigator)
                            .presentationDetents([.large])
                            .presentationDragIndicator(.visible)
                    }
                    .modifier(EnvReader(env: wishbone.env, geometry: navigator.geometry))
                    .onAppear {
                        navigator.exit = onExit ?? { dismiss() }
                        navigator.forward = onNavigate
                    }
            } else {
                NavigationStack(path: $navigator.path) {
                    RouteView(wishbone: wishbone, route: root, navigator: navigator)
                        .navigationDestination(for: Route.self) { RouteView(wishbone: wishbone, route: $0, navigator: navigator) }
                }
                .sheet(item: $navigator.sheet) { route in
                    StackHost(wishbone: wishbone, root: route, presenter: navigator)
                        .presentationDetents([.large])
                        .presentationDragIndicator(.visible)
                }
                .modifier(EnvReader(env: wishbone.env, geometry: navigator.geometry))
                .onAppear { navigator.exit = onExit ?? { dismiss() } }
            }
        }
    }
}

/// One screen of the stack: loads its document (cache first, then network), renders it.
struct RouteView: View {
    let wishbone: Wishbone
    let route: Route
    @State private var model: ScreenModel
    @State private var shown: Bool

    init(wishbone: Wishbone, route: Route, navigator: Navigator) {
        self.wishbone = wishbone
        self.route = route
        _model = State(initialValue: ScreenModel(wishbone: wishbone, route: route, navigator: navigator))
        _shown = State(initialValue: !route.fade)
    }

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
    @ObservationIgnored private var cached: ScreenResponse?
    /// Handler runs of this screen's stores; cancelled when the screen goes away.
    @ObservationIgnored let tasks = TaskBag()

    init(wishbone: Wishbone, route: Route, navigator: Navigator) {
        self.wishbone = wishbone
        self.route = route
        self.navigator = navigator
    }

    deinit { tasks.cancelAll() }

    var store: Store? { if case let .ready(s) = phase { s } else { nil } }

    func load() async {
        if case .failed = phase { phase = .loading }
        if let local = wishbone.localScreens[route.screen] {
            do { try apply(throwing: API.parseScreen(local)) } catch { fail(error) }
            return
        }
        // the server's id rule: anything else (e.g. `..` from a deeplink) would resolve to another path
        guard API.isScreenID(route.screen) else {
            return fail(WishboneError.server(status: 404, code: "SCREEN_NOT_FOUND", message: "invalid screen id \"\(route.screen)\"", retryable: false))
        }
        let key = DiskCache.screenKey(scope: wishbone.cacheScope, id: route.screen, params: route.params)
        if cached == nil, let data = wishbone.cache.read(key), let r = try? API.parseScreen(data), r.document != nil,
           (try? apply(throwing: r)) != nil {
            cached = r
        }
        do {
            let req = API.screen(await wishbone.requestInfo(), id: route.screen, params: route.params, etag: cached?.etag)
            let (body, _) = try await API.send(req)
            var fresh = try API.parseScreen(body ?? Data())
            if fresh.document == nil, var c = cached {
                (c.etag, c.data, c.rawData) = (fresh.etag, fresh.data, fresh.rawData)
                store?.update(data: c.data)
                fresh = c
            } else {
                try apply(throwing: fresh)
            }
            cached = fresh
            wishbone.cache.write(key, try API.encodeScreen(fresh))
        } catch {
            wishbone.log("warn", "screen \(route.screen): \(error)")
            if cached == nil { fail(error) }
        }
    }

    /// Renders an answer (preview document, cache, network); state of the previous store survives.
    func apply(_ r: ScreenResponse) {
        do { try apply(throwing: r) } catch { fail(error) }
    }

    private func apply(throwing r: ScreenResponse) throws {
        guard let json = r.document else { throw WishboneError.decode("no document") }
        let doc = try Document(json: json)
        // In-flight handlers of the replaced store may finish (their effects still run); runs are only
        // cancelled when the screen itself goes away (deinit).
        phase = .ready(Store(document: doc, params: route.params, data: r.data, wishbone: wishbone, navigator: navigator,
                             previous: store, tasks: tasks))
    }

    private func fail(_ error: Error) {
        wishbone.log("error", "screen \(route.screen): \(error)")
        phase = .failed(String(describing: error))
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
