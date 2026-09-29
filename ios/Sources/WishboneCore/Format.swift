import Foundation
import Synchronization

/// `str()`, `formatNumber`/`formatDuration`/`formatDate`/`plural`, and the shared
/// fixed-point/number-to-string algorithms from contract/stdlib.json `notes`.
enum Format {
    // MARK: Rounding

    /// Half away from zero. Reference (contract/stdlib.json): t = trunc(x); |x-t| >= 0.5 ?
    /// t + sign(x) : t. Swift's `.toNearestOrAwayFromZero` implements exactly this rule.
    static func roundHalfAwayFromZero(_ x: Double) -> Double {
        x.rounded(.toNearestOrAwayFromZero)
    }

    // MARK: Fixed-point digits (shared by numberToString and formatNumber)

    /// Digit string (no sign, no point) of `round-half-away(|x| * 10^d)`, left-padded to at
    /// least `d + 1` digits — or, when `|x| >= 1e15`, the exact integer digits of
    /// `round-half-away(x)` followed by `d` zeros.
    private static func fixedPointDigits(_ x: Double, _ d: Int) -> String {
        let absX = abs(x)
        if absX >= 1e15 {
            let r = roundHalfAwayFromZero(absX)
            return integerDigits(r) + String(repeating: "0", count: d)
        }
        let scaled = absX * pow(10.0, Double(d))
        let r = roundHalfAwayFromZero(scaled)
        var digits = integerDigits(r)
        while digits.count < d + 1 { digits = "0" + digits }
        return digits
    }

    /// Sign, integer part, fraction part (fraction part is "" when d == 0). Sign is "-" iff
    /// x < 0 and at least one printed digit is non-zero (so -0 / -0.00 never print a sign).
    private static func fixedPointComponents(_ x: Double, _ d: Int) -> (sign: String, intPart: String, fracPart: String) {
        let digits = fixedPointDigits(x, d)
        let hasNonZero = digits.contains { $0 != "0" }
        let sign = (x < 0 && hasNonZero) ? "-" : ""
        guard d > 0 else { return (sign, digits, "") }
        let splitIndex = digits.index(digits.endIndex, offsetBy: -d)
        return (sign, String(digits[..<splitIndex]), String(digits[splitIndex...]))
    }

    /// Exact decimal form of a whole, finite `r` (never -0, which `%.0f` prints as "-0") —
    /// what `%.0f` prints. `Int64` holds it exactly below 9e18, ~100× cheaper than `String(format:)`.
    private static func integerDigits(_ r: Double) -> String {
        Swift.abs(r) < 9e18 ? String(Int64(r)) : String(format: "%.0f", r)
    }

    // MARK: Number -> string (str, interpolation, join)

    static func numberToString(_ x: Double) -> String {
        if x.isNaN { return "NaN" }
        if x.isInfinite { return x > 0 ? "Infinity" : "-Infinity" }
        if x == 0 { return "0" } // also covers -0
        let absX = abs(x)
        if absX < 1e15, x == x.rounded(.towardZero) {
            return integerDigits(x)
        }
        if absX >= 1e15 {
            return integerDigits(roundHalfAwayFromZero(x))
        }
        let (sign, intPart, fracPart) = fixedPointComponents(x, 6)
        var frac = fracPart
        while frac.hasSuffix("0") { frac.removeLast() }
        return frac.isEmpty ? "\(sign)\(intPart)" : "\(sign)\(intPart).\(frac)"
    }

    // MARK: JSON string quoting (str() of list/record, and Value.toJSON())

    /// Escapes only `"`, `\`, and U+0000–U+001F (`\n \r \t \b \f`, others as `\u00xx`).
    /// `/` and non-ASCII pass through raw.
    static func jsonQuote(_ s: String) -> String {
        var out = "\""
        for scalar in s.unicodeScalars {
            switch scalar {
            case "\"": out += "\\\""
            case "\\": out += "\\\\"
            case "\n": out += "\\n"
            case "\r": out += "\\r"
            case "\t": out += "\\t"
            case Unicode.Scalar(0x08): out += "\\b"
            case Unicode.Scalar(0x0C): out += "\\f"
            default:
                if scalar.value < 0x20 {
                    out += String(format: "\\u%04x", scalar.value)
                } else {
                    out.unicodeScalars.append(scalar)
                }
            }
        }
        out += "\""
        return out
    }

    /// `str(x)`: identity for strings, compact JSON (keys sorted by code point) otherwise.
    static func str(_ v: Value) -> String {
        if case let .string(s) = v { return s }
        return v.toJSONString()
    }

    // MARK: formatNumber

    static func formatNumber(_ x: Double, _ decimalsRaw: Double) -> String {
        guard x.isFinite else { return numberToString(x) }
        // clamp(floor(d), 0, 10) — compared in Double space first: `decimalsRaw` may be
        // NaN/±infinity/1e19, and `Int(decimalsRaw.rounded(.down))` alone would trap.
        let d: Int
        if decimalsRaw.isNaN {
            d = 0
        } else {
            let floored = decimalsRaw.rounded(.down)
            d = floored <= 0 ? 0 : (floored >= 10 ? 10 : Int(floored))
        }
        let (sign, intPart, fracPart) = fixedPointComponents(x, d)
        let grouped = groupThousands(intPart)
        return d > 0 ? "\(sign)\(grouped).\(fracPart)" : "\(sign)\(grouped)"
    }

    private static func groupThousands(_ digits: String) -> String {
        let chars = Array(digits)
        guard chars.count > 3 else { return digits }
        var out = ""
        for (i, c) in chars.enumerated() {
            let fromEnd = chars.count - i
            if i > 0, fromEnd % 3 == 0 { out.append("\u{00A0}") }
            out.append(c)
        }
        return out
    }

    // MARK: formatDuration

    static func formatDuration(_ ms: Double, _ style: String) -> String {
        // `ms` may be finite but astronomically large (e.g. 1e19); clamp in Double space
        // before ever converting to Int, same reasoning as `formatNumber`'s `d` above.
        let totalSeconds: Int
        if ms.isFinite, ms >= 0 {
            let secondsFloor = (ms / 1000).rounded(.down)
            totalSeconds = secondsFloor < 0x1p62 ? Int(secondsFloor) : Int.max
        } else {
            totalSeconds = 0
        }
        func pad2(_ n: Int) -> String { n < 10 ? "0\(n)" : "\(n)" }
        switch style {
        case "hms":
            let h = totalSeconds / 3600, m = (totalSeconds % 3600) / 60, s = totalSeconds % 60
            return "\(pad2(h)):\(pad2(m)):\(pad2(s))"
        case "ms":
            let m = totalSeconds / 60, s = totalSeconds % 60
            return "\(pad2(m)):\(pad2(s))"
        case "dhms":
            let d = totalSeconds / 86400
            let h = (totalSeconds % 86400) / 3600
            let m = (totalSeconds % 3600) / 60
            let s = totalSeconds % 60
            return "\(d):\(pad2(h)):\(pad2(m)):\(pad2(s))"
        default:
            return ""
        }
    }

    // MARK: formatDate

    private struct DateFormatterKey: Hashable {
        let format: String
        let timeZone: String
    }

    /// Building a `DateFormatter` costs ~25 µs; one per (format, zone), reused. Formatting runs
    /// under the lock (a `DateFormatter` isn't `Sendable`), so it's safe from any thread.
    /// ponytail: cleared wholesale past 32 entries — the key space is 3 formats × the host's
    /// zone(s), so it never fills in practice.
    private static let dateFormatters = Mutex<[DateFormatterKey: DateFormatter]>([:])

    static func formatDate(_ ms: Double, _ style: String, timeZone: String) -> String {
        guard ms.isFinite else { return "" }
        let format: String
        switch style {
        case "date": format = "dd.MM.yyyy"
        case "time": format = "HH:mm"
        case "dateTime": format = "dd.MM.yyyy HH:mm"
        default: return ""
        }
        let date = Date(timeIntervalSince1970: ms / 1000)
        let key = DateFormatterKey(format: format, timeZone: timeZone)
        return dateFormatters.withLock { cache in
            if let df = cache[key] { return df.string(from: date) }
            if cache.count >= 32 { cache.removeAll() }
            let df = DateFormatter()
            df.locale = Locale(identifier: "en_US_POSIX")
            df.timeZone = TimeZone(identifier: timeZone) ?? TimeZone(identifier: "UTC")
            df.dateFormat = format
            cache[key] = df
            return df.string(from: date)
        }
    }

    // MARK: plural

    private static let enLikeLanguages: Set<String> = ["en", "az", "ka", "uz", "ab", "kk"]

    static func pluralLanguage(_ locale: String) -> String {
        let lower = locale.lowercased()
        if let dash = lower.firstIndex(where: { $0 == "-" || $0 == "_" }) {
            return String(lower[..<dash])
        }
        return lower
    }

    static func pluralCategory(_ n: Double, locale: String) -> String {
        let lang = pluralLanguage(locale)
        switch lang {
        case "ru":
            // `n` integral (checked below) doesn't mean "small" — every double beyond ~2^52
            // is trivially its own truncation, so a huge finite `n` (e.g. 1e19) would still
            // pass this guard and then trap converting to Int64. Compute the digits via
            // `truncatingRemainder` instead, which never traps for any finite Double.
            guard n.isFinite, n == n.rounded(.towardZero) else { return "other" }
            let absN = Swift.abs(n)
            let mod10 = absN.truncatingRemainder(dividingBy: 10)
            let mod100 = absN.truncatingRemainder(dividingBy: 100)
            if mod10 == 1, mod100 != 11 { return "one" }
            if (2.0...4.0).contains(mod10), !(12.0...14.0).contains(mod100) { return "few" }
            return "many"
        case let l where enLikeLanguages.contains(l):
            return abs(n) == 1 ? "one" : "other"
        default:
            return "other"
        }
    }

    static func plural(_ n: Double, forms: [String: Value], locale: String) -> String {
        let category = pluralCategory(n, locale: locale)
        let chosen = forms[category] ?? forms["other"]
        guard case let .string(template)? = chosen else { return "" }
        return template.replacingOccurrences(of: "#", with: numberToString(n), options: .literal)
    }
}
