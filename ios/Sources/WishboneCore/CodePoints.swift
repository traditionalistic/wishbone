import Foundation

/// Code-point-based string helpers. `contract/stdlib.json` requires DSL string semantics
/// (length, indices, substring, comparison, equality) to operate on Unicode scalars with
/// no normalization — Swift's native `String.count`/`==`/`<` use grapheme clusters and
/// (for `==`/`<`) canonical equivalence, so they must not be used for DSL string values.
enum CodePoints {
    static func equal(_ a: String, _ b: String) -> Bool {
        var ai = a.unicodeScalars.makeIterator()
        var bi = b.unicodeScalars.makeIterator()
        while true {
            let x = ai.next()
            let y = bi.next()
            if x == nil, y == nil { return true }
            guard let x, let y, x == y else { return false }
        }
    }

    /// -1, 0, 1
    static func compare(_ a: String, _ b: String) -> Int {
        var ai = a.unicodeScalars.makeIterator()
        var bi = b.unicodeScalars.makeIterator()
        while true {
            let x = ai.next()
            let y = bi.next()
            switch (x, y) {
            case (nil, nil): return 0
            case (nil, _): return -1
            case (_, nil): return 1
            case let (x?, y?):
                if x.value != y.value { return x.value < y.value ? -1 : 1 }
            }
        }
    }

    static func length(_ s: String) -> Int { s.unicodeScalars.count }

    /// Clamped to [0, length]; matches `substring`/`slice` semantics.
    static func clampIndex(_ i: Int, length: Int) -> Int { max(0, min(i, length)) }

    static func substring(_ s: String, _ start: Int, _ end: Int) -> String {
        let scalars = Array(s.unicodeScalars)
        let n = scalars.count
        let a = clampIndex(start, length: n)
        let b = clampIndex(end, length: n)
        guard a < b else { return "" }
        return String(String.UnicodeScalarView(scalars[a..<b]))
    }

    static func isWhitespace(_ scalar: Unicode.Scalar) -> Bool {
        switch scalar.value {
        case 0x09...0x0D, 0x20, 0x85, 0xA0, 0x1680, 0x2000...0x200A, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000:
            return true
        default:
            return false
        }
    }

    static func trim(_ s: String) -> String {
        let scalars = Array(s.unicodeScalars)
        var start = 0
        var end = scalars.count
        while start < end, isWhitespace(scalars[start]) { start += 1 }
        while end > start, isWhitespace(scalars[end - 1]) { end -= 1 }
        return String(String.UnicodeScalarView(scalars[start..<end]))
    }
}
