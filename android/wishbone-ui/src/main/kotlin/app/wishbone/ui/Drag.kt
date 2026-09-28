package app.wishbone.ui

import androidx.compose.animation.core.animate
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.MutableDoubleState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.pointer.PointerInputChange
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.util.VelocityTracker
import androidx.compose.ui.unit.dp
import app.wishbone.core.Node
import app.wishbone.core.Value
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

/**
 * `drag` (spec §4.3): the translation follows the finger along `axis` (clamped to `bounds`) and lives in the
 * instance's `drag.<id>.x|y` cells, which the transform layer adds to offsetX/Y. On release `dragEnd` fires
 * at once with the release translation and velocity (dp/s), then the node springs (`@motion.interactive`)
 * to the nearest `snap` point (or 0). A cancelled gesture only springs back, without `dragEnd`.
 */
@Composable
internal fun Modifier.dragGesture(node: Node, ctx: RenderContext): Modifier {
    val spec = ctx.store.extras[node.id]?.drag ?: return this
    val key = node.id + ctx.keyPath
    val x = ctx.store.driverCell("drag", key, "x")
    val y = ctx.store.driverCell("drag", key, "y")
    val scope = rememberCoroutineScope()
    val current by rememberUpdatedState(ctx)
    DisposableEffect(key) {
        onDispose {
            x.doubleValue = 0.0
            y.doubleValue = 0.0
        }
    }
    return pointerInput(key, spec) {
        val tracker = VelocityTracker()
        var total = Offset.Zero
        var settle: Job? = null
        val onStart = { _: Offset ->
            settle?.cancel()
            tracker.resetTracking()
            total = Offset(x.doubleValue.toFloat().dp.toPx(), y.doubleValue.toFloat().dp.toPx())
        }
        fun move(change: PointerInputChange, dx: Float, dy: Float) {
            change.consume()
            total += Offset(dx, dy)
            tracker.addPosition(change.uptimeMillis, total)
            if (spec.axis != "y") x.doubleValue = spec.clamp((total.x / density).toDouble())
            if (spec.axis != "x") y.doubleValue = spec.clamp((total.y / density).toDouble())
        }
        // a cancelled gesture (no real release) just settles, without dragEnd — as on iOS
        fun release(cancelled: Boolean) {
            val v = tracker.calculateVelocity()
            val rx = x.doubleValue
            val ry = y.doubleValue
            val vx = (v.x / density).toDouble()
            val vy = (v.y / density).toDouble()
            if (!cancelled) node.on["dragEnd"]?.let { current.fire(it, Value.rec("x" to Value.Num(rx), "y" to Value.Num(ry), "vx" to Value.Num(vx), "vy" to Value.Num(vy))) }
            settle = scope.launch {
                val motion = current.store.theme.motions["interactive"] ?: MotionSpec.INTERACTIVE_FALLBACK
                suspend fun settleAxis(cell: MutableDoubleState, from: Double, velocity: Double) =
                    animate(from.toFloat(), nearestSnap(from, spec.snap).toFloat(), velocity.toFloat(), motion.animationSpec()) { value, _ -> cell.doubleValue = value.toDouble() }
                if (spec.axis != "y") launch { settleAxis(x, rx, vx) }
                if (spec.axis != "x") launch { settleAxis(y, ry, vy) }
            }
        }
        val onEnd = { release(cancelled = false) }
        val onCancel = { release(cancelled = true) }
        when (spec.axis) {
            "x" -> detectHorizontalDragGestures(onStart, onEnd, onCancel) { change, dx -> move(change, dx, 0f) }
            "y" -> detectVerticalDragGestures(onStart, onEnd, onCancel) { change, dy -> move(change, 0f, dy) }
            else -> detectDragGestures(onStart, onEnd, onCancel) { change, d -> move(change, d.x, d.y) }
        }
    }
}
