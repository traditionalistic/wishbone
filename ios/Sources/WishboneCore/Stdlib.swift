import Foundation

/// Errors surfaced by stdlib function calls (bad arity/types on a malformed wire AST — the
/// compiler is expected to prevent these for well-formed documents).
public enum StdlibError: Error, Sendable, CustomStringConvertible {
    case badArgs(String)

    public var description: String {
        switch self {
        case let .badArgs(message): return "stdlib: \(message)"
        }
    }
}

/// Non-higher-order stdlib functions (contract/stdlib.json), operating on already-evaluated
/// `Value`s. Higher-order ones (map/filter/find/some/every/reduce/sortBy) need to call back
/// into the interpreter to invoke lambdas, so they live on `Interpreter` instead.
enum Stdlib {
    /// Function names whose first argument is a receiver that null-propagates
    /// (`x.m(...)`/`x.p` with `x: T?` — contract/stdlib.json note). Applied uniformly by the
    /// interpreter before dispatch; harmless for the direct-call form since the compiler
    /// already guarantees a non-optional receiver there.
    static let nullPropagatingReceiver: Set<String> = [
        "length", "isEmpty", "upper", "lower", "trim", "contains", "startsWith", "endsWith",
        "split", "replace", "matches", "substring", "count", "first", "last", "map", "filter",
        "find", "some", "every", "reduce", "sum", "join", "append", "removeAt", "set", "slice",
        "indexOf", "sortBy", "reverse", "with",
    ]

    // MARK: Math

    static func min(_ a: Double, _ b: Double) -> Double { Swift.min(a, b) }
    static func max(_ a: Double, _ b: Double) -> Double { Swift.max(a, b) }
    static func abs(_ x: Double) -> Double { Swift.abs(x) }
    static func round(_ x: Double) -> Double { Format.roundHalfAwayFromZero(x) }
    static func floor(_ x: Double) -> Double { x.rounded(.down) }
    static func ceil(_ x: Double) -> Double { x.rounded(.up) }
    static func sqrt(_ x: Double) -> Double { Foundation.sqrt(x) }
    static func pow(_ a: Double, _ b: Double) -> Double { Foundation.pow(a, b) }
    static func sin(_ x: Double) -> Double { Foundation.sin(x) }
    static func cos(_ x: Double) -> Double { Foundation.cos(x) }
    static func clamp(_ x: Double, _ lo: Double, _ hi: Double) -> Double { Swift.min(Swift.max(x, lo), hi) }

    static func interpolate(_ x: Double, _ input: [Double], _ output: [Double]) -> Double {
        guard input.count == output.count, input.count >= 2 else { return output.first ?? 0 }
        let n = input.count
        if x <= input[0] { return output[0] }
        if x >= input[n - 1] { return output[n - 1] }
        var seg = 0
        for i in 0..<(n - 1) where x >= input[i] { seg = i }
        let x0 = input[seg], x1 = input[seg + 1]
        let y0 = output[seg], y1 = output[seg + 1]
        guard x1 != x0 else { return y1 }
        let t = (x - x0) / (x1 - x0)
        return y0 + (y1 - y0) * t
    }

    // MARK: Strings

    static func length(_ s: String) -> Double { Double(CodePoints.length(s)) }

    /// Full case mapping applied per code point, independently — not Swift's
    /// `String.uppercased()/lowercased()`, which apply *whole-string* (locale- and
    /// context-sensitive, e.g. Greek final sigma) mapping. `Unicode.Scalar.Properties`
    /// exposes the same per-scalar full case mapping tables with no such context rules.
    static func upper(_ s: String) -> String {
        var result = String.UnicodeScalarView()
        for scalar in s.unicodeScalars { result.append(contentsOf: scalar.properties.uppercaseMapping.unicodeScalars) }
        return String(result)
    }

    static func lower(_ s: String) -> String {
        var result = String.UnicodeScalarView()
        for scalar in s.unicodeScalars { result.append(contentsOf: scalar.properties.lowercaseMapping.unicodeScalars) }
        return String(result)
    }

    static func trim(_ s: String) -> String { CodePoints.trim(s) }

    static func startsWith(_ s: String, _ p: String) -> Bool { s.hasPrefix(p) }
    static func endsWith(_ s: String, _ p: String) -> Bool { s.hasSuffix(p) }
    static func containsString(_ s: String, _ t: String) -> Bool { t.isEmpty || s.contains(t) }

    static func split(_ s: String, _ sep: String) -> [Value] {
        if sep.isEmpty { return s.unicodeScalars.map { .string(String($0)) } }
        return s.components(separatedBy: sep).map { .string($0) }
    }

    static func replace(_ s: String, _ a: String, _ b: String) -> String {
        guard !a.isEmpty else { return s }
        return s.replacingOccurrences(of: a, with: b)
    }

    static func matches(_ s: String, _ pattern: String) -> Bool {
        guard let re = try? NSRegularExpression(pattern: "^(?:\(pattern))$") else { return false }
        let range = NSRange(s.startIndex..., in: s)
        return re.firstMatch(in: s, range: range) != nil
    }

    private static func floorArgToInt(_ n: Double) -> Int {
        guard n.isFinite else { return 0 }
        return Int(n.rounded(.down))
    }

    static func substring(_ s: String, _ start: Double, _ end: Double?) -> String {
        let length = CodePoints.length(s)
        let startI = floorArgToInt(start)
        let endI = end.map(floorArgToInt) ?? length
        return CodePoints.substring(s, startI, endI)
    }

    static let numPattern = "^[+-]?(\\d+\\.?\\d*|\\.\\d+)([eE][+-]?\\d+)?$"

    static func num(_ s: String) -> Double? {
        let t = CodePoints.trim(s)
        guard !t.isEmpty, t.range(of: numPattern, options: .regularExpression) != nil else { return nil }
        guard let d = Double(t), d.isFinite else { return nil }
        return d
    }

    static func str(_ v: Value) -> String { Format.str(v) }

    // MARK: Lists

    static func count(_ xs: [Value]) -> Double { Double(xs.count) }
    static func first(_ xs: [Value]) -> Value { xs.first ?? .null }
    static func last(_ xs: [Value]) -> Value { xs.last ?? .null }
    static func sum(_ xs: [Value]) -> Double {
        xs.reduce(0.0) { acc, v in if case let .number(n) = v { return acc + n } else { return acc } }
    }
    static func join(_ xs: [Value], _ sep: String) -> String { xs.map(Format.str).joined(separator: sep) }
    static func append(_ xs: [Value], _ x: Value) -> [Value] { xs + [x] }

    private static func validIndex(_ i: Double, count: Int) -> Int? {
        guard i.truncatingRemainder(dividingBy: 1) == 0, i >= 0, i < Double(count) else { return nil }
        return Int(i)
    }

    static func removeAt(_ xs: [Value], _ i: Double) -> [Value] {
        guard let idx = validIndex(i, count: xs.count) else { return xs }
        var copy = xs
        copy.remove(at: idx)
        return copy
    }

    static func set(_ xs: [Value], _ i: Double, _ v: Value) -> [Value] {
        guard let idx = validIndex(i, count: xs.count) else { return xs }
        var copy = xs
        copy[idx] = v
        return copy
    }

    static func slice(_ xs: [Value], _ start: Double, _ end: Double?) -> [Value] {
        let startI = floorArgToInt(start)
        let endI = end.map(floorArgToInt) ?? xs.count
        let a = CodePoints.clampIndex(startI, length: xs.count)
        let b = CodePoints.clampIndex(endI, length: xs.count)
        guard a < b else { return [] }
        return Array(xs[a..<b])
    }

    static func indexOf(_ xs: [Value], _ v: Value) -> Double {
        Double(xs.firstIndex(of: v) ?? -1)
    }

    static func reverse(_ xs: [Value]) -> [Value] { xs.reversed() }

    static func range(_ n: Double) -> [Value] {
        guard n.isFinite, n > 0 else { return [] }
        let count = Swift.min(Int(n.rounded(.down)), 10000)
        return (0..<count).map { .number(Double($0)) }
    }

    static func with(_ r: Value, _ patch: Value) -> Value {
        guard case var .record(fields) = r, case let .record(patchFields) = patch else { return r }
        for (k, v) in patchFields { fields[k] = v }
        return .record(fields)
    }

    // MARK: Format

    static func formatNumber(_ x: Double, _ decimals: Double) -> String { Format.formatNumber(x, decimals) }
    static func formatDuration(_ ms: Double, _ style: String) -> String { Format.formatDuration(ms, style) }
    static func formatDate(_ ms: Double, _ style: String, timeZone: String) -> String { Format.formatDate(ms, style, timeZone: timeZone) }
    static func plural(_ n: Double, _ forms: [String: Value], locale: String) -> String { Format.plural(n, forms: forms, locale: locale) }

    // MARK: Color / motion tokens

    static func gradient(_ colors: [Value], _ angle: Double) -> Value {
        .record(["$gradient": .list(colors), "angle": .number(angle)])
    }

    static func alpha(_ c: Value, _ aRaw: Double) -> Value {
        guard case var .record(fields) = c, case .string? = fields["$color"] else { return c }
        let a = Swift.min(1, Swift.max(0, aRaw))
        let existing: Double = { if case let .number(n)? = fields["alpha"] { return n } else { return 1.0 } }()
        let product = existing * a
        if product == 1 {
            fields.removeValue(forKey: "alpha")
        } else {
            fields["alpha"] = .number(product)
        }
        return .record(fields)
    }

    private static func motion(_ spec: [String: Value]) -> Value { .record(["$motion": .record(spec)]) }

    static func spring(_ response: Double, _ damping: Double) -> Value {
        motion(["kind": .string("spring"), "response": .number(response), "damping": .number(damping)])
    }

    private static func curveMotion(duration: Double, bezier: [Double]) -> Value {
        motion(["kind": .string("curve"), "duration": .number(duration), "bezier": .list(bezier.map(Value.number))])
    }

    static func ease(_ duration: Double) -> Value { curveMotion(duration: duration, bezier: [0.42, 0, 0.58, 1]) }
    static func linear(_ duration: Double) -> Value { curveMotion(duration: duration, bezier: [0, 0, 1, 1]) }
    static func curve(_ duration: Double, _ x1: Double, _ y1: Double, _ x2: Double, _ y2: Double) -> Value {
        curveMotion(duration: duration, bezier: [x1, y1, x2, y2])
    }
}
