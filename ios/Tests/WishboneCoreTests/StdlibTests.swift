import Testing
@testable import WishboneCore

@Suite struct StdlibTests {
    // MARK: math

    @Test func roundingIsHalfAwayFromZero() {
        #expect(Stdlib.round(2.5) == 3)
        #expect(Stdlib.round(-2.5) == -3)
        #expect(Stdlib.round(2.4) == 2)
        #expect(Stdlib.round(-0.4) == 0)
        #expect(Stdlib.round(0.49999999999999994) == 0)
    }

    @Test func interpolatePiecewiseLinearClamped() {
        #expect(Stdlib.interpolate(50, [0, 100], [1, 0]) == 0.5)
        #expect(Stdlib.interpolate(-20, [0, 100], [1, 0]) == 1)
        #expect(Stdlib.interpolate(150, [0, 100, 200], [0, 10, 0]) == 5)
        #expect(Stdlib.interpolate(1, [0], [3]) == 3) // fewer than 2 stops -> output.first
    }

    // MARK: strings (code-point based)

    @Test func lengthCountsCodePointsNotGraphemes() {
        #expect(Stdlib.length("héllo") == 5)
        #expect(Stdlib.length("😀a") == 2)
    }

    @Test func trimStripsExactWhitespaceSet() {
        #expect(Stdlib.trim(" Hi ") == "Hi")
        #expect(Stdlib.trim("\u{00A0}x\u{3000}") == "x")
    }

    @Test func splitKeepsEmptyPartsAndEmptySeparatorSplitsCodePoints() {
        #expect(Stdlib.split("a,,b", ",") == [.string("a"), .string(""), .string("b")])
        #expect(Stdlib.split("ab", "") == [.string("a"), .string("b")])
    }

    @Test func substringUsesCodePointIndicesAndClamps() {
        #expect(Stdlib.substring("abc", 1, nil) == "bc")
        #expect(Stdlib.substring("abc", -5, 2) == "ab")
        #expect(Stdlib.substring("abc", 2, 1) == "")
    }

    @Test func numParsesStrictDecimalOrNil() {
        #expect(Stdlib.num(" 42.5 ") == 42.5)
        #expect(Stdlib.num("abc") == nil)
        #expect(Stdlib.num("1e3") == 1000)
        #expect(Stdlib.num("") == nil)
        #expect(Stdlib.num("123.") == 123) // matches the updated regex \d+\.?\d*
        #expect(Stdlib.num("NaN") == nil)
        #expect(Stdlib.num("Infinity") == nil)
    }

    @Test func matchesIsWholeStringMatch() {
        #expect(Stdlib.matches("test@example.com", "[^@]+@[^@]+\\.[a-z]+"))
        #expect(!Stdlib.matches("test@", "[^@]+@[^@]+\\.[a-z]+"))
        #expect(!Stdlib.matches("abc", "b"))
    }

    // MARK: lists

    @Test func codePointCompareOrdersUppercaseBeforeLowercase() {
        let xs = ["b", "a", "B"]
        let sorted = xs.sorted { CodePoints.compare($0, $1) < 0 }
        #expect(sorted == ["B", "a", "b"])
    }

    @Test func rangeCapsAndHandlesEdgeCases() {
        #expect(Stdlib.range(4) == [.number(0), .number(1), .number(2), .number(3)])
        #expect(Stdlib.range(-1) == [])
        #expect(Stdlib.range(2.9) == [.number(0), .number(1)])
        #expect(Stdlib.range(.nan) == [])
        #expect(Stdlib.range(.infinity) == []) // non-finite -> []
    }

    @Test func withMergesOnlyPatchFieldsPreservingOthers() {
        let r = Value.record(["name": .string("Ann"), "age": .number(30)])
        let patched = Stdlib.with(r, .record(["age": .number(31)]))
        #expect(patched == .record(["name": .string("Ann"), "age": .number(31)]))
    }

    @Test func indexOfUsesDeepEquality() {
        let xs: [Value] = [.record(["id": .number(1)]), .record(["id": .number(2)])]
        #expect(Stdlib.indexOf(xs, .record(["id": .number(2)])) == 1)
        #expect(Stdlib.indexOf(xs, .record(["id": .number(9)])) == -1)
    }

    @Test func setAndRemoveAtOutOfRangeAreUnchangedCopies() {
        let xs: [Value] = [.number(1), .number(2)]
        #expect(Stdlib.set(xs, 5, .number(9)) == xs)
        #expect(Stdlib.removeAt(xs, 5) == xs)
        #expect(Stdlib.set(xs, 0.5, .number(9)) == xs) // non-integer index -> unchanged
    }

    // MARK: tokens

    @Test func alphaClampsMultipliesAndOmitsWhenOne() {
        let c = Value.record(["$color": .string("bg1")])
        let half = Stdlib.alpha(c, 0.5)
        #expect(half == .record(["$color": .string("bg1"), "alpha": .number(0.5)]))
        let quarter = Stdlib.alpha(half, 0.5)
        #expect(quarter == .record(["$color": .string("bg1"), "alpha": .number(0.25)]))
        #expect(Stdlib.alpha(c, 1.0) == c) // alpha(c, 1) == c
        #expect(Stdlib.alpha(c, 5.0) == c) // a clamped to [0,1] first, so product is 1 -> alpha key omitted
        // a is clamped alone (to 1), not the product: 0.5 existing * clamp(2.0)=1 -> still 0.5, unchanged
        #expect(Stdlib.alpha(half, 2.0) == half)
    }

    @Test func springAndCurveConstructMotionRecords() {
        #expect(Stdlib.spring(300, 0.8) == .record(["$motion": .record(["kind": .string("spring"), "response": .number(300), "damping": .number(0.8)])]))
        #expect(Stdlib.ease(200) == .record(["$motion": .record(["kind": .string("curve"), "duration": .number(200), "bezier": .list([.number(0.42), .number(0), .number(0.58), .number(1)])])]))
        #expect(Stdlib.linear(100) == .record(["$motion": .record(["kind": .string("curve"), "duration": .number(100), "bezier": .list([.number(0), .number(0), .number(1), .number(1)])])]))
    }
}
