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
        var parser = Parser(scalars: Array(String(decoding: data, as: UTF8.self).unicodeScalars))
        parser.skipWhitespace()
        let value = try parser.parseValue()
        parser.skipWhitespace()
        guard parser.isAtEnd else {
            throw JSONParseError.syntax("trailing content", at: parser.index)
        }
        return value
    }

    static func parse(_ text: String) throws -> JSONValue {
        try parse(Data(text.utf8))
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
    let scalars: [Unicode.Scalar]
    var index: Int = 0

    var isAtEnd: Bool { index >= scalars.count }

    mutating func skipWhitespace() {
        while index < scalars.count {
            switch scalars[index] {
            case " ", "\t", "\n", "\r": index += 1
            default: return
            }
        }
    }

    mutating func parseValue() throws -> JSONValue {
        guard index < scalars.count else { throw JSONParseError.syntax("unexpected end", at: index) }
        switch scalars[index] {
        case "{": return try parseObject()
        case "[": return try parseArray()
        case "\"": return .string(try parseString())
        case "t": try expectLiteral("true"); return .bool(true)
        case "f": try expectLiteral("false"); return .bool(false)
        case "n": try expectLiteral("null"); return .null
        default: return .number(try parseNumber())
        }
    }

    mutating func expectLiteral(_ literal: String) throws {
        for expected in literal.unicodeScalars {
            guard index < scalars.count, scalars[index] == expected else {
                throw JSONParseError.syntax("expected literal '\(literal)'", at: index)
            }
            index += 1
        }
    }

    mutating func parseObject() throws -> JSONValue {
        index += 1 // '{'
        var result: [String: JSONValue] = [:]
        skipWhitespace()
        if index < scalars.count, scalars[index] == "}" {
            index += 1
            return .object(result)
        }
        while true {
            skipWhitespace()
            guard index < scalars.count, scalars[index] == "\"" else {
                throw JSONParseError.syntax("expected string key", at: index)
            }
            let key = try parseString()
            skipWhitespace()
            guard index < scalars.count, scalars[index] == ":" else {
                throw JSONParseError.syntax("expected ':'", at: index)
            }
            index += 1
            skipWhitespace()
            let value = try parseValue()
            result[key] = value
            skipWhitespace()
            guard index < scalars.count else { throw JSONParseError.syntax("unexpected end in object", at: index) }
            if scalars[index] == "," {
                index += 1
                continue
            } else if scalars[index] == "}" {
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
        if index < scalars.count, scalars[index] == "]" {
            index += 1
            return .array(result)
        }
        while true {
            skipWhitespace()
            result.append(try parseValue())
            skipWhitespace()
            guard index < scalars.count else { throw JSONParseError.syntax("unexpected end in array", at: index) }
            if scalars[index] == "," {
                index += 1
                continue
            } else if scalars[index] == "]" {
                index += 1
                break
            } else {
                throw JSONParseError.syntax("expected ',' or ']'", at: index)
            }
        }
        return .array(result)
    }

    mutating func parseString() throws -> String {
        index += 1 // opening quote
        var scalarsOut = String.UnicodeScalarView()
        while true {
            guard index < scalars.count else { throw JSONParseError.syntax("unterminated string", at: index) }
            let c = scalars[index]
            if c == "\"" {
                index += 1
                break
            } else if c == "\\" {
                index += 1
                guard index < scalars.count else { throw JSONParseError.syntax("unterminated escape", at: index) }
                let esc = scalars[index]
                switch esc {
                case "\"": scalarsOut.append("\""); index += 1
                case "\\": scalarsOut.append("\\"); index += 1
                case "/": scalarsOut.append("/"); index += 1
                case "b": scalarsOut.append(Unicode.Scalar(0x08)!); index += 1
                case "f": scalarsOut.append(Unicode.Scalar(0x0C)!); index += 1
                case "n": scalarsOut.append("\n"); index += 1
                case "r": scalarsOut.append("\r"); index += 1
                case "t": scalarsOut.append("\t"); index += 1
                case "u":
                    index += 1
                    let hi = try parseHex4()
                    if (0xD800...0xDBFF).contains(hi), index < scalars.count, scalars[index] == "\\",
                       index + 1 < scalars.count, scalars[index + 1] == "u" {
                        index += 2
                        let lo = try parseHex4()
                        if (0xDC00...0xDFFF).contains(lo) {
                            let combined = 0x10000 + (hi - 0xD800) * 0x400 + (lo - 0xDC00)
                            if let scalar = Unicode.Scalar(combined) { scalarsOut.append(scalar) }
                        }
                    } else if let scalar = Unicode.Scalar(hi) {
                        scalarsOut.append(scalar)
                    }
                default:
                    throw JSONParseError.syntax("invalid escape", at: index)
                }
            } else {
                scalarsOut.append(c)
                index += 1
            }
        }
        return String(scalarsOut)
    }

    mutating func parseHex4() throws -> UInt32 {
        guard index + 4 <= scalars.count else { throw JSONParseError.syntax("invalid \\u escape", at: index) }
        var value: UInt32 = 0
        for _ in 0..<4 {
            guard let digit = scalars[index].hexDigitValue else {
                throw JSONParseError.syntax("invalid hex digit", at: index)
            }
            value = value * 16 + UInt32(digit)
            index += 1
        }
        return value
    }

    mutating func parseNumber() throws -> Double {
        let start = index
        if index < scalars.count, scalars[index] == "-" { index += 1 }
        while index < scalars.count, scalars[index].isDigit { index += 1 }
        if index < scalars.count, scalars[index] == "." {
            index += 1
            while index < scalars.count, scalars[index].isDigit { index += 1 }
        }
        if index < scalars.count, scalars[index] == "e" || scalars[index] == "E" {
            index += 1
            if index < scalars.count, scalars[index] == "+" || scalars[index] == "-" { index += 1 }
            while index < scalars.count, scalars[index].isDigit { index += 1 }
        }
        guard start < index else { throw JSONParseError.syntax("invalid number", at: index) }
        let text = String(String.UnicodeScalarView(scalars[start..<index]))
        guard let value = Double(text) else { throw JSONParseError.syntax("invalid number literal '\(text)'", at: start) }
        return value
    }
}

private extension Unicode.Scalar {
    var isDigit: Bool { ("0"..."9").contains(self) }
    var hexDigitValue: UInt32? {
        switch value {
        case 0x30...0x39: return value - 0x30
        case 0x41...0x46: return value - 0x41 + 10
        case 0x61...0x66: return value - 0x61 + 10
        default: return nil
        }
    }
}
