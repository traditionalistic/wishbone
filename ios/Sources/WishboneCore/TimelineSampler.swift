import Foundation

/// WebKit-style `UnitBezier` solver (contract/README.md): Newton's method from `t = p`, at
/// most 8 iterations, stopping early if the derivative underflows; falls back to bisection on
/// `[0, 1]`, at most 64 iterations. Target tolerance `1e-12` on `x`.
public struct CubicBezier: Sendable {
    private let ax, bx, cx: Double
    private let ay, by, cy: Double

    public init(_ x1: Double, _ y1: Double, _ x2: Double, _ y2: Double) {
        cx = 3 * x1
        bx = 3 * (x2 - x1) - cx
        ax = 1 - cx - bx
        cy = 3 * y1
        by = 3 * (y2 - y1) - cy
        ay = 1 - cy - by
    }

    private func sampleX(_ t: Double) -> Double { ((ax * t + bx) * t + cx) * t }
    private func sampleY(_ t: Double) -> Double { ((ay * t + by) * t + cy) * t }
    private func sampleDerivativeX(_ t: Double) -> Double { (3 * ax * t + 2 * bx) * t + cx }

    /// Solves `x(t) = p` for `t` (expected `p` in `[0, 1]`) and returns `y(t)`.
    public func solve(_ p: Double) -> Double {
        var t = p
        for _ in 0..<8 {
            let x = sampleX(t) - p
            if abs(x) < 1e-12 { return sampleY(t) }
            let d = sampleDerivativeX(t)
            if abs(d) < 1e-6 { break }
            t -= x / d
        }
        var lo = 0.0
        var hi = 1.0
        t = p
        if t < lo { return sampleY(lo) }
        if t > hi { return sampleY(hi) }
        for _ in 0..<64 {
            let x = sampleX(t)
            if abs(x - p) < 1e-12 { break }
            if p > x { lo = t } else { hi = t }
            t = (lo + hi) / 2
        }
        return sampleY(t)
    }
}

/// Samples timeline tracks per contract/README.md's normative algorithm. Pure/static: the
/// running/stopped state of a timeline, and combining its output with node props (multiply
/// for opacity/scale, add for offsets/rotation), is the renderer's job (tasks 9/10).
public enum TimelineSampler {
    /// `["target.prop": value]` for every track, at time `tMs` for repeat-instance `index`.
    public static func sample(_ timeline: Timeline, at tMs: Double, index: Int) -> [String: Double] {
        var result: [String: Double] = [:]
        for track in timeline.tracks {
            result["\(track.target).\(track.prop)"] = sampleTrack(
                track, duration: timeline.duration, repeatCount: timeline.repeatCount,
                autoreverse: timeline.autoreverse, tMs: tMs, index: index
            )
        }
        return result
    }

    private static func sampleTrack(
        _ track: TimelineTrack, duration D: Double, repeatCount: Int, autoreverse: Bool, tMs: Double, index: Int
    ) -> Double {
        let keys = track.keys.sorted { $0.t < $1.t }
        guard let k0 = keys.first, let kn = keys.last else { return 0 }
        if D == 0 { return kn.v }
        let u = tMs - track.stagger * Double(index)
        if u < 0 { return k0.v }
        let c = (u / D).rounded(.down)
        var ph = u - c * D
        let cInt = Int(c)
        if repeatCount != -1, cInt >= repeatCount {
            ph = (autoreverse && (repeatCount - 1) % 2 != 0) ? 0 : D
        } else if autoreverse, !cInt.isMultiple(of: 2) {
            ph = D - ph
        }
        return valueAt(ph, keys: keys)
    }

    private static func valueAt(_ ph: Double, keys: [TimelineKey]) -> Double {
        guard let first = keys.first, let last = keys.last else { return 0 }
        if ph <= first.t { return first.v }
        if ph >= last.t { return last.v }
        for i in 0..<(keys.count - 1) {
            let a = keys[i]
            let b = keys[i + 1]
            guard ph >= a.t, ph <= b.t else { continue }
            if b.t == a.t { return b.v } // equal key times: a step to the later key
            let frac = (ph - a.t) / (b.t - a.t)
            let eased = ease(frac, curve: b.ease)
            return a.v + (b.v - a.v) * eased
        }
        return last.v
    }

    private static func ease(_ t: Double, curve: [Double]?) -> Double {
        guard let c = curve, c.count == 4 else { return t } // absent = linear
        return CubicBezier(c[0], c[1], c[2], c[3]).solve(t)
    }

    /// Total wall-clock duration until every repeat-instance in `[0, instanceCount)` settles
    /// (used by `await play`); infinite for `repeat == -1`.
    public static func duration(_ timeline: Timeline, instanceCount: Int) -> Double {
        if timeline.repeatCount == -1 { return .infinity }
        let maxStagger = timeline.tracks.map(\.stagger).max() ?? 0
        return timeline.duration * Double(timeline.repeatCount) + maxStagger * Double(max(0, instanceCount - 1))
    }
}
