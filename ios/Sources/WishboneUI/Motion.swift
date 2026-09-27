import SwiftUI
import WishboneCore

/// Motion value → SwiftUI animation (renderer-decisions: Motion).
enum Motion {
    static func animation(_ spec: MotionSpec) -> Animation {
        if spec.kind == "spring" {
            return .spring(response: (spec.response ?? 300) / 1000, dampingFraction: spec.damping ?? 0.86)
        }
        let b = spec.bezier.flatMap { $0.count == 4 ? $0 : nil } ?? [0.42, 0, 0.58, 1]
        return .timingCurve(b[0], b[1], b[2], b[3], duration: (spec.duration ?? 300) / 1000)
    }

    /// `animate` of a node: prop (or `*`) → animation.
    struct Implicit {
        var specs: [String: Animation] = [:]

        /// The prop's own spec, else `*`.
        subscript(_ prop: String) -> Animation? { specs[prop] ?? specs["*"] }
        /// First spec found among props (a SwiftUI modifier can carry one animation).
        func any(_ props: String...) -> Animation? { props.lazy.compactMap { specs[$0] }.first ?? specs["*"] }
    }
}

extension Theme {
    func animation(_ value: Value) -> Animation? { motion(value).map(Motion.animation) }
}

/// One part of a `transition` list on the wire: `{"k": "fade"}`, `{"k": "slide", "edge"}`,
/// `{"k": "scale", "from"}`, `{"k": "offset", "x", "y"}`.
enum TransitionPart: Equatable {
    case fade
    case slide(Edge)
    case scale(Double)
    case offset(Double, Double)

    static func parts(_ wire: Value?) -> [TransitionPart] {
        guard case let .list(items)? = wire else { return [] }
        return items.compactMap { item in
            guard case let .record(r) = item, case let .string(kind)? = r["k"] else { return nil }
            func n(_ k: String, _ d: Double) -> Double { if case let .number(x)? = r[k], x.isFinite { x } else { d } }
            switch kind {
            case "fade": return .fade
            case "slide":
                let edge: Edge = switch r["edge"] {
                case .string("top")?: .top
                case .string("start")?: .leading
                case .string("end")?: .trailing
                default: .bottom
                }
                return .slide(edge)
            case "scale": return .scale(n("from", 0.8))
            case "offset": return .offset(n("x", 0), n("y", 0))
            default: return nil
            }
        }
    }

    /// `env.reduceMotion`: slide/scale/offset become fades.
    static func reduced(_ parts: [TransitionPart]) -> [TransitionPart] {
        parts.isEmpty ? [] : [.fade]
    }

    static func transition(_ parts: [TransitionPart]) -> AnyTransition {
        parts.reduce(AnyTransition.identity) { t, part in
            let p: AnyTransition = switch part {
            case .fade: .opacity
            case let .slide(edge): .move(edge: edge)
            case let .scale(from): .scale(scale: from)
            case let .offset(x, y): .offset(x: x, y: y)
            }
            return t.combined(with: p)
        }
    }
}

/// `drag: {axis, snap?, bounds?}` (spec §4.3).
struct DragSpec: Equatable {
    var axis: String
    var snap: [Double]
    var bounds: ClosedRange<Double>?

    init?(_ wire: Value?) {
        guard case let .record(r)? = wire else { return nil }
        axis = if case let .string(a)? = r["axis"] { a } else { "both" }
        func nums(_ k: String) -> [Double] {
            if case let .list(l)? = r[k] { l.compactMap { if case let .number(n) = $0, n.isFinite { n } else { nil } } } else { [] }
        }
        snap = nums("snap")
        let b = nums("bounds")
        bounds = b.count == 2 && b[0] <= b[1] ? b[0]...b[1] : nil
    }

    var movesX: Bool { axis == "x" || axis == "both" }
    var movesY: Bool { axis == "y" || axis == "both" }

    /// Finger translation → node translation along the allowed axes, clamped to `bounds`.
    func follow(_ t: CGSize) -> CGSize {
        func clamp(_ v: Double) -> Double { bounds.map { min(max(v, $0.lowerBound), $0.upperBound) } ?? v }
        return CGSize(width: movesX ? clamp(t.width) : 0, height: movesY ? clamp(t.height) : 0)
    }

    /// Axis lock on the first move: an `x` drag ignores mostly-vertical swipes (the scroll keeps them).
    func claims(_ t: CGSize) -> Bool {
        switch axis {
        case "x": abs(t.width) >= abs(t.height)
        case "y": abs(t.height) >= abs(t.width)
        default: true
        }
    }

    /// Resting point after release: the nearest snap point (or 0 without snaps).
    func rest(_ v: Double) -> Double {
        snap.min { abs($0 - v) < abs($1 - v) } ?? 0
    }
}

/// Timeline values for one node instance, composed onto its transform layer.
struct TimelineSample: Equatable {
    var opacity = 1.0, scale = 1.0, offsetX = 0.0, offsetY = 0.0, rotation = 0.0

    /// opacity/scale multiply, offsets/rotation add (contract/README.md).
    mutating func compose(_ prop: String, _ v: Double) {
        switch prop {
        case "opacity": opacity *= v
        case "scale": scale *= v
        case "offsetX": offsetX += v
        case "offsetY": offsetY += v
        case "rotation": rotation += v
        default: break
        }
    }
}

/// `scroll.<id>.x|y`: content offset from the resting top/left (negative when overscrolled).
enum ScrollDriver {
    static func offset(contentOffset: CGPoint, insets: EdgeInsets) -> CGPoint {
        CGPoint(x: contentOffset.x + insets.leading, y: contentOffset.y + insets.top)
    }
}
