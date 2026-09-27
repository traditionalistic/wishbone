import SwiftUI
import WishboneCore

/// Renders one wire node: `repeat` expands into one `NodeBody` per item (identity = `key`, else
/// index); each body evaluates `when`, props, events and a11y.
struct NodeView: View {
    let node: Node
    let ctx: RenderContext

    var body: some View {
        if let rep = node.rep {
            ForEach(RepeatItem.items(rep, ctx: ctx)) { item in
                NodeBody(node: node, ctx: item.ctx)
            }
        } else {
            NodeBody(node: node, ctx: ctx)
        }
    }
}

struct RepeatItem: Identifiable {
    let id: String
    let ctx: RenderContext

    @MainActor
    static func items(_ rep: RepeatBinding, ctx: RenderContext) -> [RepeatItem] {
        guard case let .list(values)? = ctx.eval(rep.over) else { return [] }
        var seen = Set<String>()
        return values.enumerated().map { i, v in
            var c = ctx
            c.locals = Locals(name: rep.asName, value: v, parent: Locals(name: rep.indexName, value: .number(Double(i)), parent: ctx.locals))
            var key = rep.key.flatMap { c.eval($0) }.map(keyString) ?? String(i)
            if !seen.insert(key).inserted { key += "#\(i)" }
            c.idSuffix += "." + key
            return RepeatItem(id: key, ctx: c)
        }
    }

    static func keyString(_ v: Value) -> String {
        if case let .string(s) = v { s } else { API.jsonString(v) }
    }
}

struct NodeBody: View {
    let node: Node
    let ctx: RenderContext

    static let builtins: Set<String> = ["stack", "scroll", "grid", "spacer", "text", "image", "icon", "shape", "textField", "native"]

    var body: some View {
        let ctx = scoped()
        if visible(ctx), let props = evaluate(ctx) {
            if node.type == "spacer" {
                if node.on.isEmpty {
                    SpacerNode(props: props, ctx: ctx)
                } else {
                    SpacerNode(props: props, ctx: ctx).modifier(EventModifier(node: node, ctx: ctx, corner: 0))
                }
            } else if Self.builtins.contains(node.type) || WishboneRenderers.registry[node.type] != nil {
                decorated(props: props, ctx: ctx)
            } else {
                let _ = ctx.store.warnOnce("no renderer for \(node.type) (\(node.id))")
            }
        }
    }

    private func scoped() -> RenderContext {
        guard !node.instanceState.isEmpty else { return ctx }
        var c = ctx
        c.instance = ctx.store.instance(node, ctx: ctx)
        return c
    }

    private func visible(_ ctx: RenderContext) -> Bool {
        guard let when = node.when else { return true }
        return ctx.eval(when) == .bool(true)
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
        let box = BoxProps(props)
        let dark = wb.env.dark
        return content(props: props, ctx: ctx)
            .modifier(BoxModifier(box: box, alignment: Align.frame(type: node.type, props: props), theme: wb.theme, dark: dark, extendsBackground: ctx.isRoot))
            .modifier(EventModifier(node: node, ctx: ctx, corner: box.corner))
            .modifier(A11yModifier(node: node, ctx: ctx))
            .modifier(OuterModifier(box: box, theme: wb.theme, dark: dark))
            .ignoresSafeArea(.container, edges: ctx.isRoot ? Self.edges(props.string("ignoreSafeArea")) : [])
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
    private func content(props: Props, ctx: RenderContext) -> some View {
        switch node.type {
        case "stack": StackNode(node: node, props: props, ctx: ctx)
        case "scroll": ScrollNode(node: node, props: props, ctx: ctx)
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

/// `tap` (a `Button`, so it neither blocks scrolling nor loses a11y), `longPress`, `appear`, `disappear`.
struct EventModifier: ViewModifier {
    let node: Node
    let ctx: RenderContext
    let corner: Double

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
                .onTapGesture { fire("tap") }
                .onLongPressGesture { fire("longPress") }
                .accessibilityElement(children: .combine)
                .accessibilityAddTraits(tap ? .isButton : [])
                .accessibilityAction(named: Text(verbatim: "longPress")) { fire("longPress") }
        } else if tap {
            Button { fire("tap") } label: { content.contentShape(BoxShape(corner: corner)) }
                .buttonStyle(NodeButtonStyle())
        } else {
            content
        }
    }

    private func fire(_ event: String) {
        guard let stmts = node.on[event] else { return }
        ctx.store.fire(stmts, ctx: ctx, event: .record([:]))
    }
}

/// Renders the label as is (press visuals come from the DSL via `self.pressed`, phase b).
struct NodeButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
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
