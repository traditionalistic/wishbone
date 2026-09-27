package app.wishbone.ui

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import app.wishbone.core.Value
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.double
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.math.abs
import kotlin.math.cos
import kotlin.math.sin

data class TypeToken(val family: String, val size: Double, val weight: Int, val lineHeight: Double?, val tracking: Double?)

/** A brand theme (spec §6.4): `colors {name: {light, dark}}`, `type`, `space`, `radius`, `motion`. */
class Theme(
    val brand: String,
    val version: String?,
    val colors: Map<String, Pair<Color, Color>>,
    val type: Map<String, TypeToken>,
    val space: Map<String, Double>,
    val radius: Map<String, Double>,
    val motions: Map<String, MotionSpec>,
) {
    /** `{"$color": name, "alpha"?: a}` → the theme color for the scheme, alpha multiplied. */
    fun color(v: Value, dark: Boolean): Color? {
        val f = (v as? Value.Rec)?.fields ?: return null
        (f["\$argb"] as? Value.Num)?.let { return Color(it.value.toLong().toInt()) } // an animated color in flight
        val name = (f["\$color"] as? Value.Str)?.value ?: return null
        val pair = colors[name] ?: return null
        val c = if (dark) pair.second else pair.first
        val a = (f["alpha"] as? Value.Num)?.value ?: return c
        return c.copy(alpha = c.alpha * a.toFloat().coerceIn(0f, 1f))
    }

    fun paint(v: Value, dark: Boolean): Paint? {
        val f = (v as? Value.Rec)?.fields ?: return null
        val stops = f["\$gradient"] as? Value.ListV ?: return color(v, dark)?.let { Paint.Solid(it) }
        val cs = stops.items.mapNotNull { color(it, dark) }
        if (cs.isEmpty()) return null
        return Paint.Linear(cs, (f["angle"] as? Value.Num)?.value ?: 180.0)
    }

    companion object {
        fun parse(json: String): Theme {
            val o = Json.parseToJsonElement(json).jsonObject
            fun nums(key: String) = o[key]?.jsonObject?.mapValues { it.value.jsonPrimitive.double } ?: emptyMap()
            return Theme(
                brand = o["brand"]?.jsonPrimitive?.contentOrNull ?: "",
                version = o["version"]?.jsonPrimitive?.contentOrNull,
                colors = o["colors"]?.jsonObject?.mapValues { (_, v) ->
                    val c = v.jsonObject
                    parseHexColor(c["light"]!!.jsonPrimitive.content) to parseHexColor(c["dark"]!!.jsonPrimitive.content)
                } ?: emptyMap(),
                type = o["type"]?.jsonObject?.mapValues { (_, v) ->
                    val t = v.jsonObject
                    TypeToken(
                        t["family"]!!.jsonPrimitive.content,
                        t["size"]!!.jsonPrimitive.double,
                        t["weight"]?.jsonPrimitive?.int ?: 400,
                        t["lineHeight"]?.jsonPrimitive?.doubleOrNull,
                        t["tracking"]?.jsonPrimitive?.doubleOrNull,
                    )
                } ?: emptyMap(),
                space = nums("space"),
                radius = nums("radius"),
                motions = o["motion"]?.jsonObject?.mapNotNull { (k, v) -> MotionSpec.fromJson(v.jsonObject)?.let { k to it } }?.toMap() ?: emptyMap(),
            )
        }
    }
}

sealed interface Paint {
    data class Solid(val color: Color) : Paint
    data class Linear(val colors: List<Color>, val angle: Double) : Paint
}

/** `#RRGGBBAA` (contract order: alpha last) or `#RRGGBB`. */
fun parseHexColor(hex: String): Color {
    val h = hex.removePrefix("#")
    val v = h.toLong(16)
    return if (h.length == 8) {
        Color(red = ((v shr 24) and 0xFF).toInt(), green = ((v shr 16) and 0xFF).toInt(), blue = ((v shr 8) and 0xFF).toInt(), alpha = (v and 0xFF).toInt())
    } else {
        Color(0xFF000000 or v)
    }
}

/** CSS `linear-gradient(<angle>deg)` line for a w×h box: 0 = to top, 90 = to right. */
fun gradientLine(angleDeg: Double, w: Float, h: Float): Pair<Offset, Offset> {
    val a = Math.toRadians(angleDeg)
    val dx = sin(a).toFloat()
    val dy = -cos(a).toFloat()
    val half = (abs(w * dx) + abs(h * dy)) / 2
    val c = Offset(w / 2, h / 2)
    return Offset(c.x - dx * half, c.y - dy * half) to Offset(c.x + dx * half, c.y + dy * half)
}
