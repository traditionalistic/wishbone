import Observation
import SwiftUI
import WishboneCore

/// A server-driven screen with its own navigation stack (spec §6.3):
/// `WishboneScreen(wishbone, id: "showcase.promo", params: [:])`.
public struct WishboneScreen: View {
    let wishbone: Wishbone
    let root: Route

    public init(_ wishbone: Wishbone, id: String, params: [String: Value] = [:]) {
        self.wishbone = wishbone
        root = Route(screen: id, params: params)
    }

    public var body: some View {
        StackHost(wishbone: wishbone, root: root, presenter: nil)
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
    }
}

struct StackHost: View {
    let wishbone: Wishbone
    let root: Route
    @State private var navigator: Navigator
    @Environment(\.dismiss) private var dismiss

    init(wishbone: Wishbone, root: Route, presenter: Navigator?) {
        self.wishbone = wishbone
        self.root = root
        _navigator = State(initialValue: Navigator(presenter: presenter))
    }

    var body: some View {
        NavigationStack(path: $navigator.path) {
            RouteView(wishbone: wishbone, route: root, navigator: navigator)
                .navigationDestination(for: Route.self) { RouteView(wishbone: wishbone, route: $0, navigator: navigator) }
        }
        .sheet(item: $navigator.sheet) { route in
            StackHost(wishbone: wishbone, root: route, presenter: navigator)
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
        }
        .modifier(EnvReader(env: wishbone.env))
        .onAppear { navigator.exit = { dismiss() } }
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
        #if os(iOS)
        .toolbar(.hidden, for: .navigationBar)
        .background(SwipeBackEnabler().frame(width: 0, height: 0))
        #endif
    }
}

struct ScreenContent: View {
    let store: Store

    var body: some View {
        var ctx = RenderContext(store: store)
        ctx.isRoot = true
        return NodeView(node: store.document.root, ctx: ctx)
            .frame(maxWidth: .infinity, maxHeight: .infinity) // a wrap-sized root is centered
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

    init(wishbone: Wishbone, route: Route, navigator: Navigator) {
        self.wishbone = wishbone
        self.route = route
        self.navigator = navigator
    }

    var store: Store? { if case let .ready(s) = phase { s } else { nil } }

    func load() async {
        if case .failed = phase { phase = .loading }
        if let local = wishbone.localScreens[route.screen] {
            do { try apply(throwing: API.parseScreen(local)) } catch { fail(error) }
            return
        }
        let key = DiskCache.screenKey(brand: wishbone.brand, id: route.screen, params: route.params)
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
        phase = .ready(Store(document: doc, params: route.params, data: r.data, wishbone: wishbone, navigator: navigator, previous: store))
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
