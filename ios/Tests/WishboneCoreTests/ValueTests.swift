import Testing
@testable import WishboneCore

@Suite struct ValueTests {
    @Test func equalityIsStructuralAndOrderInsensitive() {
        #expect(Value.record(["a": .number(1), "b": .number(2)]) == Value.record(["b": .number(2), "a": .number(1)]))
        #expect(Value.record(["a": .number(1)]) != Value.record(["a": .number(2)]))
        #expect(Value.list([.number(1), .number(2)]) == Value.list([.number(1), .number(2)]))
    }

    @Test func numberEqualityMatchesIEEE754() {
        #expect(Value.number(.nan) != Value.number(.nan))
        #expect(Value.number(0) == Value.number(-0.0))
    }

    @Test func stringEqualityIsCodePointBased() {
        // "e" + combining acute (2 scalars) must NOT equal precomposed "é" (1 scalar).
        let combining = "e\u{0301}"
        let precomposed = "\u{00e9}"
        #expect(Value.string(combining) != Value.string(precomposed))
        #expect(combining == precomposed) // Swift's own `==` normalizes — the trap this guards against.
    }

    @Test func decodesLiteralsFromJSON() throws {
        #expect(try Value(json: "true") == .bool(true))
        #expect(try Value(json: "1") == .number(1))
        #expect(try Value(json: "null") == .null)
        #expect(try Value(json: "\"hi\"") == .string("hi"))
        #expect(try Value(json: "[1,2,3]") == .list([.number(1), .number(2), .number(3)]))
        #expect(try Value(json: "{\"a\":1,\"b\":\"x\"}") == .record(["a": .number(1), "b": .string("x")]))
    }

    @Test func trueAndOneDecodeDistinctly() throws {
        // The reason for a hand-rolled parser: JSONSerialization bridges both to NSNumber.
        #expect(try Value(json: "true") != (try Value(json: "1")))
    }

    @Test func settingPathUpdatesNestedFieldsAndIndices() {
        let v = Value.record(["items": .list([.record(["done": .bool(false)])])])
        let updated = v.settingPath([.string("items"), .number(0), .string("done")], to: .bool(true))
        #expect(updated == .record(["items": .list([.record(["done": .bool(true)])])]))
    }

    @Test func settingPathIsNoOpOnMissingFieldOOBOrNull() {
        let v = Value.record(["items": .list([.record(["done": .bool(false)])])])
        // missing field
        #expect(v.settingPath([.string("nope")], to: .bool(true)) == v)
        // out of range index
        #expect(v.settingPath([.string("items"), .number(5), .string("done")], to: .bool(true)) == v)
        // null along the path
        let withNull = Value.record(["x": .null])
        #expect(withNull.settingPath([.string("x"), .string("y")], to: .number(1)) == withNull)
    }

    @Test func emptyPathReplacesWholeValue() {
        #expect(Value.number(1).settingPath([], to: .number(2)) == .number(2))
    }

    @Test func strOfRecordIsCompactJSONWithKeysSortedByCodePoint() {
        // stdlib.json: "record keys sorted by code point"
        let v = Value.record(["b": .number(2), "a": .string("x")])
        #expect(Format.str(v) == "{\"a\":\"x\",\"b\":2}")
    }

    @Test func strOfStringIsIdentity() {
        #expect(Format.str(.string("hi \"there\"")) == "hi \"there\"")
    }

    @Test func strEscapesOnlyQuoteBackslashAndControlChars() {
        // contract/conformance/expr.json str.record.sorted: '/' is raw, not escaped.
        #expect(Format.str(.record(["b": .number(1), "a": .string("x/y")])) == "{\"a\":\"x/y\",\"b\":1}")
    }

    @Test func combiningSequenceIsTwoCodePointsNotOneGrapheme() {
        // contract/conformance/expr.json str.combining.length / str.combining.eq
        #expect(CodePoints.length("e\u{0301}") == 2)
        #expect(!CodePoints.equal("e\u{0301}", "\u{00E9}"))
    }

    @Test func astralComparesAboveBMP() {
        // U+FF01 < U+1F600 by code point (even though UTF-16 lexicographic order would differ)
        #expect(CodePoints.compare("\u{FF01}", "\u{1F600}") < 0)
    }
}
