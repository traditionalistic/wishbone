package app.wishbone.ui

import app.wishbone.core.Value

internal sealed interface Dim {
    data class Fixed(val dp: Double) : Dim
    data object Fill : Dim
    data object Wrap : Dim
}

internal data class Insets(val top: Double, val bottom: Double, val start: Double, val end: Double)

/** The numeric part of the common props (spec §2.1), defaults from contract/primitives.json `common`. */
internal data class BoxProps(
    val width: Dim,
    val height: Dim,
    val minWidth: Double?,
    val maxWidth: Double?,
    val minHeight: Double?,
    val maxHeight: Double?,
    val aspect: Double?,
    val padding: Insets,
    val corner: Double,
    val clip: Boolean,
    val opacity: Double,
    val offsetX: Double,
    val offsetY: Double,
    val scale: Double,
    val rotation: Double,
    val zIndex: Double,
) {
    val hasTransform: Boolean get() = opacity != 1.0 || offsetX != 0.0 || offsetY != 0.0 || scale != 1.0 || rotation != 0.0

    /** SwiftUI-greedy primitives (shape, image, scroll, grid width, textField width): `wrap` takes the offered space. */
    fun greedy(horizontal: Boolean = true, vertical: Boolean = true) = copy(
        width = if (horizontal && width == Dim.Wrap) Dim.Fill else width,
        height = if (vertical && height == Dim.Wrap) Dim.Fill else height,
    )

    companion object {
        fun from(p: Map<String, Value>): BoxProps {
            fun num(k: String) = (p[k] as? Value.Num)?.value
            return BoxProps(
                width = dim(p["width"]),
                height = dim(p["height"]),
                minWidth = num("minWidth"),
                maxWidth = num("maxWidth"),
                minHeight = num("minHeight"),
                maxHeight = num("maxHeight"),
                aspect = num("aspect"),
                padding = insets(p["padding"]),
                corner = num("corner") ?: 0.0,
                clip = (p["clip"] as? Value.Bool)?.value ?: false,
                opacity = num("opacity") ?: 1.0,
                offsetX = num("offsetX") ?: 0.0,
                offsetY = num("offsetY") ?: 0.0,
                scale = num("scale") ?: 1.0,
                rotation = num("rotation") ?: 0.0,
                zIndex = num("zIndex") ?: 0.0,
            )
        }

        private fun dim(v: Value?): Dim = when {
            v is Value.Num -> Dim.Fixed(v.value)
            v is Value.Str && v.value == "fill" -> Dim.Fill
            else -> Dim.Wrap
        }

        private fun insets(v: Value?): Insets {
            if (v is Value.Num) return Insets(v.value, v.value, v.value, v.value)
            val f = (v as? Value.Rec)?.fields ?: return Insets(0.0, 0.0, 0.0, 0.0)
            fun n(k: String) = (f[k] as? Value.Num)?.value
            val h = n("h") ?: 0.0
            val vv = n("v") ?: 0.0
            return Insets(n("top") ?: vv, n("bottom") ?: vv, n("start") ?: h, n("end") ?: h)
        }
    }
}
