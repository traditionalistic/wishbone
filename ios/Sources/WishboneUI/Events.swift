import SwiftUI
import WishboneCore

/// `tap` (a `Button`, so it neither blocks scrolling nor loses a11y), `longPress`, `appear`, `disappear`.
struct EventModifier: ViewModifier {
    let node: Node
    let ctx: RenderContext
    let corner: Double
    let onPress: (Bool) -> Void

    func body(content: Content) -> some View {
        events(content)
            .onAppear { fire("appear") }
            .onDisappear { fire("disappear") }
    }

    @ViewBuilder
    private func events(_ content: Content) -> some View {
        let tap = node.on["tap"] != nil, long = node.on["longPress"] != nil
        if long {
            content
                .contentShape(BoxShape(corner: corner))
                .modifier(TapIfNeeded(enabled: tap) { fire("tap") })
                .onLongPressGesture { fire("longPress") }
                .accessibilityElement(children: .combine)
                .accessibilityAddTraits(tap ? .isButton : [])
                .accessibilityAction(named: Text(verbatim: "longPress")) { fire("longPress") }
        } else if tap {
            Button { fire("tap") } label: { content.contentShape(BoxShape(corner: corner)) }
                .buttonStyle(NodeButtonStyle(onPress: onPress))
        } else {
            content
        }
    }

    private func fire(_ event: String) {
        guard let stmts = node.on[event] else { return }
        ctx.store.fire(stmts, ctx: ctx, event: .record([:]))
    }
}

/// A long-press node without `tap` must not swallow its ancestors' taps.
struct TapIfNeeded: ViewModifier {
    let enabled: Bool
    let action: () -> Void
    func body(content: Content) -> some View {
        if enabled { content.onTapGesture(perform: action) } else { content }
    }
}

/// Renders the label as is and reports `isPressed` (press visuals come from the DSL via `self.pressed`).
struct NodeButtonStyle: ButtonStyle {
    let onPress: (Bool) -> Void

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .onChange(of: configuration.isPressed) { _, pressed in onPress(pressed) }
    }
}

/// `drag: {axis, snap, bounds}`: the node follows the finger (per materialized instance), springs to
/// the nearest snap point on release, then fires `dragEnd {x, y, vx, vy}` (translation at release).
struct DragModifier: ViewModifier {
    let node: Node
    let ctx: RenderContext
    let spec: DragSpec
    /// nil until the first move decides whether this gesture is ours (axis lock), then true/false.
    @State private var claimed: Bool?

    func body(content: Content) -> some View {
        let key = "drag:\(node.id)\(ctx.idSuffix)"
        let x = ctx.store.driver(key + ":x"), y = ctx.store.driver(key + ":y")
        content
            .simultaneousGesture(
                DragGesture(minimumDistance: 8, coordinateSpace: .global)
                    .onChanged { v in
                        if claimed == nil { claimed = spec.claims(v.translation) }
                        guard claimed == true else { return }
                        let t = spec.follow(v.translation)
                        x.value = .number(t.width)
                        y.value = .number(t.height)
                    }
                    .onEnded { v in
                        defer { claimed = nil }
                        guard claimed == true else { return }
                        let t = spec.follow(v.translation)
                        let theme = ctx.store.wishbone.theme
                        withAnimation(theme.animation(.record(["$motion": .string("interactive")])) ?? .spring) {
                            x.value = .number(spec.movesX ? spec.rest(t.width) : 0)
                            y.value = .number(spec.movesY ? spec.rest(t.height) : 0)
                        }
                        guard let stmts = node.on["dragEnd"] else { return }
                        let event: Value = .record([
                            "x": .number(t.width), "y": .number(t.height),
                            "vx": .number(v.velocity.width), "vy": .number(v.velocity.height),
                        ])
                        ctx.store.fire(stmts, ctx: ctx, event: event)
                    }
            )
            .onDisappear {
                // An instance that leaves the tree comes back at rest.
                x.value = .number(0)
                y.value = .number(0)
            }
    }
}

/// Identifier `wb.<screen>.<nodeId>[.<repeatKey>…]`, `a11y.label/role/hidden`; containers keep
/// their children visible to UI tests.
struct A11yModifier: ViewModifier {
    let node: Node
    let ctx: RenderContext

    func body(content: Content) -> some View {
        let id = "wb.\(ctx.store.document.screen).\(node.id)\(ctx.idSuffix)"
        let label: String? = if case let .string(s)? = node.a11y?.label.flatMap({ ctx.eval($0) }) { s } else { nil }
        let hidden = node.a11y?.hidden.flatMap { ctx.eval($0) } == .bool(true)
        let container = !node.children.isEmpty && node.on["tap"] == nil && node.on["longPress"] == nil
        if hidden {
            content.accessibilityHidden(true).accessibilityIdentifier(id)
        } else {
            grouped(content, container: container, labelled: label != nil)
                .accessibilityIdentifier(id)
                .modifier(LabelModifier(label: label))
                .accessibilityAddTraits(Self.traits(node.a11y?.role))
        }
    }

    @ViewBuilder
    private func grouped(_ content: Content, container: Bool, labelled: Bool) -> some View {
        if container && labelled {
            content.accessibilityElement(children: .combine)
        } else if container {
            content.accessibilityElement(children: .contain)
        } else {
            content
        }
    }

    static func traits(_ role: String?) -> AccessibilityTraits {
        switch role {
        case "button": .isButton
        case "header": .isHeader
        case "image": .isImage
        default: []
        }
    }
}

struct LabelModifier: ViewModifier {
    let label: String?
    func body(content: Content) -> some View {
        if let label { content.accessibilityLabel(Text(verbatim: label)) } else { content }
    }
}
