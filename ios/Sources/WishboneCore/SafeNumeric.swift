import Foundation

/// `Int(someDouble)` traps for non-finite values (NaN, ±infinity) and for finite magnitudes
/// outside `Int`'s representable range (e.g. `1e19`) — both of which can appear in wire JSON
/// numbers or DSL-computed values. Every conversion here compares in `Double` space *before*
/// converting, so it never trades a correctness bug for a crash.
enum SafeNumeric {
    /// `Int(d)`, or `fallback` when `d` is non-finite or magnitude-out-of-range for `Int`.
    static func int(_ d: Double, fallback: Int = 0) -> Int {
        guard d.isFinite, d >= -0x1p63, d < 0x1p63 else { return fallback }
        return Int(d)
    }

    /// Floors `d` and clamps it into `[0, length]` without ever converting an out-of-range
    /// `Double` to `Int`; `NaN` clamps to 0 (contract/stdlib.json: "NaN counts as 0").
    static func clampedIndex(_ d: Double, length: Int) -> Int {
        guard !d.isNaN else { return 0 }
        let floored = d.rounded(.down)
        if floored < 0 { return 0 }
        if floored >= Double(length) { return length }
        return Int(floored)
    }
}
