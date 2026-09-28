import SwiftUI
import WishboneCore

/// Inline server-driven content inside a host screen (spec 2026-09-28 §2): takes the width the host offers,
/// is as tall as its content, and is nothing at all (zero height, no spinner, no fallback) while loading
/// without a cache, when unpublished (its cache is dropped), failed without a cache, or when the root's
/// `when` is false. DSL `navigate` and `present` go to `onNavigate` (`present` with transition "present");
/// `back`/`dismiss` do nothing. Loads on appearance (cache at once, then network); the host forces a
/// reload by changing the view's identity.
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
        // A different id/params is a different slot: fresh model and store.
        SlotHost(wishbone: wishbone, route: route, onNavigate: onNavigate)
            .id("\(route.screen)|\(API.jsonString(.record(route.params)))")
    }
}

struct SlotHost: View {
    let wishbone: Wishbone
    let route: Route
    let onNavigate: (WishboneRoute) -> Void
    @State private var navigator: Navigator
    @State private var model: ScreenModel
    /// `matchId` scope: the whole slot.
    @Namespace private var namespace

    init(wishbone: Wishbone, route: Route, onNavigate: @escaping (WishboneRoute) -> Void) {
        self.wishbone = wishbone
        self.route = route
        self.onNavigate = onNavigate
        let navigator = Navigator()
        _navigator = State(initialValue: navigator)
        _model = State(initialValue: ScreenModel(wishbone: wishbone, route: route, navigator: navigator, slot: true))
    }

    var body: some View {
        // A VStack, not a Group: an empty Group never appears, so it would never load.
        VStack(spacing: 0) {
            if let store = model.store { root(store) }
        }
        // `env.screenWidth` = the width the host offers (as on Android); a content-sized root sits top-leading.
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .modifier(EnvReader(env: wishbone.env, geometry: navigator.geometry, inline: true))
        .task {
            navigator.forward = onNavigate
            navigator.forwardsPresent = true
            navigator.exit = {} // `back`/`dismiss` at the root: no-op
            await model.load()
        }
        .onAppear { wishbone.clock.retain() }
        .onDisappear { wishbone.clock.release() }
    }

    private func root(_ store: Store) -> some View {
        var ctx = RenderContext(store: store)
        ctx.namespace = namespace // not `isRoot`: the root's background stays inside the slot
        return NodeView(node: store.document.root, ctx: ctx)
    }
}
