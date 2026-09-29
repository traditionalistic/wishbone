import SwiftUI
import WishboneCore

extension RGBA {
    var color: Color { Color(.sRGB, red: r, green: g, blue: b, opacity: a) }
}

/// Fills `shape` with a paint; gradients follow CSS angle semantics for the actual box size.
struct PaintFill<S: Shape>: View {
    let paint: Paint?
    let shape: S

    var body: some View {
        switch paint {
        case let .color(c)?:
            shape.fill(c.color)
        case let .gradient(stops, angle)?:
            GeometryReader { g in
                let pts = Align.gradientPoints(angle: angle, size: g.size)
                shape.fill(LinearGradient(colors: stops.map(\.color), startPoint: pts.start, endPoint: pts.end))
            }
        case nil:
            shape.fill(Color.clear)
        }
    }
}

extension View {
    /// Applies `modifier` when `present` (built only then), animating its changes with `animation` when the node
    /// declares one (scoped, so it never animates anything else; no spec = the modifier follows the transaction).
    /// `present` must be static per node (a declared prop, never an evaluated value): it keeps the view's identity.
    @ViewBuilder
    func animated<M: ViewModifier>(_ animation: @autoclosure () -> Animation?, _ modifier: @autoclosure () -> M,
                                   if present: Bool = true) -> some View {
        if !present {
            self
        } else if let animation = animation() {
            self.animation(animation) { $0.modifier(modifier()) }
        } else {
            self.modifier(modifier())
        }
    }

    /// `modifier` only when `present` (static per node, see `animated`), built only then.
    @ViewBuilder
    func modifier<M: ViewModifier>(_ modifier: @autoclosure () -> M, if present: Bool) -> some View {
        if present { self.modifier(modifier()) } else { self }
    }
}

/// The box-model layers a node needs: those whose props it declares. An undeclared prop has its default, which its
/// layer would apply as a no-op; declared keys are static per node, so skipping the rest keeps view identity stable.
struct BoxLayers: OptionSet {
    let rawValue: UInt16

    static let padding = BoxLayers(rawValue: 1 << 0)
    /// `BoxFrame`: some size prop (wrap × wrap without min/max/aspect sizes the node as its content), or a primitive
    /// that may render nothing (`icon`, `native`, adapters): the frame keeps it a zero-size stack child, as always.
    static let frame = BoxLayers(rawValue: 1 << 1)
    static let background = BoxLayers(rawValue: 1 << 2)
    static let border = BoxLayers(rawValue: 1 << 3)
    static let shadow = BoxLayers(rawValue: 1 << 4)
    static let opacity = BoxLayers(rawValue: 1 << 5)
    static let scale = BoxLayers(rawValue: 1 << 6)
    static let rotation = BoxLayers(rawValue: 1 << 7)
    static let offset = BoxLayers(rawValue: 1 << 8)
    /// A timeline target or a `drag` node: per-frame transforms on top of the props.
    static let motion = BoxLayers(rawValue: 1 << 9)

    init(rawValue: UInt16) { self.rawValue = rawValue }

    init(_ node: Node, timelineTarget: Bool) {
        let p = node.props
        var l: BoxLayers = []
        if p["padding"] != nil, node.type != "scroll" { l.insert(.padding) } // a scroll pads its content (ScrollNode)
        if p["width"] != nil || p["height"] != nil || p["minWidth"] != nil || p["maxWidth"] != nil
            || p["minHeight"] != nil || p["maxHeight"] != nil || p["aspect"] != nil
            || node.type == "icon" || node.type == "native" || !NodeBody.builtins.contains(node.type) { l.insert(.frame) }
        if p["background"] != nil || p["material"] != nil { l.insert(.background) }
        if p["border"] != nil { l.insert(.border) }
        if p["shadow"] != nil { l.insert(.shadow) }
        if p["opacity"] != nil { l.insert(.opacity) }
        if p["scale"] != nil { l.insert(.scale) }
        if p["rotation"] != nil { l.insert(.rotation) }
        if p["offsetX"] != nil || p["offsetY"] != nil { l.insert(.offset) }
        if timelineTarget || node.drag != nil { l.insert(.motion) }
        self = l
    }
}

/// Box model, inner part: content → padding → size → (matchId) → background/material → border → clip.
struct BoxModifier: ViewModifier {
    let box: BoxProps
    let alignment: Alignment
    let theme: Theme
    let dark: Bool
    /// The screen root's background extends under the system bars.
    let extendsBackground: Bool
    let anim: Motion.Implicit
    let match: (id: String, namespace: Namespace.ID)?
    let layers: BoxLayers

    func body(content: Content) -> some View {
        content
            .animated(anim["padding"], PaddingModifier(insets: box.padding.edgeInsets), if: layers.contains(.padding))
            .animated(anim.any("width", "height"), FrameModifier(box: box, alignment: alignment), if: layers.contains(.frame))
            .modifier(MatchModifier(match: match))
            .animated(anim.any("background", "corner"), BackgroundModifier(box: box, theme: theme, dark: dark, extends: extendsBackground),
                      if: layers.contains(.background))
            .animated(anim["corner"], BorderModifier(box: box, theme: theme, dark: dark), if: layers.contains(.border))
            .modifier(ClipModifier(clip: box.clip, corner: box.corner))
    }
}

struct PaddingModifier: ViewModifier {
    let insets: EdgeInsets
    func body(content: Content) -> some View { content.padding(insets) }
}

struct FrameModifier: ViewModifier {
    let box: BoxProps
    let alignment: Alignment
    func body(content: Content) -> some View {
        BoxFrame(
            width: box.width, height: box.height,
            minWidth: box.minWidth.map { CGFloat($0) }, maxWidth: box.maxWidth.map { CGFloat($0) },
            minHeight: box.minHeight.map { CGFloat($0) }, maxHeight: box.maxHeight.map { CGFloat($0) },
            aspect: box.aspect.map { CGFloat($0) }, alignment: alignment
        ) { content }
    }
}

struct MatchModifier: ViewModifier {
    let match: (id: String, namespace: Namespace.ID)?
    func body(content: Content) -> some View {
        if let match { content.matchedGeometryEffect(id: match.id, in: match.namespace) } else { content }
    }
}

struct BackgroundModifier: ViewModifier {
    let box: BoxProps
    let theme: Theme
    let dark: Bool
    let extends: Bool

    func body(content: Content) -> some View {
        let shape = BoxShape(corner: box.corner)
        content.background {
            ZStack {
                if let m = box.material { shape.fill(Self.material(m)) }
                PaintFill(paint: box.background.flatMap { theme.paint($0, dark: dark) }, shape: shape)
            }
            .ignoresSafeArea(edges: extends ? .all : [])
        }
    }

    static func material(_ name: String) -> Material {
        switch name {
        case "thin": .thinMaterial
        case "thick": .thickMaterial
        default: .regularMaterial
        }
    }
}

struct BorderModifier: ViewModifier {
    let box: BoxProps
    let theme: Theme
    let dark: Bool
    func body(content: Content) -> some View {
        content.overlay {
            if let b = box.border, b.width > 0, let c = theme.color(b.color, dark: dark) {
                BoxShape(corner: box.corner).strokeBorder(c.color, lineWidth: b.width)
            }
        }
    }
}

struct ClipModifier: ViewModifier {
    let clip: Bool
    let corner: Double
    func body(content: Content) -> some View {
        if clip { content.clipShape(BoxShape(corner: corner)) } else { content }
    }
}

/// Leaf props (`color`, `fill`, `progress`) animate with the node's spec when they change.
struct ContentAnimation: ViewModifier {
    let animation: Animation?
    let value: [Value]
    func body(content: Content) -> some View {
        if let animation { content.animation(animation, value: value) } else { content }
    }
}

/// A `when` flip (or a repeat's item set changing) outside `animate(m) {}` still runs the node's
/// transition with its fallback motion.
struct WhenTransaction<V: Equatable>: ViewModifier {
    let visible: V
    let fallback: Animation?
    func body(content: Content) -> some View {
        if let fallback {
            content.transaction(value: visible) { t in
                if t.animation == nil && !t.disablesAnimations { t.animation = fallback }
            }
        } else {
            content
        }
    }
}

/// Box model, outer part: shadow → transform (props ∘ timelines ∘ drag, visual only) → zIndex.
/// Timeline targets are wrapped in a `TimelineView` that only ticks while one of their timelines runs.
struct TransformModifier: ViewModifier {
    let node: Node
    let ctx: RenderContext
    let box: BoxProps
    let anim: Motion.Implicit
    let theme: Theme
    let dark: Bool
    let layers: BoxLayers

    func body(content: Content) -> some View {
        let base = content.modifier(DragIfNeeded(node: node, ctx: ctx), if: node.drag != nil)
        let order = Double(ctx.siblingIndex) * 1e-3 + Double(ctx.repeatIndex) * 1e-6
        let drag = node.drag == nil ? .zero : dragOffset
        let timelines = ctx.store.timelines
        if timelines.isTarget(node.id) {
            let _ = timelines.register(node.id, index: ctx.repeatIndex)
            let running = timelines.isRunning(node.id)
            TimelineView(.animation(minimumInterval: nil, paused: !running)) { tv in
                let sample = timelines.sample(node.id, index: ctx.repeatIndex, at: running ? tv.date : Date())
                base.modifier(OuterModifier(box: box, anim: anim, theme: theme, dark: dark, sample: sample, drag: drag, order: order, layers: layers))
            }
        } else {
            base.modifier(OuterModifier(box: box, anim: anim, theme: theme, dark: dark, sample: TimelineSample(), drag: drag, order: order,
                                        layers: layers))
        }
    }

    /// 0 until the first drag makes the cells (then their creation re-renders this).
    private var dragOffset: CGSize {
        let key = "drag:\(node.id)\(ctx.idSuffix)"
        func n(_ axis: String) -> Double {
            if case let .number(v)? = ctx.store.existingDriver(key + ":" + axis)?.value { v } else { 0 }
        }
        return CGSize(width: n("x"), height: n("y"))
    }
}

struct DragIfNeeded: ViewModifier {
    let node: Node
    let ctx: RenderContext
    func body(content: Content) -> some View {
        if let spec = DragSpec(node.drag) { content.modifier(DragModifier(node: node, ctx: ctx, spec: spec)) } else { content }
    }
}

struct OuterModifier: ViewModifier {
    let box: BoxProps
    let anim: Motion.Implicit
    let theme: Theme
    let dark: Bool
    let sample: TimelineSample
    let drag: CGSize
    /// Sibling paint order (see `RenderContext.siblingIndex`).
    let order: Double
    let layers: BoxLayers

    func body(content: Content) -> some View {
        let motion = layers.contains(.motion)
        content
            .modifier(ShadowModifier(shadow: box.shadow.flatMap { s in
                theme.color(s.color, dark: dark).map { ($0.color, s.radius, s.x, s.y) }
            }), if: layers.contains(.shadow))
            // Timeline opacity/scale/rotation (per frame) innermost, then the prop values with their
            // implicit specs, then all translations — so no rotation ever turns an offset.
            .modifier(SampleModifier(sample: sample), if: motion)
            .animated(anim["opacity"], OpacityModifier(value: box.opacity), if: layers.contains(.opacity))
            .animated(anim["scale"], ScaleModifier(value: box.scale), if: layers.contains(.scale))
            .animated(anim["rotation"], RotationModifier(degrees: box.rotation), if: layers.contains(.rotation))
            .animated(anim.any("offsetX", "offsetY"), OffsetModifier(x: box.offsetX, y: box.offsetY), if: layers.contains(.offset))
            .modifier(OffsetModifier(x: sample.offsetX + drag.width, y: sample.offsetY + drag.height), if: motion)
            .zIndex(box.zIndex + order)
    }
}

/// Timeline opacity, scale and rotation of one frame.
struct SampleModifier: ViewModifier {
    let sample: TimelineSample
    func body(content: Content) -> some View {
        content.opacity(sample.opacity).scaleEffect(sample.scale).rotationEffect(.degrees(sample.rotation))
    }
}

struct OpacityModifier: ViewModifier {
    let value: Double
    func body(content: Content) -> some View { content.opacity(value) }
}

struct ScaleModifier: ViewModifier {
    let value: Double
    func body(content: Content) -> some View { content.scaleEffect(value) }
}

struct RotationModifier: ViewModifier {
    let degrees: Double
    func body(content: Content) -> some View { content.rotationEffect(.degrees(degrees)) }
}

struct OffsetModifier: ViewModifier {
    let x: Double, y: Double
    func body(content: Content) -> some View { content.offset(x: x, y: y) }
}

struct ShadowModifier: ViewModifier {
    let shadow: (color: Color, radius: Double, x: Double, y: Double)?
    func body(content: Content) -> some View {
        if let s = shadow { content.compositingGroup().shadow(color: s.color, radius: s.radius, x: s.x, y: s.y) } else { content }
    }
}
