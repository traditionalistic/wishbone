import Foundation

/// Minimal JSON tree used to decode the wire AST (`Expr`/`Stmt`/`Node`/`Document`).
///
/// Foundation's `JSONSerialization` bridges both booleans and numbers to `NSNumber`,
/// which makes it unreliable to tell `true` apart from `1` on decode. This parser keeps
/// the JSON tag (null/bool/number/string/array/object) exactly as written instead.
enum JSONValue: Sendable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])
}

enum JSONParseError: Error, Sendable, CustomStringConvertible {
    case syntax(String, at: Int)

    var description: String {
        switch self {
        case let .syntax(message, at: index):
            return "JSON syntax error at offset \(index): \(message)"
        }
    }
}

extension JSONValue {
    static func parse(_ data: Data) throws -> JSONValue {
        try data.withUnsafeBytes { try parse(utf8: $0.bindMemory(to: UInt8.self)) }
    }

    static func parse(_ text: String) throws -> JSONValue {
        var text = text
        return try text.withUTF8 { try parse(utf8: $0) }
    }

    /// Parses the UTF-8 bytes in place — no decoded copy of the document. Error offsets are
    /// byte offsets.
    private static func parse(utf8 bytes: UnsafeBufferPointer<UInt8>) throws -> JSONValue {
        var parser = Parser(bytes: bytes)
        parser.skipWhitespace()
        let value = try parser.parseValue()
        parser.skipWhitespace()
        guard parser.isAtEnd else {
            throw JSONParseError.syntax("trailing content", at: parser.index)
        }
        return value
    }

    /// Convenience accessors used throughout the decoders below.
    var asObject: [String: JSONValue]? { if case let .object(o) = self { return o } else { return nil } }
    var asArray: [JSONValue]? { if case let .array(a) = self { return a } else { return nil } }
    var asString: String? { if case let .string(s) = self { return s } else { return nil } }
    var asNumber: Double? { if case let .number(n) = self { return n } else { return nil } }
    var asBool: Bool? { if case let .bool(b) = self { return b } else { return nil } }
    var isNull: Bool { if case .null = self { return true } else { return false } }
}

private struct Parser {
    /// Caps object/array nesting so a maliciously (or accidentally) deep document fails with a
    /// clear parse error instead of a stack overflow — both here and in the `Expr`/`Stmt`/
    /// `Node` decoders that recurse over the resulting tree, since their depth is bounded by
    /// whatever depth this parser allowed through.
    static let maxDepth = 500

    let bytes: UnsafeBufferPointer<UInt8>
    var index: Int = 0
    var depth: Int = 0

    var isAtEnd: Bool { index >= bytes.count }

    /// The byte at `index` is `c` (false at the end).
    func at(_ c: Unicode.Scalar) -> Bool { index < bytes.count && bytes[index] == UInt8(ascii: c) }

    mutating func skipWhitespace() {
        while index < bytes.count {
            switch bytes[index] {
            case UInt8(ascii: " "), UInt8(ascii: "\t"), UInt8(ascii: "\n"), UInt8(ascii: "\r"): index += 1
            default: return
            }
        }
    }

    mutating func skipDigits() {
        while index < bytes.count, (UInt8(ascii: "0")...UInt8(ascii: "9")).contains(bytes[index]) { index += 1 }
    }

    mutating func parseValue() throws -> JSONValue {
        guard index < bytes.count else { throw JSONParseError.syntax("unexpected end", at: index) }
        switch bytes[index] {
        case UInt8(ascii: "{"), UInt8(ascii: "["):
            depth += 1
            defer { depth -= 1 }
            guard depth <= Parser.maxDepth else {
                throw JSONParseError.syntax("exceeded max nesting depth (\(Parser.maxDepth))", at: index)
            }
            return at("{") ? try parseObject() : try parseArray()
        case UInt8(ascii: "\""): return .string(try parseString())
        case UInt8(ascii: "t"): try expectLiteral("true"); return .bool(true)
        case UInt8(ascii: "f"): try expectLiteral("false"); return .bool(false)
        case UInt8(ascii: "n"): try expectLiteral("null"); return .null
        default: return .number(try parseNumber())
        }
    }

    mutating func expectLiteral(_ literal: StaticString) throws {
        for expected in UnsafeBufferPointer(start: literal.utf8Start, count: literal.utf8CodeUnitCount) {
            guard index < bytes.count, bytes[index] == expected else {
                throw JSONParseError.syntax("expected literal '\(literal)'", at: index)
            }
            index += 1
        }
    }

    mutating func parseObject() throws -> JSONValue {
        index += 1 // '{'
        var result: [String: JSONValue] = [:]
        skipWhitespace()
        if at("}") {
            index += 1
            return .object(result)
        }
        while true {
            skipWhitespace()
            guard at("\"") else { throw JSONParseError.syntax("expected string key", at: index) }
            let key = try parseString()
            skipWhitespace()
            guard at(":") else { throw JSONParseError.syntax("expected ':'", at: index) }
            index += 1
            skipWhitespace()
            let value = try parseValue()
            result[key] = value
            skipWhitespace()
            guard index < bytes.count else { throw JSONParseError.syntax("unexpected end in object", at: index) }
            if at(",") {
                index += 1
                continue
            } else if at("}") {
                index += 1
                break
            } else {
                throw JSONParseError.syntax("expected ',' or '}'", at: index)
            }
        }
        return .object(result)
    }

    mutating func parseArray() throws -> JSONValue {
        index += 1 // '['
        var result: [JSONValue] = []
        skipWhitespace()
        if at("]") {
            index += 1
            return .array(result)
        }
        while true {
            skipWhitespace()
            result.append(try parseValue())
            skipWhitespace()
            guard index < bytes.count else { throw JSONParseError.syntax("unexpected end in array", at: index) }
            if at(",") {
                index += 1
                continue
            } else if at("]") {
                index += 1
                break
            } else {
                throw JSONParseError.syntax("expected ',' or ']'", at: index)
            }
        }
        return .array(result)
    }

    /// Each raw run between escapes decodes straight from the document's bytes, on its own: a
    /// run is bounded by ASCII `"`/`\`, which ends any broken sequence, so invalid UTF-8 turns
    /// into U+FFFD exactly where decoding the whole document at once put it — also when an
    /// escape in between produces nothing (a dropped lone surrogate).
    mutating func parseString() throws -> String {
        index += 1 // opening quote
        var out = ""
        var run = index
        while true {
            guard index < bytes.count else { throw JSONParseError.syntax("unterminated string", at: index) }
            let c = bytes[index]
            guard c == UInt8(ascii: "\"") || c == UInt8(ascii: "\\") else {
                index += 1
                continue
            }
            out += String(decoding: UnsafeBufferPointer(rebasing: bytes[run..<index]), as: UTF8.self)
            index += 1
            if c == UInt8(ascii: "\"") { return out }
            guard index < bytes.count else { throw JSONParseError.syntax("unterminated escape", at: index) }
            let esc = bytes[index]
            switch esc {
            case UInt8(ascii: "\""), UInt8(ascii: "\\"), UInt8(ascii: "/"): out.unicodeScalars.append(Unicode.Scalar(esc)); index += 1
            case UInt8(ascii: "b"): out.unicodeScalars.append("\u{08}"); index += 1
            case UInt8(ascii: "f"): out.unicodeScalars.append("\u{0C}"); index += 1
            case UInt8(ascii: "n"): out.unicodeScalars.append("\n"); index += 1
            case UInt8(ascii: "r"): out.unicodeScalars.append("\r"); index += 1
            case UInt8(ascii: "t"): out.unicodeScalars.append("\t"); index += 1
            case UInt8(ascii: "u"):
                index += 1
                let hi = try parseHex4()
                if (0xD800...0xDBFF).contains(hi), at("\\"), index + 1 < bytes.count, bytes[index + 1] == UInt8(ascii: "u") {
                    index += 2
                    let lo = try parseHex4()
                    if (0xDC00...0xDFFF).contains(lo) {
                        let combined = 0x10000 + (hi - 0xD800) * 0x400 + (lo - 0xDC00)
                        if let scalar = Unicode.Scalar(combined) { out.unicodeScalars.append(scalar) }
                    }
                } else if let scalar = Unicode.Scalar(hi) {
                    out.unicodeScalars.append(scalar)
                }
            default:
                throw JSONParseError.syntax("invalid escape", at: index)
            }
            run = index
        }
    }

    mutating func parseHex4() throws -> UInt32 {
        guard index + 4 <= bytes.count else { throw JSONParseError.syntax("invalid \\u escape", at: index) }
        var value: UInt32 = 0
        for _ in 0..<4 {
            let b = UInt32(bytes[index])
            let digit: UInt32
            switch b {
            case 0x30...0x39: digit = b - 0x30
            case 0x41...0x46: digit = b - 0x41 + 10
            case 0x61...0x66: digit = b - 0x61 + 10
            default: throw JSONParseError.syntax("invalid hex digit", at: index)
            }
            value = value * 16 + digit
            index += 1
        }
        return value
    }

    mutating func parseNumber() throws -> Double {
        let start = index
        if at("-") { index += 1 }
        skipDigits()
        if at(".") {
            index += 1
            skipDigits()
        }
        if at("e") || at("E") {
            index += 1
            if at("+") || at("-") { index += 1 }
            skipDigits()
        }
        guard start < index else { throw JSONParseError.syntax("invalid number", at: index) }
        let text = String(decoding: UnsafeBufferPointer(rebasing: bytes[start..<index]), as: UTF8.self)
        guard let value = Double(text) else { throw JSONParseError.syntax("invalid number literal '\(text)'", at: start) }
        return value
    }
}
