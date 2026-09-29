package app.wishbone.ui

import app.wishbone.core.Value
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

/** `shape.trim` → drawArc angles (0° = 3 o'clock, clockwise). Same vectors as iOS `ShapeTrimTests`. */
class ShapeTrimTest {
    private fun arc(vararg kv: Pair<String, Double>) = trimArc(kv.associate { (k, v) -> k to Value.Num(v) })

    @Test
    fun fractionsFromTwelveOClockBecomeArcAngles() {
        assertEquals(-90f to 90f, arc("from" to 0.0, "to" to 0.25)) // 12 → 3 o'clock
        assertEquals(-90f to 360f, arc("from" to -1.0, "to" to 2.0)) // clamped to 0…1
        assertEquals(90f to 0f, arc("from" to 0.5, "to" to 0.2)) // to <= from: nothing to draw
    }

    @Test
    fun absentOrMalformedTrimMeansTheWholeStroke() {
        assertNull(trimArc(null))
        assertNull(arc("from" to 0.0))
        assertNull(arc("from" to Double.NaN, "to" to 1.0))
    }
}
