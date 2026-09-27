package app.wishbone.core

import kotlin.math.abs
import kotlin.math.truncate

/** CLDR plural subset per contract/stdlib.json `functions.plural`. */
object Plural {
    private val oneOtherLanguages = setOf("en", "az", "ka", "uz", "ab", "kk")

    /** language = lowercase of env.locale up to the first '-' or '_'. */
    fun category(n: Double, locale: String): String {
        val lang = locale.substringBefore('-').substringBefore('_').lowercase()
        return when (lang) {
            "ru" -> ruCategory(n)
            in oneOtherLanguages -> if (abs(n) == 1.0) "one" else "other"
            else -> "other"
        }
    }

    private fun ruCategory(n: Double): String {
        if (!n.isFinite() || n != truncate(n)) return "other"
        val i = abs(n.toLong())
        val mod10 = i % 10
        val mod100 = i % 100
        return when {
            mod10 == 1L && mod100 != 11L -> "one"
            mod10 in 2L..4L && mod100 !in 12L..14L -> "few"
            else -> "many"
        }
    }

    /** Missing form => other ('other' is required, compile-time enforced upstream). Every '#' becomes str(n). */
    fun format(n: Double, forms: Map<String, Value>, locale: String): String {
        val cat = category(n, locale)
        val chosen = (forms[cat] as? Value.Str)?.value ?: (forms["other"] as? Value.Str)?.value ?: ""
        return chosen.replace("#", Format.numberToString(n))
    }
}
