import Foundation
import Testing
@testable import WishboneCore

@Suite struct JSONValueTests {
    /// Every JSON file the repo ships (screens, templates, conformance vectors, tokens, backend
    /// testdata) decodes to the same tree `JSONSerialization` builds.
    @Test func everyRepoFixtureDecodesLikeFoundation() throws {
        let files = ["contract", "backend"].flatMap { dir -> [URL] in
            let enumerator = FileManager.default.enumerator(at: repoRoot.appendingPathComponent(dir), includingPropertiesForKeys: nil)
            return (enumerator?.allObjects as? [URL] ?? []).filter { $0.pathExtension == "json" }
        }
        #expect(files.count > 50)
        for url in files {
            let data = try Data(contentsOf: url)
            let reference = try JSONSerialization.jsonObject(with: data, options: .fragmentsAllowed)
            #expect(same(try JSONValue.parse(data), reference), "\(url.path)")
        }
    }

    @Test func stringEscapesSurrogatesAndInvalidUTF8() throws {
        func scalars(_ bytes: [UInt8]) throws -> [UInt32] {
            guard case let .string(s) = try JSONValue.parse(Data(bytes)) else { return [] }
            return s.unicodeScalars.map(\.value)
        }
        func scalars(_ text: String) throws -> [UInt32] { try scalars(Array(text.utf8)) }

        #expect(try scalars(#""a\"\\\/\b\f\n\r\t\u00e9\ud83d\ude00z""#)
            == [0x61, 0x22, 0x5C, 0x2F, 0x08, 0x0C, 0x0A, 0x0D, 0x09, 0xE9, 0x1F600, 0x7A])
        #expect(try scalars("\"привет 😀\"") == "привет 😀".unicodeScalars.map(\.value))
        #expect(try scalars(#""\ud800x""#) == [0x78]) // lone high surrogate: dropped
        #expect(try scalars(#""\udc00""#) == []) // lone low surrogate: dropped
        #expect(try scalars(#""\ud800\u0041""#) == []) // high + non-low: both dropped
        // Invalid UTF-8 is repaired to U+FFFD per maximal subpart, as `String(decoding:)` does.
        #expect(try scalars([0x22, 0x61, 0xFF, 0x62, 0xE2, 0x82, 0x22]) == [0x61, 0xFFFD, 0x62, 0xFFFD])
        #expect(try scalars([0x22, 0xE2, 0x82, 0x5C, 0x6E, 0x22]) == [0xFFFD, 0x0A])
        #expect(try scalars([0x22, 0xE2, 0x82, 0x5C, 0x75, 0x30, 0x30, 0x61, 0x63, 0x22]) == [0xFFFD, 0xAC])
        // A dropped lone surrogate still splits the bytes around it: "§" (C2 A7) cut by `\ud800`.
        #expect(try scalars([0x22, 0xC2] + Array(#"\ud800"#.utf8) + [0xA7, 0x22]) == [0xFFFD, 0xFFFD])
    }

    @Test func numbersLiteralsAndSyntaxErrors() throws {
        #expect(try JSONValue.parse("-12.5e2").asNumber == -1250)
        #expect(try JSONValue.parse(" 1E+2 ").asNumber == 100)
        #expect(try JSONValue.parse("-0").asNumber?.sign == .minus)
        // Lenient forms the parser has always accepted (it hands the scanned text to `Double`).
        #expect(try JSONValue.parse(".5").asNumber == 0.5)
        #expect(try JSONValue.parse("01").asNumber == 1)
        #expect(try JSONValue.parse("1.").asNumber == 1)
        #expect(try JSONValue.parse("[true,false,null]").asArray?.count == 3)
        #expect(try JSONValue.parse(#"{"a":1,"a":2}"#).asObject?["a"]?.asNumber == 2) // last duplicate wins
        for bad in ["", "-", "+1", "1e", "tru", "[1,]", #"{"a":1,}"#, "[1 2]", "1 2", #""abc"#, #""\x""#, #""\u12""#, "\u{FEFF}1"] {
            #expect(throws: JSONParseError.self, "\(bad.debugDescription)") { try JSONValue.parse(bad) }
        }
        #expect(throws: JSONParseError.self) { try JSONValue.parse(Data()) }
    }
}

/// Structural equality with a Foundation JSON tree (booleans told apart from numbers,
/// strings by code points).
private func same(_ value: JSONValue, _ reference: Any) -> Bool {
    switch value {
    case .null:
        return reference is NSNull
    case let .bool(b):
        guard let n = reference as? NSNumber, CFGetTypeID(n) == CFBooleanGetTypeID() else { return false }
        return n.boolValue == b
    case let .number(x):
        guard let n = reference as? NSNumber, CFGetTypeID(n) != CFBooleanGetTypeID() else { return false }
        return n.doubleValue == x
    case let .string(s):
        guard let r = reference as? String else { return false }
        return s.unicodeScalars.elementsEqual(r.unicodeScalars)
    case let .array(items):
        guard let r = reference as? [Any], r.count == items.count else { return false }
        return zip(items, r).allSatisfy { same($0, $1) }
    case let .object(fields):
        guard let r = reference as? [String: Any], r.count == fields.count else { return false }
        return fields.allSatisfy { key, v in r[key].map { same(v, $0) } ?? false }
    }
}
