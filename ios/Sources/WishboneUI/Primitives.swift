import SwiftUI
import WishboneCore
#if canImport(UIKit)
import UIKit
#endif

/// Children of a container node, each with the container's axis in its context.
struct Children: View {
    let node: Node
    let ctx: RenderContext
    let axis: String?

    var body: some View {
        ForEach(Positioned(nodes: node.children), id: \.element.id) { i, child in
            NodeView(node: child, ctx: ctx.child(axis: axis, sibling: i))
        }
    }
}

/// Nodes with their positions, without copying them into an array on every render.
struct Positioned: RandomAccessCollection {
    let nodes: [Node]
    var startIndex: Int { 0 }
    var endIndex: Int { nodes.count }
    subscript(i: Int) -> (offset: Int, element: Node) { (i, nodes[i]) }
}

struct StackNode: View {
    let node: Node
    let props: Props
    let ctx: RenderContext

    var body: some View {
        let spacing = props.number("spacing") ?? 0, align = props.string("align")
        switch props.string("axis") {
        case "h": HStack(alignment: Align.vertical(align), spacing: spacing) { Children(node: node, ctx: ctx, axis: "h") }
        case "z": ZStack(alignment: Align.z(align)) { Children(node: node, ctx: ctx, axis: "z") }
        default: VStack(alignment: Align.horizontal(align), spacing: spacing) { Children(node: node, ctx: ctx, axis: "v") }
        }
    }
}

struct ScrollNode: View {
    let node: Node
    let props: Props
    let ctx: RenderContext
    /// The node's `padding`, applied to the scrolled content.
    let padding: Insets

    var body: some View {
        let horizontal = props.string("axis") == "h"
        let cells = (x: ctx.store.driver("scroll:\(node.id):x"), y: ctx.store.driver("scroll:\(node.id):y"))
        ScrollView(horizontal ? .horizontal : .vertical, showsIndicators: props.bool("indicators")) {
            content(horizontal: horizontal)
        }
        .modifier(PagingModifier(paging: props.bool("paging")))
        .onScrollGeometryChange(for: CGPoint.self) { geo in
            ScrollDriver.offset(contentOffset: geo.contentOffset, insets: geo.contentInsets)
        } action: { _, offset in
            // scroll.<id>.x|y cells; nodes reading them re-render every frame, nothing else does.
            if cells.x.value != .number(offset.x) { cells.x.value = .number(offset.x) }
            if cells.y.value != .number(offset.y) { cells.y.value = .number(offset.y) }
        }
    }

    @ViewBuilder
    private func content(horizontal: Bool) -> some View {
        let spacing = props.number("spacing") ?? 0, align = props.string("align"), lazy = props.bool("lazy")
        if horizontal {
            let a = Align.vertical(align)
            Group {
                if lazy {
                    LazyHStack(alignment: a, spacing: spacing) { Children(node: node, ctx: ctx, axis: "h") }
                } else {
                    HStack(alignment: a, spacing: spacing) { Children(node: node, ctx: ctx, axis: "h") }
                }
            }
            .padding(padding.edgeInsets)
        } else {
            let a = Align.horizontal(align)
            Group {
                if lazy {
                    LazyVStack(alignment: a, spacing: spacing) { Children(node: node, ctx: ctx, axis: "v") }
                } else {
                    VStack(alignment: a, spacing: spacing) { Children(node: node, ctx: ctx, axis: "v") }
                }
            }
            .padding(padding.edgeInsets)
            .frame(maxWidth: .infinity, alignment: Alignment(horizontal: a, vertical: .top))
        }
    }
}

struct PagingModifier: ViewModifier {
    let paging: Bool
    func body(content: Content) -> some View {
        if paging { content.scrollTargetBehavior(.paging) } else { content }
    }
}

struct GridNode: View {
    let node: Node
    let props: Props
    let ctx: RenderContext

    var body: some View {
        let columns = props.count("columns", in: 1...64) ?? 1
        let item = GridItem(.flexible(), spacing: props.number("spacing") ?? 0, alignment: .center)
        LazyVGrid(columns: Array(repeating: item, count: columns), alignment: .leading, spacing: props.number("rowSpacing") ?? 0) {
            Children(node: node, ctx: ctx, axis: nil)
        }
    }
}

/// `spacer` takes no box modifiers: it must stay a direct stack child to stretch along its axis.
struct SpacerNode: View {
    let props: Props
    let ctx: RenderContext

    var body: some View {
        let min = props.length("min") ?? 0
        let w = Dim(props["width"]).fixed, h = Dim(props["height"]).fixed
        if ctx.parentAxis == "v" || ctx.parentAxis == "h" {
            if w == nil && h == nil {
                Spacer(minLength: min) // unwrapped: a framed Spacer shares space like any flexible view
            } else {
                Spacer(minLength: min).frame(width: w, height: h)
            }
        } else {
            Color.clear.frame(width: w ?? min, height: h ?? min)
        }
    }
}

struct TextNode: View {
    let props: Props
    let ctx: RenderContext

    var body: some View {
        let wb = ctx.store.wishbone
        let style = wb.theme.textStyle(props["style"])
        let font = style.map(Fonts.resolved)
        let maxLines = props.count("maxLines", in: 0...10_000) ?? 0
        Text(verbatim: props.string("text") ?? "")
            .font(font?.font ?? .body)
            .tracking(style?.tracking ?? 0)
            .lineSpacing(font?.lineSpacing ?? 0)
            .foregroundStyle(wb.theme.color(props["color"], dark: wb.env.dark)?.color ?? .primary)
            .multilineTextAlignment(Self.textAlignment(props.string("textAlign")))
            .lineLimit(maxLines > 0 ? maxLines : nil)
    }

    static func textAlignment(_ s: String?) -> TextAlignment {
        switch s {
        case "center": .center
        case "end": .trailing
        default: .leading
        }
    }
}

struct ImageNode: View {
    let props: Props
    let ctx: RenderContext
    /// The picture this view loaded; kept even if the shared cache evicts it.
    @State private var loaded: (url: URL, image: PlatformImage)?

    var body: some View {
        let wb = ctx.store.wishbone
        let placeholder = wb.theme.color(props["placeholder"], dark: wb.env.dark)?.color ?? .clear
        let url = props.string("url").flatMap { Wishbone.resolve($0, base: wb.baseURL) }
        let fit = props.string("fit") == "fit"
        let image = url.flatMap { loaded?.url == $0 ? loaded?.image : ImageLoader.cached($0) }
        Group {
            if let image {
                Image(platformImage: image).resizable().aspectRatio(contentMode: fit ? .fit : .fill)
            } else {
                Rectangle().fill(placeholder)
            }
        }
        .clipped()
        .task(id: url) {
            // Runs on every appearance and is cancelled on disappear, so a load a lazy container cut off
            // is retried when the cell comes back; a failed load leaves the placeholder.
            guard let url, loaded?.url != url else { return }
            if let image = try? await ImageLoader.load(url) { loaded = (url, image) }
        }
    }
}

struct IconNode: View {
    let props: Props
    let ctx: RenderContext

    var body: some View {
        let wb = ctx.store.wishbone
        let size = props.length("size") ?? 24
        if case let .record(r) = props["name"], case let .string(name)? = r["$icon"], let symbol = Icons.symbols[name] {
            // ~1/12 inset: SF Symbols have no built-in padding, the Android 24dp drawables do.
            Image(systemName: symbol)
                .resizable()
                .scaledToFit()
                .padding(size / 12)
                .frame(width: size, height: size)
                .foregroundStyle(wb.theme.color(props["color"], dark: wb.env.dark)?.color ?? .primary)
        } else {
            let _ = ctx.store.warnOnce("unknown icon \(props["name"])")
        }
    }
}

struct ShapeNode: View {
    let props: Props
    let ctx: RenderContext

    var body: some View {
        switch props.string("kind") {
        case "circle": shaped(Circle())
        case "capsule": shaped(Capsule())
        default: shaped(BoxShape(corner: props.length("corner") ?? 0))
        }
    }

    private func shaped<S: InsettableShape>(_ shape: S) -> some View {
        let wb = ctx.store.wishbone, dark = wb.env.dark
        let stroke: (Color, Double)? = if case let .record(s) = props["stroke"], let c = s["color"].flatMap({ wb.theme.color($0, dark: dark) }),
                                          case let .number(w)? = s["width"], w.isFinite, w > 0 { (c.color, w) } else { nil }
        return PaintFill(paint: props.values["fill"].flatMap { wb.theme.paint($0, dark: dark) }, shape: shape)
            .overlay { if let stroke { shape.strokeBorder(stroke.0, lineWidth: stroke.1) } }
    }
}

struct NativeNode: View {
    let props: Props
    let ctx: RenderContext

    var body: some View {
        let args: [String: Any] = if case let .record(r) = props["props"] { r.mapValues(\.foundation) } else { [:] }
        if let view = ctx.store.wishbone.host.nativeView(name: props.string("name") ?? "", props: args) {
            view
        }
    }
}

extension Fonts {
    @MainActor private static var memo: [TypeStyle: (font: Font, lineSpacing: CGFloat)] = [:]

    /// A text style's font and extra line spacing, made once per style (a theme has a handful).
    @MainActor static func resolved(_ style: TypeStyle) -> (font: Font, lineSpacing: CGFloat) {
        if let known = memo[style] { return known }
        let made = (font(style), extraLineSpacing(style))
        memo[style] = made
        return made
    }

    /// Extra spacing between lines so a line box is `lineHeight` tall (0 when unset or smaller).
    static func extraLineSpacing(_ style: TypeStyle) -> CGFloat {
        guard let lh = style.lineHeight else { return 0 }
        #if canImport(UIKit)
        let natural = postScriptName(family: style.family, weight: style.weight)
            .flatMap { UIFont(name: $0, size: style.size)?.lineHeight } ?? UIFont.systemFont(ofSize: style.size).lineHeight
        return max(0, lh - natural)
        #else
        return max(0, lh - style.size * 1.2)
        #endif
    }
}
