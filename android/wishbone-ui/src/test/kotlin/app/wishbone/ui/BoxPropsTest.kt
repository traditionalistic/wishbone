package app.wishbone.ui

import app.wishbone.core.Value
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class BoxPropsTest {
    private fun n(v: Double) = Value.Num(v)

    @Test
    fun defaultsAreIdentity() {
        val b = BoxProps.from(emptyMap())
        assertEquals(Dim.Wrap, b.width)
        assertEquals(Dim.Wrap, b.height)
        assertEquals(Insets(0.0, 0.0, 0.0, 0.0), b.padding)
        assertEquals(1.0, b.opacity)
        assertEquals(1.0, b.scale)
        assertEquals(0.0, b.rotation)
        assertEquals(0.0, b.corner)
        assertEquals(false, b.clip)
        assertEquals(false, b.hasTransform)
    }

    @Test
    fun dimensions() {
        val b = BoxProps.from(
            mapOf("width" to Value.Str("fill"), "height" to n(40.0), "maxWidth" to n(300.0), "aspect" to n(1.5)),
        )
        assertEquals(Dim.Fill, b.width)
        assertEquals(Dim.Fixed(40.0), b.height)
        assertEquals(300.0, b.maxWidth)
        assertEquals(1.5, b.aspect)
        assertEquals(Dim.Wrap, BoxProps.from(mapOf("width" to Value.Str("wrap"))).width)
    }

    @Test
    fun paddingNumberAndRecord() {
        assertEquals(Insets(8.0, 8.0, 8.0, 8.0), BoxProps.from(mapOf("padding" to n(8.0))).padding)
        val rec = Value.rec("h" to n(16.0), "v" to n(4.0), "start" to n(8.0), "bottom" to n(0.0))
        // specific edges win over h/v
        assertEquals(Insets(top = 4.0, bottom = 0.0, start = 8.0, end = 16.0), BoxProps.from(mapOf("padding" to rec)).padding)
    }

    @Test
    fun transformAndDecor() {
        val b = BoxProps.from(
            mapOf("opacity" to n(0.5), "offsetX" to n(10.0), "rotation" to n(45.0), "zIndex" to n(2.0), "corner" to n(9999.0), "clip" to Value.TRUE),
        )
        assertEquals(0.5, b.opacity)
        assertEquals(10.0, b.offsetX)
        assertEquals(45.0, b.rotation)
        assertEquals(2.0, b.zIndex)
        assertEquals(9999.0, b.corner)
        assertEquals(true, b.clip)
        assertEquals(true, b.hasTransform)
    }

    @Test
    fun greedyWrapBecomesFill() {
        val b = BoxProps.from(mapOf("height" to n(10.0))).greedy()
        assertEquals(Dim.Fill, b.width)
        assertEquals(Dim.Fixed(10.0), b.height)
    }

    @Test
    fun untrustedGeometryIsSanitized() {
        val b = BoxProps.from(
            mapOf(
                "width" to n(Double.NaN), "height" to n(-20.0), "minWidth" to n(Double.POSITIVE_INFINITY), "maxHeight" to n(-1.0),
                "padding" to n(-8.0), "corner" to n(-4.0), "opacity" to n(Double.NaN), "aspect" to n(-2.0),
            ),
        )
        assertEquals(Dim.Wrap, b.width)
        assertEquals(Dim.Fixed(0.0), b.height)
        assertEquals(null, b.minWidth)
        assertEquals(0.0, b.maxHeight)
        assertEquals(Insets(0.0, 0.0, 0.0, 0.0), b.padding)
        assertEquals(0.0, b.corner)
        assertEquals(1.0, b.opacity)
        assertEquals(null, b.aspect)
        val rec = BoxProps.from(mapOf("padding" to Value.rec("top" to n(-3.0), "h" to n(Double.NEGATIVE_INFINITY), "v" to n(6.0)))).padding
        assertEquals(Insets(top = 0.0, bottom = 6.0, start = 0.0, end = 0.0), rec)
    }
}
