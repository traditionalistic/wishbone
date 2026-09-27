package app.wishbone.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.geometry.center
import androidx.compose.ui.graphics.Outline
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.LinearGradientShader
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.graphics.Shader
import androidx.compose.ui.graphics.ShaderBrush
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.wishbone.core.Value

/** Bundled DS fonts by theme family name; Compose picks the nearest registered weight. */
internal object Fonts {
    private val families = mapOf(
        "Manrope" to FontFamily(
            Font(R.font.wb_manrope_regular, FontWeight.W400),
            Font(R.font.wb_manrope_medium, FontWeight.W500),
            Font(R.font.wb_manrope_semibold, FontWeight.W600),
            Font(R.font.wb_manrope_bold, FontWeight.W700),
        ),
        "Montserrat" to FontFamily(
            Font(R.font.wb_montserrat_semibold, FontWeight.W600),
            Font(R.font.wb_montserrat_extrabold, FontWeight.W800),
        ),
        "GiorgioSansLCG" to FontFamily(Font(R.font.wb_giorgiosanslcg_heavyitalic, FontWeight.W800, FontStyle.Italic)),
    )

    fun family(name: String): FontFamily = families[name] ?: FontFamily.Default
}

internal val DEFAULT_INK: Value = Value.rec("\$color" to Value.Str("el16"))

internal fun Theme.brush(v: Value?, dark: Boolean): Brush? = when (val p = v?.let { paint(it, dark) }) {
    is Paint.Solid -> SolidColor(p.color)
    is Paint.Linear -> CssGradient(if (p.colors.size == 1) p.colors + p.colors else p.colors, p.angle)
    null -> null
}

private class CssGradient(val colors: List<Color>, val angle: Double) : ShaderBrush() {
    override fun createShader(size: Size): Shader {
        val (start, end) = gradientLine(angle, size.width, size.height)
        return LinearGradientShader(start, end, colors)
    }

    override fun equals(other: Any?) = other is CssGradient && other.colors == colors && other.angle == angle
    override fun hashCode() = colors.hashCode() * 31 + angle.hashCode()
}

/** `{"$type": name}` → font, size (sp, scales with font scale), weight, lineHeight, tracking. */
internal fun Theme.textStyle(v: Value?, color: Color, align: String?): TextStyle {
    val name = ((v as? Value.Rec)?.fields?.get("\$type") as? Value.Str)?.value
    val t = type[name] ?: type["body"] ?: TypeToken("Manrope", 16.0, 400, null, null)
    return TextStyle(
        color = color,
        fontFamily = Fonts.family(t.family),
        fontSize = t.size.sp,
        fontWeight = FontWeight(t.weight),
        lineHeight = t.lineHeight?.sp ?: TextUnit.Unspecified,
        letterSpacing = t.tracking?.sp ?: TextUnit.Unspecified,
        textAlign = when (align) {
            "center" -> TextAlign.Center
            "end" -> TextAlign.End
            else -> TextAlign.Start
        },
    )
}

/** `shape kind: circle`: a centered circle of diameter min(w, h), not a stretched capsule (parity ruling). */
internal object CenteredCircle : Shape {
    override fun createOutline(size: Size, layoutDirection: LayoutDirection, density: Density): Outline {
        val r = size.minDimension / 2
        return Outline.Generic(Path().apply { addOval(Rect(size.center, r)) })
    }
}

/** `corner` radius; Compose scales corners down to fit, so ≥ half the smaller side is a capsule. */
internal fun cornerShape(corner: Double): Shape = if (corner > 0) RoundedCornerShape(corner.dp) else RectangleShape

/** Finite numbers only — documents are untrusted and Compose throws on NaN/∞ geometry. */
internal fun Map<String, Value>.num(k: String) = (this[k] as? Value.Num)?.value?.takeIf { it.isFinite() }
internal fun Map<String, Value>.str(k: String) = (this[k] as? Value.Str)?.value
internal fun Map<String, Value>.bool(k: String) = (this[k] as? Value.Bool)?.value
internal fun Map<String, Value>.rec(k: String) = (this[k] as? Value.Rec)?.fields
