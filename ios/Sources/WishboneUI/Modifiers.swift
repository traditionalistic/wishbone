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

/// Box model, inner part: content → padding → size → background/material → border → clip.
struct BoxModifier: ViewModifier {
    let box: BoxProps
    let alignment: Alignment
    let theme: Theme
    let dark: Bool
    /// The screen root's background extends under the system bars.
    let extendsBackground: Bool

    func body(content: Content) -> some View {
        let shape = BoxShape(corner: box.corner)
        content
            .padding(box.padding.edgeInsets)
            .modifier(AspectModifier(ratio: box.aspect))
            .layoutFrame(box, alignment: alignment)
            .background {
                ZStack {
                    if let m = box.material { shape.fill(Self.material(m)) }
                    PaintFill(paint: box.background.flatMap { theme.paint($0, dark: dark) }, shape: shape)
                }
                .ignoresSafeArea(edges: extendsBackground ? .all : [])
            }
            .overlay {
                if let b = box.border, b.width > 0, let c = theme.color(b.color, dark: dark) {
                    shape.strokeBorder(c.color, lineWidth: b.width)
                }
            }
            .modifier(ClipModifier(clip: box.clip, corner: box.corner))
    }

    static func material(_ name: String) -> Material {
        switch name {
        case "thin": .thinMaterial
        case "thick": .thickMaterial
        default: .regularMaterial
        }
    }
}

extension View {
    func layoutFrame(_ box: BoxProps, alignment: Alignment) -> some View {
        BoxFrame(
            width: box.width, height: box.height,
            minWidth: box.minWidth.map { CGFloat($0) }, maxWidth: box.maxWidth.map { CGFloat($0) },
            minHeight: box.minHeight.map { CGFloat($0) }, maxHeight: box.maxHeight.map { CGFloat($0) },
            alignment: alignment
        ) { self }
    }
}

struct AspectModifier: ViewModifier {
    let ratio: Double?
    func body(content: Content) -> some View {
        if let ratio, ratio > 0 { content.aspectRatio(ratio, contentMode: .fit) } else { content }
    }
}

struct ClipModifier: ViewModifier {
    let clip: Bool
    let corner: Double
    func body(content: Content) -> some View {
        if clip { content.clipShape(BoxShape(corner: corner)) } else { content }
    }
}

/// Box model, outer part: shadow → transform (visual only) → zIndex.
struct OuterModifier: ViewModifier {
    let box: BoxProps
    let theme: Theme
    let dark: Bool

    func body(content: Content) -> some View {
        content
            .modifier(ShadowModifier(shadow: box.shadow.flatMap { s in
                theme.color(s.color, dark: dark).map { ($0.color, s.radius, s.x, s.y) }
            }))
            .opacity(box.opacity)
            .scaleEffect(box.scale)
            .rotationEffect(.degrees(box.rotation))
            .offset(x: box.offsetX, y: box.offsetY)
            .zIndex(box.zIndex)
    }
}

struct ShadowModifier: ViewModifier {
    let shadow: (color: Color, radius: Double, x: Double, y: Double)?
    func body(content: Content) -> some View {
        if let s = shadow { content.shadow(color: s.color, radius: s.radius, x: s.x, y: s.y) } else { content }
    }
}
