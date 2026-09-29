import SwiftUI
import WishboneCore

/// Inline server-driven content inside a host screen (spec 2026-09-28 §2): takes the width the host offers,
/// is as tall as its content, and is nothing at all (zero height, no spinner, no fallback) while loading
/// without a cache, when unpublished (its cache is dropped), failed without a cache, or when the root's
/// `when` is false. DSL `navigate` and `present` go to `onNavigate` (`present` with transition "present");
/// `back`/`dismiss` do nothing. On appearance it shows its last answer at once (from memory, else the disk cache) and
/// asks the server again unless that answer is less than 30 s old (`SlotMemo`). Content that comes or goes after the
/// first frame animates on `@motion.smooth` (fade + height from the top edge, so the host's rows below slide; reduce
/// motion: fade only); a cached slot is there in its first frame. At most `slotMaxHeight` tall: content below is clipped.
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
    /// Made once per slot (not on every re-render of the host's row).
    @StateObject private var once: Once<ScreenModel>
    /// The store on screen (nil: nothing), kept while it leaves. Set at once on appearance (a cached slot is there
    /// in its first frame); later comings and goings animate (`present`).
    @State private var shown: Store?
    /// Shown part of the content's height and its opacity, 0...1 (animated).
    @State private var height = 0.0
    @State private var opacity = 0.0
    /// `matchId` scope: the whole slot.
    @Namespace private var namespace

    init(wishbone: Wishbone, route: Route, onNavigate: @escaping (WishboneRoute) -> Void) {
        self.wishbone = wishbone
        self.route = route
        self.onNavigate = onNavigate
        _once = StateObject(wrappedValue: Once(Self.model(wishbone, route, onNavigate)))
    }

    private static func model(_ wishbone: Wishbone, _ route: Route, _ onNavigate: @escaping (WishboneRoute) -> Void) -> ScreenModel {
        let navigator = Navigator()
        // Wired before the first frame: a cached root's `appear` may navigate at once.
        navigator.forward = onNavigate
        navigator.forwardsPresent = true
        navigator.exit = {} // `back`/`dismiss` at the root: no-op
        return ScreenModel(wishbone: wishbone, route: route, navigator: navigator, slot: true)
    }

    private var model: ScreenModel { once.value }

    var body: some View {
        let visible = slotVisible(model.store) ? model.store : nil
        // A layout, not a Group: an empty Group never appears, so it would never load.
        Reveal(fraction: height) {
            // A leaving slot keeps its last content: its root's `when` may be false already, or the load failed.
            if let last = shown { root(visible ?? last) }
        }
        .opacity(opacity)
        .clipped()
        // `env.screenWidth` = the width the host offers; a content-sized root is centered in it (as on a screen).
        .frame(maxWidth: .infinity)
        .modifier(EnvReader(env: wishbone.env, geometry: model.navigator.geometry, inline: true))
        .onChange(of: visible.map(ObjectIdentifier.init)) { old, new in present(visible, animated: (old == nil) != (new == nil)) }
        .onAppear {
            wishbone.clock.retain()
            if model.store == nil { model.showCached() }
            present(slotVisible(model.store) ? model.store : nil, animated: false)
        }
        .task { await model.load() }
        .onDisappear { wishbone.clock.release() }
    }

    private var motion: Animation { wishbone.theme.animation(.record(["$motion": .string("smooth")])) ?? .easeInOut }

    /// Appearing/leaving: fade + the height growing/shrinking from the top edge; the host's rows below follow the
    /// slot's height frame by frame. Reduce motion: fade only, the rows jump (a leaving slot fades in place first).
    /// The content stays in the tree while it leaves (a removal transition in a lazy host row drops it at once).
    private func present(_ store: Store?, animated: Bool) {
        let target = store == nil ? 0.0 : 1
        if let store { shown = store }
        guard animated else {
            (height, opacity) = (target, target)
            if store == nil { shown = nil }
            return
        }
        let reduce = wishbone.env.reduceMotion
        if reduce, store != nil { height = 1 }
        withAnimation(motion) {
            opacity = target
            if !reduce { height = target }
        } completion: {
            guard store == nil, opacity == 0 else { return } // shown again meanwhile
            height = 0
            shown = nil
        }
    }

    private func root(_ store: Store) -> some View {
        var ctx = RenderContext(store: store)
        ctx.namespace = namespace // not `isRoot`: the root's background stays inside the slot
        // The slot shows and hides its root itself (`slotVisible`, animated): the root renders without its `when`.
        let r = store.document.root
        let root = Node(type: r.type, id: r.id, props: r.props, children: r.children, rep: r.rep, on: r.on, animate: r.animate,
                        transitionIn: r.transitionIn, transitionOut: r.transitionOut, match: r.match, drag: r.drag,
                        instanceState: r.instanceState, a11y: r.a11y)
        return NodeView(node: root, ctx: ctx)
    }
}

/// A reference made once per view identity: `@StateObject` builds it lazily, the first time SwiftUI installs the view,
/// where `@State(initialValue:)` would build (and throw away) one on every re-render of the parent.
@MainActor
final class Once<Value: AnyObject>: ObservableObject {
    let value: Value
    init(_ value: Value) { self.value = value }
}

/// The slot has content: a store whose root's `when` is absent or true — read in the root's own scope, as `NodeBody`
/// reads it (a template root's `when` may read its instance state; the same instance the root then renders with).
@MainActor
func slotVisible(_ store: Store?) -> Bool {
    guard let store else { return false }
    let root = store.document.root
    return RenderContext(store: store).scoped(to: root).shows(root)
}

/// A slot is a row of the host's feed, often a lazy one, where a row taller than the screen can livelock SwiftUI's
/// `LazyVStack` (decisions/renderers.md): a slot is at most this tall, content below is clipped (Android: the same).
let slotMaxHeight: CGFloat = 480

/// Reports `fraction` of its content's height, capped at `slotMaxHeight`, with the content pinned to the top (the
/// caller clips): the slot's height animates like Compose's `expandVertically(expandFrom = Top)` /
/// `shrinkVertically(shrinkTowards = Top)`.
struct Reveal: Layout {
    var fraction: Double

    var animatableData: Double {
        get { fraction }
        set { fraction = newValue }
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let size = subviews.first?.sizeThatFits(Self.content(proposal)) ?? .zero
        return CGSize(width: size.width, height: Self.height(content: size.height, fraction: fraction))
    }

    /// A spring may overshoot below 0: never a negative height.
    static func height(content: CGFloat, fraction: Double) -> CGFloat { min(content, slotMaxHeight) * max(0, fraction) }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        subviews.first?.place(at: bounds.origin, proposal: Self.content(proposal))
    }

    /// The content is as tall as it wants (as in a lazy feed row, and as on Android); the cap clips it.
    private static func content(_ proposal: ProposedViewSize) -> ProposedViewSize { ProposedViewSize(width: proposal.width, height: nil) }
}

/// Ruling R1: a `Wishbone`'s last network answer per slot cache key (screen id + params) and when it came. A slot
/// scrolling back into a host's feed renders it at once (no disk read or decode) and asks the server again only once
/// it is `freshFor` old; `SCREEN_NOT_FOUND` removes it (with the disk cache).
/// ponytail: unbounded — one entry per distinct slot id + params shown; cap it if a host varies params per row.
@MainActor
final class SlotMemo {
    static let freshFor: Duration = .seconds(30)
    private var entries: [String: (screen: LoadedScreen, loadedAt: ContinuousClock.Instant)] = [:]
    private let now: @Sendable () -> ContinuousClock.Instant

    init(now: @escaping @Sendable () -> ContinuousClock.Instant) {
        self.now = now
    }

    subscript(key: String) -> LoadedScreen? { entries[key]?.screen }

    /// The last network answer for `key` came less than `freshFor` ago.
    func isFresh(_ key: String) -> Bool {
        entries[key].map { now() - $0.loadedAt < Self.freshFor } ?? false
    }

    func put(_ key: String, _ screen: LoadedScreen) { entries[key] = (screen, now()) }

    func remove(_ key: String) { entries[key] = nil }
}
