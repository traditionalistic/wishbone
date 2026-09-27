import SwiftUI
import WishboneCore

/// `width`/`height`: number = fixed, `"fill"` = take the available space, `"wrap"` = intrinsic.
enum Dim: Equatable {
    case wrap, fill
    case fixed(Double)

    init(_ v: Value) {
        switch v {
        case let .number(n) where n.isFinite: self = .fixed(max(0, n))
        case .string("fill"): self = .fill
        default: self = .wrap
        }
    }

    var fixed: CGFloat? { if case let .fixed(n) = self { CGFloat(n) } else { nil } }
}

struct Insets: Equatable {
    var top = 0.0, bottom = 0.0, start = 0.0, end = 0.0

    /// number, or `{top, bottom, start, end, h, v}` — specific edges win over h/v.
    init(_ v: Value) {
        switch v {
        case let .number(n) where n.isFinite:
            let n = max(0, n) // ponytail: negative padding is dropped, not turned into an offset
            (top, bottom, start, end) = (n, n, n, n)
        case let .record(r):
            func n(_ k: String) -> Double? { if case let .number(x)? = r[k], x.isFinite { max(0, x) } else { nil } }
            top = n("top") ?? n("v") ?? 0
            bottom = n("bottom") ?? n("v") ?? 0
            start = n("start") ?? n("h") ?? 0
            end = n("end") ?? n("h") ?? 0
        default:
            break
        }
    }

    init(top: Double, bottom: Double, start: Double, end: Double) {
        (self.top, self.bottom, self.start, self.end) = (top, bottom, start, end)
    }

    var edgeInsets: EdgeInsets {
        EdgeInsets(top: top, leading: start, bottom: bottom, trailing: end)
    }
}

/// The common props (spec §2.1 modifiers) of one node, parsed from evaluated values.
struct BoxProps {
    var width: Dim, height: Dim
    var minWidth, maxWidth, minHeight, maxHeight, aspect: Double?
    var padding: Insets
    var background: Value?
    var material: String?
    var corner: Double
    var border: (color: Value, width: Double)?
    var clip: Bool
    var shadow: (color: Value, radius: Double, x: Double, y: Double)?
    var opacity, offsetX, offsetY, scale, rotation, zIndex: Double

    init(_ p: Props) {
        width = Dim(p["width"])
        height = Dim(p["height"])
        minWidth = p.length("minWidth")
        maxWidth = p.length("maxWidth")
        minHeight = p.length("minHeight")
        maxHeight = p.length("maxHeight")
        aspect = p.number("aspect").flatMap { $0 > 0 ? $0 : nil }
        padding = Insets(p["padding"])
        background = p.values["background"].flatMap { $0 == .null ? nil : $0 }
        material = p.string("material")
        corner = p.length("corner") ?? 0
        clip = p.bool("clip")
        if case let .record(b) = p["border"], let c = b["color"], case let .number(w)? = b["width"], w.isFinite {
            border = (c, w)
        }
        if case let .record(s) = p["shadow"], let c = s["color"], case let .number(r)? = s["radius"], r.isFinite {
            func n(_ k: String) -> Double { if case let .number(x)? = s[k], x.isFinite { x } else { 0 } }
            shadow = (c, max(0, r), n("x"), n("y"))
        }
        opacity = p.number("opacity") ?? 1
        offsetX = p.number("offsetX") ?? 0
        offsetY = p.number("offsetY") ?? 0
        scale = p.number("scale") ?? 1
        rotation = p.number("rotation") ?? 0
        zIndex = p.number("zIndex") ?? 0
    }
}

enum Align {
    /// v-stack / v-scroll `align`: horizontal placement of children.
    static func horizontal(_ s: String?) -> HorizontalAlignment {
        switch s {
        case "center": .center
        case "end": .trailing
        default: .leading
        }
    }

    /// h-stack / h-scroll `align`: vertical placement of children.
    static func vertical(_ s: String?) -> VerticalAlignment {
        switch s {
        case "center": .center
        case "end": .bottom
        default: .top
        }
    }

    /// z-stack 9-way `align` (default center).
    static func z(_ s: String?) -> Alignment {
        switch s {
        case "topStart": .topLeading
        case "top": .top
        case "topEnd": .topTrailing
        case "start": .leading
        case "end": .trailing
        case "bottomStart": .bottomLeading
        case "bottom": .bottom
        case "bottomEnd": .bottomTrailing
        default: .center
        }
    }

    /// Where a primitive's content sits inside a frame larger than itself (fixed or fill size) —
    /// matches Compose, where the size modifier sits on the layout itself: stacks keep their
    /// arrangement from the start edge, text from the top.
    static func frame(type: String, props: Props) -> Alignment {
        let align = props.string("align")
        switch type {
        case "stack":
            switch props.string("axis") {
            case "h": return Alignment(horizontal: .leading, vertical: vertical(align))
            case "z": return z(align)
            default: return Alignment(horizontal: horizontal(align), vertical: .top)
            }
        case "scroll":
            return props.string("axis") == "h"
                ? Alignment(horizontal: .leading, vertical: vertical(align))
                : Alignment(horizontal: horizontal(align), vertical: .top)
        case "grid":
            return .topLeading
        case "text":
            return Alignment(horizontal: horizontal(props.string("textAlign")), vertical: .top)
        case "textField":
            return .leading
        default:
            return .center
        }
    }

    /// CSS `linear-gradient(<angle>deg)` endpoints in unit space for a box of `size`
    /// (0 = to top, 90 = to right; the gradient line spans the box's corners projected on it).
    static func gradientPoints(angle: Double, size: CGSize) -> (start: UnitPoint, end: UnitPoint) {
        let rad = angle * .pi / 180
        let dx = sin(rad), dy = -cos(rad)
        let w = max(size.width, 1), h = max(size.height, 1)
        let half = (abs(w * dx) + abs(h * dy)) / 2
        let cx = w / 2, cy = h / 2
        return (
            UnitPoint(x: (cx - dx * half) / w, y: (cy - dy * half) / h),
            UnitPoint(x: (cx + dx * half) / w, y: (cy + dy * half) / h)
        )
    }
}

/// Rounded rectangle whose radius is clamped to half the smaller side (pill = 9999 → capsule).
struct BoxShape: InsettableShape {
    var corner: Double
    var inset: CGFloat = 0

    var animatableData: Double {
        get { corner }
        set { corner = newValue }
    }

    static func radius(_ corner: Double, in rect: CGRect) -> CGFloat {
        max(0, min(CGFloat(corner), min(rect.width, rect.height) / 2))
    }

    func path(in rect: CGRect) -> Path {
        let r = BoxShape.radius(corner, in: rect)
        let box = rect.insetBy(dx: inset, dy: inset)
        guard box.width > 0, box.height > 0 else { return Path() }
        return Path(roundedRect: box, cornerRadius: max(0, r - inset), style: .circular)
    }

    func inset(by amount: CGFloat) -> BoxShape {
        var s = self
        s.inset += amount
        return s
    }
}

/// The `size` step of the box model with Compose semantics: `fill` takes the offered space (and
/// reports infinity to a probing stack, so fill siblings share equally), `wrap` measures the
/// content within `max` and reports it clamped to `min…max`; content is placed by `alignment`.
struct BoxFrame: Layout {
    var width: Dim, height: Dim
    var minWidth, maxWidth, minHeight, maxHeight: CGFloat?
    var alignment: Alignment

    static func clamp(_ x: CGFloat, _ lo: CGFloat?, _ hi: CGFloat?) -> CGFloat {
        var v = x
        if let hi { v = min(v, hi) }
        if let lo { v = max(v, lo) }
        return v
    }

    /// What the content is offered along one axis.
    static func childProposal(_ dim: Dim, _ proposal: CGFloat?, min lo: CGFloat?, max hi: CGFloat?) -> CGFloat? {
        switch dim {
        case let .fixed(n): clamp(n, lo, hi)
        case .fill: proposal.map { clamp($0, lo, hi) }
        case .wrap: proposal.map { p in hi.map { Swift.min(p, $0) } ?? p }
        }
    }

    /// The box's own size along one axis.
    static func size(_ dim: Dim, _ proposal: CGFloat?, child: CGFloat, min lo: CGFloat?, max hi: CGFloat?) -> CGFloat {
        switch dim {
        case let .fixed(n): clamp(n, lo, hi)
        case .fill: clamp(proposal ?? child, lo, hi)
        case .wrap: clamp(child, lo, hi)
        }
    }

    private func childProposal(_ p: ProposedViewSize) -> ProposedViewSize {
        ProposedViewSize(
            width: Self.childProposal(width, p.width, min: minWidth, max: maxWidth),
            height: Self.childProposal(height, p.height, min: minHeight, max: maxHeight)
        )
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let child = subviews.reduce(CGSize.zero) { acc, s in
            let size = s.sizeThatFits(childProposal(proposal))
            return CGSize(width: max(acc.width, size.width), height: max(acc.height, size.height))
        }
        return CGSize(
            width: Self.size(width, proposal.width, child: child.width, min: minWidth, max: maxWidth),
            height: Self.size(height, proposal.height, child: child.height, min: minHeight, max: maxHeight)
        )
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        // wrap re-offers the parent's proposal (not our measured size: re-measuring a text row at its
        // own fractional width truncates the last text); fill/fixed offer the resolved size.
        let p = ProposedViewSize(
            width: width == .wrap ? Self.childProposal(width, proposal.width, min: minWidth, max: maxWidth) : bounds.width,
            height: height == .wrap ? Self.childProposal(height, proposal.height, min: minHeight, max: maxHeight) : bounds.height
        )
        for s in subviews {
            let size = s.sizeThatFits(p)
            let dx: CGFloat = switch alignment.horizontal {
            case .leading: 0
            case .trailing: bounds.width - size.width
            default: (bounds.width - size.width) / 2
            }
            let dy: CGFloat = switch alignment.vertical {
            case .top: 0
            case .bottom: bounds.height - size.height
            default: (bounds.height - size.height) / 2
            }
            s.place(at: CGPoint(x: bounds.minX + dx, y: bounds.minY + dy), anchor: .topLeading, proposal: p)
        }
    }
}
