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

    /** `wrap` takes the offered space along the given axes. */
    fun greedy(horizontal: Boolean = true, vertical: Boolean = true) = copy(
        width = if (horizontal && width == Dim.Wrap) Dim.Fill else width,
        height = if (vertical && height == Dim.Wrap) Dim.Fill else height,
    )

    /**
     * SwiftUI flexibility of `wrap` (parity ruling): shape and image take the offered space; a scroll takes it along its
     * scroll axis, and across it an axis-h scroll is as tall as its content (ScrollView) while an axis-v one takes the
     * offered width (iOS frames its content to the full width); grid and textField take the offered width.
     */
    fun flexible(type: String, axis: String?) = when (type) {
        "shape", "image" -> greedy()
        "scroll" -> if (axis == "h") greedy(vertical = false) else greedy()
        "grid", "textField" -> greedy(vertical = false)
        else -> this
    }

    /** Server documents are untrusted: non-finite numbers are dropped, sizes/padding/corner clamp at 0 (Compose throws on negatives). */
    companion object {
        private fun finite(v: Value?) = (v as? Value.Num)?.value?.takeIf { it.isFinite() }

        fun from(p: Map<String, Value>): BoxProps {
            fun num(k: String) = finite(p[k])
            fun size(k: String) = num(k)?.coerceAtLeast(0.0)
            return BoxProps(
                width = dim(p["width"]),
                height = dim(p["height"]),
                minWidth = size("minWidth"),
                maxWidth = size("maxWidth"),
                minHeight = size("minHeight"),
                maxHeight = size("maxHeight"),
                aspect = num("aspect")?.takeIf { it > 0 },
                padding = insets(p["padding"]),
                corner = size("corner") ?: 0.0,
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
            v is Value.Num -> finite(v)?.let { Dim.Fixed(it.coerceAtLeast(0.0)) } ?: Dim.Wrap
            v is Value.Str && v.value == "fill" -> Dim.Fill
            else -> Dim.Wrap
        }

        private fun insets(v: Value?): Insets {
            if (v is Value.Num) return (finite(v) ?: 0.0).coerceAtLeast(0.0).let { Insets(it, it, it, it) }
            val f = (v as? Value.Rec)?.fields ?: return Insets(0.0, 0.0, 0.0, 0.0)
            fun n(k: String) = finite(f[k])?.coerceAtLeast(0.0)
            val h = n("h") ?: 0.0
            val vv = n("v") ?: 0.0
            return Insets(n("top") ?: vv, n("bottom") ?: vv, n("start") ?: h, n("end") ?: h)
        }
    }
}
