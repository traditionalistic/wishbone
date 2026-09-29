package app.wishbone.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/** Vectors transcribed from contract/conformance/format.json + expr.json `str.*` cases. */
class FormatTest {

    @Test
    @Suppress("DIVISION_BY_ZERO")
    fun `numberToString matches str vectors`() {
        assertEquals("0.3", Format.numberToString(0.1 + 0.2))
        assertEquals("Infinity -Infinity NaN", "${Format.numberToString(1.0 / 0)} ${Format.numberToString(-1.0 / 0)} ${Format.numberToString(0.0 / 0)}")
        assertEquals("1000000000000000", Format.numberToString(1000000000000000.0))
        assertEquals("100000000000000000", Format.numberToString(100000000000000000.0))
        assertEquals("-0.5 0.333333 0.666667", "${Format.numberToString(-0.5)} ${Format.numberToString(1.0 / 3)} ${Format.numberToString(2.0 / 3)}")
        assertEquals("0", Format.numberToString(-0.0))
        assertEquals("0", Format.numberToString(-0.0000001))
    }

    @Test
    fun `round half away from zero, never half-even`() {
        assertEquals(3.0, Format.roundHalfAwayFromZero(2.5))
        assertEquals(-3.0, Format.roundHalfAwayFromZero(-2.5))
        assertEquals(2.0, Format.roundHalfAwayFromZero(2.4))
        // delta-based assertEquals compares by subtraction, not JUnit's exact/Double.compare path,
        // so it (correctly, per IEEE) treats -0.0 and 0.0 as equal: round(-0.4) legitimately gives -0.0.
        assertEquals(0.0, Format.roundHalfAwayFromZero(-0.4), 0.0)
        assertEquals(0.0, Format.roundHalfAwayFromZero(0.49999999999999994), 0.0)
    }

    @Test
    fun `formatNumber grouping, rounding, binary-double surprises`() {
        assertEquals("1 990", Format.formatNumber(1990.0, 0.0))
        assertEquals("-1 234.5", Format.formatNumber(-1234.5, 1.0))
        assertEquals("1 000.00", Format.formatNumber(999.999, 2.0))
        assertEquals("0.13", Format.formatNumber(0.125, 2.0))
        assertEquals("2.68", Format.formatNumber(2.675, 2.0)) // 2.675*100 == 267.5 in binary64
        assertEquals("1.00", Format.formatNumber(1.005, 2.0)) // 1.005*100 == 100.49999999999999
        assertEquals("0|0.00", "${Format.formatNumber(-0.4, 0.0)}|${Format.formatNumber(-0.001, 2.0)}")
        assertEquals("1 000", Format.formatNumber(1000.0, 0.0))
        assertEquals("100 000 000 000 000 000.0", Format.formatNumber(100000000000000000.0, 1.0))
        assertEquals("999|1|-1", "${Format.formatNumber(999.0, 0.0)}|${Format.formatNumber(0.5, 0.0)}|${Format.formatNumber(-0.5, 0.0)}")
        assertEquals("0.050", Format.formatNumber(0.05, 3.0))
        assertEquals("1 234 567.89", Format.formatNumber(1234567.891, 2.0))
    }

    @Test
    fun `formatDuration styles`() {
        assertEquals("01:02:03", Format.formatDuration(3723000.0, "hms"))
        assertEquals("00:59", Format.formatDuration(59999.0, "ms"))
        assertEquals("00:00:00", Format.formatDuration(-5.0, "hms"))
        assertEquals("1:01:01:01", Format.formatDuration(90061000.0, "dhms"))
        assertEquals("100:00:00", Format.formatDuration(360000000.0, "hms"))
        assertEquals("00:00:10", Format.formatDuration(20000.0 - 10000.0, "hms"))
    }

    @Test
    fun `formatDate in the given time zone, unknown zone falls back to UTC`() {
        assertEquals("01.01.1970 00:00", Format.formatDate(0.0, "dateTime", "UTC"))
        assertEquals("21.09.2026", Format.formatDate(1790000000000.0, "date", "UTC"))
        assertEquals("03:00", Format.formatDate(0.0, "time", "Europe/Moscow"))
        assertEquals("02.07.2026 15:46", Format.formatDate(1783000000000.0, "dateTime", "Europe/Berlin"))
        assertEquals("", Format.formatDate(Double.NaN, "date", "UTC"))
    }

    @Test
    fun `whole numbers print exact digits through str and formatNumber (fast-path edges)`() {
        val cases = listOf(
            0.0 to "0", -0.0 to "0", 1.0 to "1", -1.0 to "-1", 42.0 to "42",
            999999999999999.0 to "999999999999999", 1e15 to "1000000000000000", -1e15 to "-1000000000000000",
            9.2e18 to "9200000000000000000", 1e19 to "10000000000000000000", -1e19 to "-10000000000000000000",
            Double.NaN to "NaN", Double.POSITIVE_INFINITY to "Infinity", Double.NEGATIVE_INFINITY to "-Infinity",
        )
        for ((x, expected) in cases) {
            assertEquals(expected, Format.str(Value.Num(x)), "str($x)")
            assertEquals(expected, Format.numberToString(x), "numberToString($x)")
        }
        assertEquals("1 000 000 000 000 000".replace(' ', '\u00a0'), Format.formatNumber(1e15, 0.0))
        assertEquals("-1 000 000 000 000 000.00".replace(' ', '\u00a0'), Format.formatNumber(-1e15, 2.0))
        assertEquals("999 999 999 999 999".replace(' ', '\u00a0'), Format.formatNumber(999999999999999.0, 0.0))
        assertEquals("9 200 000 000 000 000 000.0".replace(' ', '\u00a0'), Format.formatNumber(9.2e18, 1.0))
        assertEquals("10 000 000 000 000 000 000".replace(' ', '\u00a0'), Format.formatNumber(1e19, 0.0))
        assertEquals("-42|0|1 000".replace(' ', '\u00a0'), "${Format.formatNumber(-42.0, 0.0)}|${Format.formatNumber(-0.0, 0.0)}|${Format.formatNumber(1000.0, 0.0)}")
        assertEquals("-9 007 199 254 740 992|NaN|-Infinity".replace(' ', '\u00a0'), "${Format.formatNumber(-9007199254740992.0, 0.0)}|${Format.formatNumber(Double.NaN, 2.0)}|${Format.formatNumber(Double.NEGATIVE_INFINITY, 0.0)}")
        assertEquals("1 234 567 890 123.50".replace(' ', '\u00a0'), Format.formatNumber(1234567890123.5, 2.0))
    }

    @Test
    fun `formatDate alternating zones and styles is stable`() {
        repeat(3) {
            assertEquals("03:00", Format.formatDate(0.0, "time", "Europe/Moscow"))
            assertEquals("00:00", Format.formatDate(0.0, "time", "UTC"))
            assertEquals("01.01.1970 09:00", Format.formatDate(0.0, "dateTime", "Asia/Tokyo"))
            assertEquals("31.12.1969", Format.formatDate(0.0, "date", "America/New_York"))
            assertEquals("01.01.1970", Format.formatDate(0.0, "date", "No/Such_Zone"))
            assertEquals("01.01.1970 00:00", Format.formatDate(0.0, "other", ""))
        }
    }

    @Test
    fun `formatDuration edge widths`() {
        assertEquals("00:00", Format.formatDuration(Double.NaN, "ms"))
        assertEquals("1000:05", Format.formatDuration(60_005_000.0, "ms"))
        assertEquals("0:00:00:00", Format.formatDuration(0.0, "dhms"))
        assertEquals("12:23:59:59", Format.formatDuration(((12 * 24 + 23) * 3600 + 59 * 60 + 59) * 1000.0, "dhms"))
        assertEquals("07:08:09", Format.formatDuration(((7 * 60 + 8) * 60 + 9) * 1000.0, "hms"))
    }

    @Test
    fun `plural CLDR subset`() {
        val ru = mapOf(
            "one" to Value.Str("# билет"), "few" to Value.Str("# билета"),
            "many" to Value.Str("# билетов"), "other" to Value.Str("# билета"),
        )
        assertEquals("1 билет", Plural.format(1.0, ru, "ru"))
        assertEquals("2 билета", Plural.format(2.0, ru, "ru"))
        assertEquals("5 билетов", Plural.format(5.0, ru, "ru"))
        assertEquals("11 билетов", Plural.format(11.0, ru, "ru"))
        assertEquals("21 билет", Plural.format(21.0, ru, "ru"))
        assertEquals("22 билета", Plural.format(22.0, ru, "ru"))
        assertEquals("111 билетов", Plural.format(111.0, ru, "ru"))
        assertEquals("1.5 билета", Plural.format(1.5, ru, "ru"))
        assertEquals("0 билетов", Plural.format(0.0, ru, "ru"))
        assertEquals("14 билетов", Plural.format(14.0, ru, "ru"))

        val en = mapOf("one" to Value.Str("# day"), "other" to Value.Str("# days"))
        assertEquals("1 day", Plural.format(1.0, en, "en"))
        assertEquals("3 days", Plural.format(3.0, en, "en"))
        assertEquals("0 days", Plural.format(0.0, en, "en"))
        assertEquals("-1 day", Plural.format(-1.0, en, "en"))

        val abcd = mapOf("one" to Value.Str("a"), "few" to Value.Str("b"), "many" to Value.Str("c"), "other" to Value.Str("d"))
        assertEquals("b", Plural.format(2.0, abcd, "ru-RU"))

        assertEquals("бір", Plural.format(1.0, mapOf("one" to Value.Str("бір"), "other" to Value.Str("көп")), "kk"))
        assertEquals("күп", Plural.format(1.0, mapOf("one" to Value.Str("бер"), "other" to Value.Str("күп")), "tt"))

        // missing "few" form falls back to "other"; default locale is "ru" (env default)
        assertEquals("other", Plural.format(2.0, mapOf("one" to Value.Str("one"), "other" to Value.Str("other")), "ru"))
    }

    @Test
    fun `str of list and record is compact sorted JSON`() {
        assertEquals("{\"a\":1,\"b\":\"x\"}", Format.str(Value.Rec(linkedMapOf("a" to Value.Num(1.0), "b" to Value.Str("x")))))
        assertEquals("{\"a\":\"x/y\",\"b\":1}", Format.str(Value.Rec(linkedMapOf("b" to Value.Num(1.0), "a" to Value.Str("x/y")))))
        assertEquals("a\"b", Format.str(Value.Str("a\"b")))
    }
}
