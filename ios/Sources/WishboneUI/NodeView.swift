import SwiftUI
import WishboneCore

/// Renders one wire node: `repeat` expands into one `NodeBody` per item (identity = `key`, else
/// index); each body evaluates `when`, props, events and a11y.
struct NodeView: View {
    let node: Node
    let ctx: RenderContext

    var body: some View {
        if let rep = node.rep {
            let items = RepeatItem.items(rep, id: node.id, ctx: ctx)
            let fallback = NodeBody.transitionFallback(node, ctx: ctx)
            let keys = fallback == nil ? [] : items.map(\.id)
            ForEach(items) { item in
                NodeBody(node: node, ctx: item.ctx)
                    .modifier(WhenTransaction(visible: keys, fallback: fallback), if: fallback != nil)
            }
        } else {
            NodeBody(node: node, ctx: ctx)
        }
    }
}

struct RepeatItem: Identifiable {
    let id: String
    let ctx: RenderContext

    /// `id` — the repeat node's. An item whose value and index are unchanged under the same parent locals keeps its
    /// previous `Locals` (by key, `Store.repeatLocals`): its row's inputs then compare equal and SwiftUI skips it.
    @MainActor
    static func items(_ rep: RepeatBinding, id: String, ctx: RenderContext) -> [RepeatItem] {
        guard case let .list(values)? = ctx.eval(rep.over) else { return [] }
        let repeatID = id + ctx.idSuffix
        let previous = ctx.store.repeatLocals[repeatID] ?? [:]
        var kept: [String: Locals] = Dictionary(minimumCapacity: values.count)
        var seen = Set<String>()
        let items = values.enumerated().map { i, v in
            var c = ctx
            let index: Value = .number(Double(i))
            c.locals = Locals(name: rep.asName, value: v, parent: Locals(name: rep.indexName, value: index, parent: ctx.locals))
            var key = rep.key.flatMap { c.eval($0) }.map(keyString) ?? String(i)
            if !seen.insert(key).inserted { key += "#\(i)" }
            if let old = previous[key], old.value == v, let oldIndex = old.parent, oldIndex.value == index, oldIndex.parent === ctx.locals {
                c.locals = old
            }
            kept[key] = c.locals
            c.idSuffix += "." + key
            c.repeatIndex = i
            return RepeatItem(id: key, ctx: c)
        }
        ctx.store.repeatLocals[repeatID] = kept // keys no longer present drop out
        return items
    }

    static func keyString(_ v: Value) -> String {
        if case let .string(s) = v { s } else { API.jsonString(v) }
    }
}

struct NodeBody: View {
    let node: Node
    let ctx: RenderContext
    /// Pressed state of this node's tap button (`self.pressed`).
    @State private var pressed = false

    nonisolated static let builtins: Set<String> = ["stack", "scroll", "grid", "spacer", "text", "image", "icon", "shape", "textField", "native"]

    var body: some View {
        let ctx = scoped()
        let visible = ctx.shows(node)
        Group {
            if visible, let props = evaluate(ctx) {
                rendered(props: props, ctx: ctx)
                    .transition(transition(ctx))
            }
        }
        .modifier(WhenTransaction(visible: visible, fallback: whenFallback(ctx)), if: node.when != nil)
    }

    @ViewBuilder
    private func rendered(props: Props, ctx: RenderContext) -> some View {
        if node.type == "spacer" {
            if node.on.isEmpty {
                SpacerNode(props: props, ctx: ctx)
            } else {
                SpacerNode(props: props, ctx: ctx).modifier(EventModifier(node: node, ctx: ctx, corner: 0) { pressed = $0 })
            }
        } else if Self.builtins.contains(node.type) || WishboneRenderers.registry[node.type] != nil {
            decorated(props: props, ctx: ctx)
        } else {
            let _ = ctx.store.warnOnce("no renderer for \(node.type) (\(node.id))")
        }
    }

    /// A node without `transition` (the value it had when built per render).
    private static let noTransition = AnyTransition.asymmetric(insertion: .identity, removal: .identity)

    /// Enter/exit of `when` nodes and repeat items; reduce motion keeps only fades.
    private func transition(_ ctx: RenderContext) -> AnyTransition {
        guard node.transitionIn != nil || node.transitionOut != nil else { return Self.noTransition }
        let reduce = ctx.store.wishbone.env.reduceMotion
        func side(_ wire: Value?) -> AnyTransition {
            let parts = TransitionPart.parts(wire)
            return TransitionPart.transition(reduce ? TransitionPart.reduced(parts) : parts)
        }
        return .asymmetric(insertion: side(node.transitionIn), removal: side(node.transitionOut))
    }

    /// A `when` flip outside a transaction still animates: the node's `animate["*"]`, else @motion.smooth.
    private func whenFallback(_ ctx: RenderContext) -> Animation? {
        node.when == nil ? nil : Self.transitionFallback(node, ctx: ctx)
    }

    /// Motion of an enter/exit that isn't inside `animate(m) {}` (nil when the node has no transition).
    static func transitionFallback(_ node: Node, ctx: RenderContext) -> Animation? {
        guard node.transitionIn != nil || node.transitionOut != nil else { return nil }
        let theme = ctx.store.wishbone.theme
        return node.animate["*"].flatMap { ctx.eval($0) }.flatMap(theme.animation)
            ?? theme.animation(.record(["$motion": .string("smooth")])) ?? .easeInOut
    }

    private func implicit(_ ctx: RenderContext) -> Motion.Implicit {
        var m = Motion.Implicit()
        let theme = ctx.store.wishbone.theme
        for (prop, expr) in node.animate {
            if let a = ctx.eval(expr).flatMap(theme.animation) { m.specs[prop] = a }
        }
        return m
    }

    private func scoped() -> RenderContext {
        var c = ctx
        c.nodeID = node.id
        if !node.instanceState.isEmpty { c.instance = ctx.store.instance(node, ctx: ctx) }
        c.pressed = node.on["tap"] != nil && pressed
        return c
    }

    private func evaluate(_ ctx: RenderContext) -> Props? {
        var values: [String: Value] = [:]
        for (key, expr) in node.props {
            guard let v = ctx.eval(expr) else { return nil }
            values[key] = v
        }
        return Props(type: node.type, values: values)
    }

    private func decorated(props: Props, ctx: RenderContext) -> some View {
        let wb = ctx.store.wishbone
        var box = BoxProps(props)
        // `scroll` padding pads the scrolled content, not the viewport.
        let scrollPadding = node.type == "scroll" ? box.padding : Insets(.null)
        if node.type == "scroll" { box.padding = Insets(.null) }
        let dark = wb.env.dark
        let anim = implicit(ctx)
        let match = node.match.flatMap { ctx.eval($0) }.map(RepeatItem.keyString)
        let layers = BoxLayers(node, timelineTarget: ctx.store.timelines.isTarget(node.id))
        return content(props: props, ctx: ctx, scrollPadding: scrollPadding)
            .modifier(ContentAnimation(animation: anim.any("color", "fill", "progress"), value: [props["color"], props["fill"], props["progress"]]),
                      if: !node.animate.isEmpty)
            .modifier(BoxModifier(box: box, alignment: Align.frame(type: node.type, props: props), theme: wb.theme, dark: dark,
                                  extendsBackground: ctx.isRoot, anim: anim, match: match.flatMap { m in ctx.namespace.map { (m, $0) } },
                                  layers: layers))
            .modifier(EventModifier(node: node, ctx: ctx, corner: box.corner) { pressed = $0 }, if: !node.on.isEmpty)
            .modifier(A11yModifier(node: node, ctx: ctx))
            .modifier(TransformModifier(node: node, ctx: ctx, box: box, anim: anim, theme: wb.theme, dark: dark, layers: layers))
    }

    /// `ignoreSafeArea` — honored on the screen root only.
    static func edges(_ s: String?) -> Edge.Set {
        switch s {
        case "top": .top
        case "bottom": .bottom
        case "all": .all
        default: []
        }
    }

    @ViewBuilder
    private func content(props: Props, ctx: RenderContext, scrollPadding: Insets) -> some View {
        switch node.type {
        case "stack": StackNode(node: node, props: props, ctx: ctx)
        case "scroll": ScrollNode(node: node, props: props, ctx: ctx, padding: scrollPadding)
        case "grid": GridNode(node: node, props: props, ctx: ctx)
        case "text": TextNode(props: props, ctx: ctx)
        case "image": ImageNode(props: props, ctx: ctx)
        case "icon": IconNode(props: props, ctx: ctx)
        case "shape": ShapeNode(props: props, ctx: ctx)
        case "textField": TextFieldNode(node: node, props: props, ctx: ctx)
        case "native": NativeNode(props: props, ctx: ctx)
        default:
            if let make = WishboneRenderers.registry[node.type] {
                make(WishboneNodeContext(props: props.values, baseURL: ctx.store.wishbone.baseURL) { event, payload in
                    if let stmts = node.on[event] { ctx.store.fire(stmts, ctx: ctx, event: payload) }
                })
            }
        }
    }
}
