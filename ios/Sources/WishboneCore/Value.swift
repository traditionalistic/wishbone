import Foundation

/// A lambda literal: parameters plus body, closing over the locals chain active where it
/// was created (`["lam", params, body]`).
public struct Lambda: Sendable {
    public let params: [String]
    public let body: Expr
    public let captured: Locals?

    public init(params: [String], body: Expr, captured: Locals?) {
        self.params = params
        self.body = body
        self.captured = captured
    }
}

/// Runtime value. `record` key order is not observable (`str()` sorts keys by code point,
/// equality is order-insensitive per contract/stdlib.json), so a plain dictionary suffices.
/// All cases are immutable value types; assignment always produces a new `Value`.
///
/// ponytail: `[String: Value]` hashes/compares keys by Swift's native `String` semantics
/// (canonical equivalence), not strict code points like DSL string *values* are required to
/// (see `CodePoints`). A record built from two keys that are byte-different but canonically
/// equivalent (e.g. "e"+combining-acute vs. precomposed "é") would collapse to one dictionary
/// entry instead of two distinct fields. `map<T>` keys come from the backend's own JSON, where
/// this is exotic but not impossible. Upgrade path if it ever bites: wrap `String` in a small
/// `CodePointKey: Hashable` that hashes/compares `unicodeScalars`, and use
/// `[CodePointKey: Value]` for `record`.
public enum Value: Sendable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case list([Value])
    case record([String: Value])
    case lambda(Lambda)
}

extension Value: Equatable {
    /// Deep, structural equality: numbers by value (NaN != NaN), strings by code points,
    /// lists elementwise, records by key set + values (order-insensitive). Lambdas are
    /// never equal (functions aren't DSL-comparable values).
    public static func == (lhs: Value, rhs: Value) -> Bool {
        switch (lhs, rhs) {
        case (.null, .null):
            return true
        case let (.bool(a), .bool(b)):
            return a == b
        case let (.number(a), .number(b)):
            return a == b
        case let (.string(a), .string(b)):
            return CodePoints.equal(a, b)
        case let (.list(a), .list(b)):
            return a.count == b.count && zip(a, b).allSatisfy { $0 == $1 }
        case let (.record(a), .record(b)):
            guard a.count == b.count else { return false }
            for (key, value) in a {
                guard let other = b[key], other == value else { return false }
            }
            return true
        default:
            return false
        }
    }
}

// MARK: - JSON

public enum ValueError: Error, Sendable, CustomStringConvertible {
    case decode(String)

    public var description: String {
        switch self {
        case let .decode(message): return "Value decode error: \(message)"
        }
    }
}

extension Value {
    public init(json: Data) throws {
        self = Value(wire: try JSONValue.parse(json))
    }

    public init(json text: String) throws {
        self = Value(wire: try JSONValue.parse(text))
    }

    init(wire: JSONValue) {
        switch wire {
        case .null: self = .null
        case let .bool(b): self = .bool(b)
        case let .number(n): self = .number(n)
        case let .string(s): self = .string(s)
        case let .array(items): self = .list(items.map(Value.init(wire:)))
        case let .object(pairs):
            var record: [String: Value] = [:]
            for (k, v) in pairs { record[k] = Value(wire: v) }
            self = .record(record)
        }
    }

    /// Plain JSON encoding (not the DSL `str()` formatting rule).
    public func toJSON() -> Data {
        Data(toJSONString().utf8)
    }

    func toJSONString() -> String {
        switch self {
        case .null: return "null"
        case let .bool(b): return b ? "true" : "false"
        case let .number(n): return Format.numberToString(n)
        case let .string(s): return Format.jsonQuote(s)
        case let .list(items): return "[" + items.map { $0.toJSONString() }.joined(separator: ",") + "]"
        case let .record(fields):
            let sortedKeys = fields.keys.sorted { CodePoints.compare($0, $1) < 0 }
            return "{" + sortedKeys.map { "\(Format.jsonQuote($0)):\(fields[$0]!.toJSONString())" }.joined(separator: ",") + "}"
        case .lambda: return "null"
        }
    }
}

// MARK: - Path assignment (Scope.set support)

extension Value {
    /// Copy-on-write deep update at `path` (string element → record field, number element →
    /// list index). Returns `self` unchanged (a no-op) when the path doesn't resolve: a
    /// missing field, an out-of-range/non-integer index, or a null/wrong-shaped value along
    /// the way. An empty path replaces the whole value.
    public func settingPath(_ path: [Value], to newValue: Value) -> Value {
        guard let head = path.first else { return newValue }
        let rest = Array(path.dropFirst())
        switch (self, head) {
        case let (.record(fields), .string(key)):
            guard let existing = fields[key] else { return self }
            var updated = fields
            updated[key] = existing.settingPath(rest, to: newValue)
            return .record(updated)
        case let (.list(items), .number(indexValue)):
            // Compare against `items.count` in Double space *before* converting to Int —
            // `indexValue` may be e.g. 1e19 (passes the integrality check below), and
            // `Int(indexValue)` alone would trap before the bounds check ever ran.
            guard indexValue.truncatingRemainder(dividingBy: 1) == 0,
                  indexValue >= 0, indexValue < Double(items.count) else { return self }
            var updated = items
            let i = Int(indexValue)
            updated[i] = updated[i].settingPath(rest, to: newValue)
            return .list(updated)
        default:
            return self
        }
    }
}
