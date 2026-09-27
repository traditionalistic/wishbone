package app.wishbone.core

import java.math.BigDecimal
import java.time.Instant
import java.time.ZoneId
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale
import kotlin.math.abs
import kotlin.math.pow
import kotlin.math.sign
import kotlin.math.truncate

/**
 * Number/duration/date formatting, normative per contract/stdlib.json `notes` + `functions`.
 * Rounding is always half-away-from-zero (never `kotlin.math.round`, which is half-even).
 */
object Format {

    /** Reference: t = trunc(x); |x - t| >= 0.5 ? t + sign(x) : t. round(0.49999999999999994) = 0. */
    fun roundHalfAwayFromZero(x: Double): Double {
        if (x.isNaN() || x.isInfinite()) return x
        val t = truncate(x)
        return if (abs(x - t) >= 0.5) t + sign(x) else t
    }

    /** Exact digits of round-half-away(|x|) scaled by 10^d, magnitude only (sign handled by the caller). */
    private fun magnitudeDigits(x: Double, d: Int): String {
        val ax = abs(x)
        return if (ax >= 1e15) {
            BigDecimal(roundHalfAwayFromZero(ax)).toPlainString() + "0".repeat(d)
        } else {
            BigDecimal(roundHalfAwayFromZero(ax * 10.0.pow(d))).toPlainString()
        }
    }

    private fun hasNonZeroDigit(digits: String): Boolean = digits.any { it != '0' }

    /** Left-pads to at least d+1 digits, then splits d digits from the right. */
    private fun splitFraction(digits: String, d: Int): Pair<String, String> {
        if (d == 0) return digits to ""
        val padded = digits.padStart(d + 1, '0')
        return padded.dropLast(d) to padded.takeLast(d)
    }

    private fun groupThousands(intDigits: String): String {
        val n = intDigits.length
        val sb = StringBuilder(n + n / 3)
        for (i in intDigits.indices) {
            if (i > 0 && (n - i) % 3 == 0) sb.append(' ')
            sb.append(intDigits[i])
        }
        return sb.toString()
    }

    /**
     * `str(number)` / interpolation / join: integral and |n| < 1e15 => integer digits; |n| >= 1e15
     * => exact integer digits of round-half-away(n); otherwise fixed-point with d=6, trailing
     * zeros (and a lone trailing '.') trimmed. -0 prints '0'. Decimal separator is always '.'.
     */
    fun numberToString(x: Double): String {
        if (x.isNaN()) return "NaN"
        if (x.isInfinite()) return if (x > 0) "Infinity" else "-Infinity"
        val digits = magnitudeDigits(x, 6)
        val (intPart, fracRaw) = splitFraction(digits, 6)
        val fracPart = fracRaw.trimEnd('0')
        val negative = x < 0 && hasNonZeroDigit(intPart + fracRaw)
        val body = if (fracPart.isEmpty()) intPart else "$intPart.$fracPart"
        return if (negative) "-$body" else body
    }

    /**
     * `formatNumber(x, decimals)`. decimals = clamp(floor(d), 0, 10). Integer part grouped by 3
     * from the right with U+00A0. Never rounds a decimal string representation — the rounding
     * happens on `x * 10^d` computed in double, exactly as the reference algorithm.
     */
    fun formatNumber(x: Double, decimals: Double): String {
        if (x.isNaN() || x.isInfinite()) return numberToString(x)
        val d = (if (decimals.isNaN()) 0.0 else truncate(decimals)).toInt().coerceIn(0, 10)
        val digits = magnitudeDigits(x, d)
        val (intPart, fracPart) = splitFraction(digits, d)
        val negative = x < 0 && hasNonZeroDigit(intPart + fracPart)
        val body = if (d == 0) groupThousands(intPart) else "${groupThousands(intPart)}.$fracPart"
        return if (negative) "-$body" else body
    }

    /** negative/non-finite => 0; whole seconds (floor). */
    fun formatDuration(ms: Double, style: String): String {
        val totalSeconds = if (!ms.isFinite() || ms <= 0.0) 0L else truncate(ms / 1000.0).toLong()
        val s = totalSeconds % 60
        val totalMinutes = totalSeconds / 60
        val m = totalMinutes % 60
        val totalHours = totalMinutes / 60
        return when (style) {
            "ms" -> String.format(Locale.ROOT, "%02d:%02d", totalMinutes, s)
            "dhms" -> {
                val days = totalHours / 24
                val h = totalHours % 24
                String.format(Locale.ROOT, "%d:%02d:%02d:%02d", days, h, m, s)
            }
            else -> String.format(Locale.ROOT, "%02d:%02d:%02d", totalHours, m, s)
        }
    }

    /** epoch ms in env.timeZone (unknown zone => UTC); non-finite => ''. */
    fun formatDate(ms: Double, style: String, timeZone: String): String {
        if (!ms.isFinite()) return ""
        val zone = runCatching { ZoneId.of(timeZone) }.getOrDefault(ZoneOffset.UTC)
        val instant = Instant.ofEpochMilli(roundHalfAwayFromZero(ms).toLong())
        val zdt = instant.atZone(zone)
        val pattern = when (style) {
            "date" -> "dd.MM.yyyy"
            "time" -> "HH:mm"
            else -> "dd.MM.yyyy HH:mm"
        }
        return DateTimeFormatter.ofPattern(pattern, Locale.ROOT).format(zdt)
    }

    /**
     * `str(x)` for any [Value]: identity for strings; compact JSON for list/record (record keys
     * sorted by code point); numbers via [numberToString]; null => 'null'; bool => 'true'/'false'.
     */
    fun str(v: Value): String = when (v) {
        Value.Null -> "null"
        is Value.Bool -> if (v.value) "true" else "false"
        is Value.Num -> numberToString(v.value)
        is Value.Str -> v.value
        is Value.ListV -> v.items.joinToString(",", prefix = "[", postfix = "]") { compactJson(it) }
        is Value.Rec -> v.fields.entries
            .sortedWith(compareBy(codePointComparator) { it.key })
            .joinToString(",", prefix = "{", postfix = "}") { (k, value) -> "${jsonStringLiteral(k)}:${compactJson(value)}" }
        is Value.Lambda -> "null"
    }

    /** Same as [str] but strings are JSON-quoted (used for nested values inside list/record). */
    private fun compactJson(v: Value): String = when (v) {
        is Value.Str -> jsonStringLiteral(v.value)
        else -> str(v)
    }

    private fun jsonStringLiteral(s: String): String {
        val sb = StringBuilder(s.length + 2)
        sb.append('"')
        for (ch in s) {
            when (ch) {
                '"' -> sb.append("\\\"")
                '\\' -> sb.append("\\\\")
                '\n' -> sb.append("\\n")
                '\r' -> sb.append("\\r")
                '\t' -> sb.append("\\t")
                '\b' -> sb.append("\\b")
                '\u000C' -> sb.append("\\f")
                else -> if (ch.code in 0x00..0x1F) {
                    sb.append("\\u").append(ch.code.toString(16).padStart(4, '0'))
                } else {
                    sb.append(ch)
                }
            }
        }
        sb.append('"')
        return sb.toString()
    }
}

/** Code-point ordering (never UTF-16 code unit ordering — see contract/stdlib.json `notes`). */
val codePointComparator: Comparator<String> = Comparator { a, b -> compareCodePoints(a, b) }

fun compareCodePoints(a: String, b: String): Int {
    var i = 0
    var j = 0
    while (i < a.length && j < b.length) {
        val cpA = a.codePointAt(i)
        val cpB = b.codePointAt(j)
        if (cpA != cpB) return cpA.compareTo(cpB)
        i += Character.charCount(cpA)
        j += Character.charCount(cpB)
    }
    return (a.length - i).compareTo(b.length - j)
}
