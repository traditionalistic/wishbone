package app.wishbone.ui

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import app.wishbone.core.Value
import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.io.File
import kotlin.math.abs

class ThemeTest {
    private val contractFile = File("../../contract/tokens/korpu.json")
    private val theme = Theme.parse(contractFile.readText())

    @Test
    fun bundledFallbackEqualsContract() {
        val bundled = File("src/main/assets/wishbone/korpu.json").readText()
        assertEquals(Json.parseToJsonElement(contractFile.readText()), Json.parseToJsonElement(bundled))
        assertEquals(File("../../contract/fonts/OFL.txt").readText(), File("src/main/assets/wishbone/OFL.txt").readText())
    }

    @Test
    fun hexIsRgbaOrder() {
        assertEquals(Color(0xFFFF0026), parseHexColor("#FF0026FF"))
        assertEquals(Color(0x80112233), parseHexColor("#11223380"))
        assertEquals(Color(0xFF112233), parseHexColor("#112233"))
    }

    @Test
    fun colorTokenPerSchemeWithAlpha() {
        val err = Value.rec("\$color" to Value.Str("error"))
        assertEquals(Color(0xFFFF0026), theme.color(err, dark = false))
        assertEquals(Color(0xFFFF6B6B), theme.color(err, dark = true))
        val half = Value.rec("\$color" to Value.Str("error"), "alpha" to Value.Num(0.5))
        assertEquals(0.5f, theme.color(half, dark = false)!!.alpha, 0.01f)
        assertNull(theme.color(Value.rec("\$color" to Value.Str("nope")), dark = false))
        assertNull(theme.color(Value.Str("error"), dark = false))
    }

    @Test
    fun paintSolidAndGradient() {
        assertEquals(Paint.Solid(Color(0xFFFF0026)), theme.paint(Value.rec("\$color" to Value.Str("error")), false))
        val g = Value.rec(
            "\$gradient" to Value.ListV(listOf(Value.rec("\$color" to Value.Str("el1")), Value.rec("\$color" to Value.Str("el16")))),
            "angle" to Value.Num(90.0),
        )
        assertEquals(Paint.Linear(listOf(Color(0xFFFFFFFF), Color(0xFF080808)), 90.0), theme.paint(g, false))
    }

    @Test
    fun gradientUsesCssAngles() {
        assertLine(Offset(0f, 25f), Offset(100f, 25f), gradientLine(90.0, 100f, 50f)) // to right
        assertLine(Offset(50f, 50f), Offset(50f, 0f), gradientLine(0.0, 100f, 50f)) // to top
        assertLine(Offset(50f, 0f), Offset(50f, 50f), gradientLine(180.0, 100f, 50f)) // to bottom
        assertLine(Offset(0f, 100f), Offset(100f, 0f), gradientLine(45.0, 100f, 100f)) // to top right corner
    }

    @Test
    fun typeTokensAndScales() {
        assertEquals(TypeToken("Manrope", 18.0, 400, 24.0, 0.1), theme.type["sheetMessage"])
        assertEquals(TypeToken("Manrope", 16.0, 400, null, null), theme.type["body"])
        assertEquals(16.0, theme.space["lg"])
        assertEquals(9999.0, theme.radius["pill"])
        assertEquals("korpu", theme.brand)
    }

    private fun assertLine(start: Offset, end: Offset, actual: Pair<Offset, Offset>) {
        val ok = listOf(start.x to actual.first.x, start.y to actual.first.y, end.x to actual.second.x, end.y to actual.second.y)
            .all { (a, b) -> abs(a - b) < 0.01f }
        assert(ok) { "expected $start → $end, got ${actual.first} → ${actual.second}" }
    }
}
