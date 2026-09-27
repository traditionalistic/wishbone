import Testing
@testable import WishboneCore

@Suite struct FormatTests {
    // MARK: numberToString (str/interpolation/join rule)

    @Test func integralSmallHasNoFraction() {
        #expect(Format.numberToString(1800) == "1800")
        #expect(Format.numberToString(-2) == "-2")
    }

    @Test func negativeZeroPrintsZero() {
        #expect(Format.numberToString(-0.0) == "0")
    }

    @Test func nonIntegralUsesSixFractionDigitsTrimmed() {
        #expect(Format.numberToString(0.1 + 0.2) == "0.3")
        #expect(Format.numberToString(1.0 / 3.0) == "0.333333")
        #expect(Format.numberToString(2.0 / 3.0) == "0.666667")
        #expect(Format.numberToString(-0.5) == "-0.5")
    }

    @Test func bigIntegerAtBoundary() {
        #expect(Format.numberToString(1_000_000_000_000_000) == "1000000000000000")
    }

    @Test func specials() {
        #expect(Format.numberToString(.nan) == "NaN")
        #expect(Format.numberToString(.infinity) == "Infinity")
        #expect(Format.numberToString(-.infinity) == "-Infinity")
    }

    // MARK: formatNumber

    @Test func formatNumberGroupsAndRounds() {
        #expect(Format.formatNumber(1990, 0) == "1\u{00A0}990")
        #expect(Format.formatNumber(-1234.5, 1) == "-1\u{00A0}234.5")
        #expect(Format.formatNumber(999.999, 2) == "1\u{00A0}000.00")
        #expect(Format.formatNumber(0.125, 2) == "0.13")
        #expect(Format.formatNumber(0.05, 3) == "0.050")
        #expect(Format.formatNumber(1_234_567.891, 2) == "1\u{00A0}234\u{00A0}567.89")
        #expect(Format.formatNumber(999, 0) == "999")
        #expect(Format.formatNumber(0.5, 0) == "1")
        #expect(Format.formatNumber(-0.5, 0) == "-1")
    }

    @Test func formatNumberBinaryDoubleSurprises() {
        // Updated contract (commit b85015c): 2.675*100 == 267.5 exactly in binary64.
        #expect(Format.formatNumber(2.675, 2) == "2.68")
        // 1.005*100 == 100.49999999999999 in binary64.
        #expect(Format.formatNumber(1.005, 2) == "1.00")
    }

    @Test func formatNumberNegativeZeroNeverPrintsASign() {
        #expect(Format.formatNumber(-0.4, 0) + "|" + Format.formatNumber(-0.001, 2) == "0|0.00")
    }

    @Test func formatNumberGroupsExactThousand() {
        #expect(Format.formatNumber(1000, 0) == "1\u{00A0}000")
    }

    @Test func formatNumberHandlesHugeMagnitudes() {
        #expect(Format.formatNumber(100_000_000_000_000_000, 1) == "100\u{00A0}000\u{00A0}000\u{00A0}000\u{00A0}000\u{00A0}000.0")
    }

    @Test func numberToStringHandlesHugeAndTinyMagnitudes() {
        #expect(Format.numberToString(100_000_000_000_000_000) == "100000000000000000")
        #expect(Format.numberToString(-0.0000001) == "0")
    }

    // MARK: formatDuration

    @Test func formatDurationStyles() {
        #expect(Format.formatDuration(3_723_000, "hms") == "01:02:03")
        #expect(Format.formatDuration(59999, "ms") == "00:59")
        #expect(Format.formatDuration(-5, "hms") == "00:00:00")
        #expect(Format.formatDuration(90_061_000, "dhms") == "1:01:01:01")
        #expect(Format.formatDuration(360_000_000, "hms") == "100:00:00")
        #expect(Format.formatDuration(.nan, "hms") == "00:00:00")
    }

    // MARK: formatDate

    @Test func formatDateStyles() {
        #expect(Format.formatDate(0, "dateTime", timeZone: "UTC") == "01.01.1970 00:00")
        #expect(Format.formatDate(1_790_000_000_000, "date", timeZone: "UTC") == "21.09.2026")
        #expect(Format.formatDate(0, "time", timeZone: "Europe/Moscow") == "03:00")
        #expect(Format.formatDate(1_783_000_000_000, "dateTime", timeZone: "Europe/Berlin") == "02.07.2026 15:46")
    }

    @Test func formatDateUnknownZoneFallsBackToUTC() {
        #expect(Format.formatDate(0, "time", timeZone: "Not/AZone") == "00:00")
    }

    // MARK: plural

    private let ruForms: [String: Value] = [
        "one": .string("# билет"), "few": .string("# билета"),
        "many": .string("# билетов"), "other": .string("# билета"),
    ]

    @Test func pluralRussianCategories() {
        #expect(Format.plural(1, forms: ruForms, locale: "ru") == "1 билет")
        #expect(Format.plural(2, forms: ruForms, locale: "ru") == "2 билета")
        #expect(Format.plural(5, forms: ruForms, locale: "ru") == "5 билетов")
        #expect(Format.plural(11, forms: ruForms, locale: "ru") == "11 билетов")
        #expect(Format.plural(21, forms: ruForms, locale: "ru") == "21 билет")
        #expect(Format.plural(1.5, forms: ruForms, locale: "ru") == "1.5 билета")
        #expect(Format.plural(0, forms: ruForms, locale: "ru") == "0 билетов")
    }

    @Test func pluralEnglishAndOthers() {
        let enForms: [String: Value] = ["one": .string("# day"), "other": .string("# days")]
        #expect(Format.plural(1, forms: enForms, locale: "en") == "1 day")
        #expect(Format.plural(3, forms: enForms, locale: "en") == "3 days")
        #expect(Format.plural(-1, forms: enForms, locale: "en") == "-1 day") // |n| == 1

        let ttForms: [String: Value] = ["one": .string("бер"), "other": .string("күп")]
        #expect(Format.plural(1, forms: ttForms, locale: "tt") == "күп")
    }

    @Test func pluralMissingFormFallsBackToOther() {
        let forms: [String: Value] = ["one": .string("one"), "other": .string("other")]
        #expect(Format.plural(2, forms: forms, locale: "ru") == "other") // ru "few" missing -> other
    }

    @Test func pluralLanguagePrefixBeforeDashOrUnderscore() {
        #expect(Format.pluralLanguage("en-US") == "en")
        #expect(Format.pluralLanguage("ru_RU") == "ru")
        #expect(Format.pluralLanguage("KK") == "kk")
    }

    @Test func pluralUsesAbsoluteValueForEnLikeLanguages() {
        let enForms: [String: Value] = ["one": .string("# day"), "other": .string("# days")]
        #expect(Format.plural(-1, forms: enForms, locale: "en") == "-1 day")
    }

    @Test func pluralIgnoresRegionSubtag() {
        let forms: [String: Value] = ["one": .string("a"), "few": .string("b"), "many": .string("c"), "other": .string("d")]
        #expect(Format.plural(2, forms: forms, locale: "ru-RU") == "b")
    }

    @Test func pluralKazakhIsEnLike() {
        let forms: [String: Value] = ["one": .string("бір"), "other": .string("көп")]
        #expect(Format.plural(1, forms: forms, locale: "kk") == "бір")
    }
}
