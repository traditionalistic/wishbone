package app.wishbone.ui

import androidx.compose.animation.EnterTransition
import androidx.compose.animation.ExitTransition
import androidx.compose.animation.core.CubicBezierEasing
import androidx.compose.animation.core.FiniteAnimationSpec
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.animation.slideIn
import androidx.compose.animation.slideOut
import androidx.compose.ui.unit.IntOffset
import app.wishbone.core.Value
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.math.PI
import kotlin.math.abs
import kotlin.math.roundToInt

/** A motion token value (spec §4, theme `motion`): spring `{response ms, damping}` or curve `{duration ms, bezier}`. */
sealed interface MotionSpec {
    data class Spring(val responseMs: Double, val damping: Double) : MotionSpec {
        /** Compose stiffness for a SwiftUI-style response: (2π / response_s)². */
        val stiffness: Float get() = (2 * PI / (responseMs / 1000)).let { it * it }.toFloat()
    }

    data class Curve(val durationMs: Double, val bezier: List<Double>) : MotionSpec

    fun <T> animationSpec(): FiniteAnimationSpec<T> = when (this) {
        is Spring -> spring(dampingRatio = damping.toFloat(), stiffness = stiffness)
        is Curve -> tween(durationMs.roundToInt(), easing = CubicBezierEasing(bezier[0].toFloat(), bezier[1].toFloat(), bezier[2].toFloat(), bezier[3].toFloat()))
    }

    companion object {
        val SMOOTH_FALLBACK = Curve(300.0, listOf(0.38, 0.7, 0.125, 1.0))
        val INTERACTIVE_FALLBACK = Spring(280.0, 0.86)

        /** Theme JSON `{kind: "spring", response, damping}` | `{kind: "curve", duration, bezier}`. */
        fun fromJson(o: JsonObject): MotionSpec? {
            fun n(k: String) = o[k]?.jsonPrimitive?.doubleOrNull
            return when (o["kind"]?.jsonPrimitive?.contentOrNull) {
                "spring" -> Spring(n("response") ?: return null, n("damping") ?: return null)
                "curve" -> Curve(n("duration") ?: return null, (o["bezier"] as? JsonArray)?.map { it.jsonPrimitive.doubleOrNull ?: 0.0 }?.takeIf { it.size == 4 } ?: return null)
                else -> null
            }
        }

        /** Inline stdlib motion record (`spring(…)`, `ease(…)`, `linear(…)`, `curve(…)`). */
        fun fromValue(f: Map<String, Value>): MotionSpec? {
            fun n(k: String) = (f[k] as? Value.Num)?.value
            return when ((f["kind"] as? Value.Str)?.value) {
                "spring" -> Spring(n("response") ?: return null, n("damping") ?: return null)
                "curve" -> Curve(n("duration") ?: return null, (f["bezier"] as? Value.ListV)?.items?.map { (it as? Value.Num)?.value ?: 0.0 }?.takeIf { it.size == 4 } ?: return null)
                else -> null
            }
        }
    }
}

/** `{"$motion": name}` → theme token, `{"$motion": {kind…}}` → inline spec. */
fun Theme.motionOf(v: Value?): MotionSpec? = when (val m = (v as? Value.Rec)?.fields?.get("\$motion")) {
    is Value.Str -> motions[m.value]
    is Value.Rec -> MotionSpec.fromValue(m.fields)
    else -> null
}

/** One `tr.in`/`tr.out` element: fade | slide(edge) | scale(from) | offset(x, y). */
internal data class TransitionPart(val kind: String, val edge: String?, val from: Double?, val x: Double, val y: Double) {
    companion object {
        fun list(arr: JsonArray?): List<TransitionPart> = arr?.mapNotNull { e ->
            val o = e.jsonObject
            fun n(k: String) = o[k]?.jsonPrimitive?.doubleOrNull
            TransitionPart(o["k"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null, o["edge"]?.jsonPrimitive?.contentOrNull, n("from"), n("x") ?: 0.0, n("y") ?: 0.0)
        } ?: emptyList()
    }
}

/** reduceMotion: slide/scale/offset become fades. */
internal fun List<TransitionPart>.reduced(reduceMotion: Boolean): List<TransitionPart> =
    if (!reduceMotion) this else if (isEmpty()) this else listOf(TransitionPart("fade", null, null, 0.0, 0.0))

private fun TransitionPart.offset(w: Int, h: Int, density: Float) = when (kind) {
    "slide" -> when (edge) {
        "start" -> IntOffset(-w, 0)
        "end" -> IntOffset(w, 0)
        "top" -> IntOffset(0, -h)
        else -> IntOffset(0, h)
    }
    else -> IntOffset((x * density).roundToInt(), (y * density).roundToInt())
}

internal fun enterOf(parts: List<TransitionPart>, spec: MotionSpec, density: Float): EnterTransition =
    parts.fold(EnterTransition.None) { acc, p ->
        acc + when (p.kind) {
            "fade" -> fadeIn(spec.animationSpec())
            "scale" -> scaleIn(spec.animationSpec(), initialScale = (p.from ?: 0.0).toFloat())
            "slide", "offset" -> slideIn(spec.animationSpec()) { p.offset(it.width, it.height, density) }
            else -> EnterTransition.None
        }
    }

internal fun exitOf(parts: List<TransitionPart>, spec: MotionSpec, density: Float): ExitTransition =
    parts.fold(ExitTransition.None) { acc, p ->
        acc + when (p.kind) {
            "fade" -> fadeOut(spec.animationSpec())
            "scale" -> scaleOut(spec.animationSpec(), targetScale = (p.from ?: 0.0).toFloat())
            "slide", "offset" -> slideOut(spec.animationSpec()) { p.offset(it.width, it.height, density) }
            else -> ExitTransition.None
        }
    }

/** Nearest `snap` point to the release position (0 when there are none). */
internal fun nearestSnap(v: Double, snaps: List<Double>): Double = snaps.minByOrNull { abs(it - v) } ?: 0.0
