package app.wishbone.ui

import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import app.wishbone.core.Node
import app.wishbone.core.Value

/** Adapter for primitives rendered by optional SDK products (lottie, rive): evaluated props, box modifier, event emitter. */
typealias PrimitiveRenderer = @Composable (props: Map<String, Value>, modifier: Modifier, emit: (event: String, payload: Value) -> Unit) -> Unit

object WishboneRenderers {
    private val registry = HashMap<String, PrimitiveRenderer>()

    fun register(type: String, renderer: PrimitiveRenderer) {
        registry[type] = renderer
    }

    internal operator fun get(type: String): PrimitiveRenderer? = registry[type]
}

/** Evaluates every prop; any eval error → logged by the context, the node renders nothing. */
private fun RenderContext.evalProps(node: Node): Map<String, Value>? {
    val out = HashMap<String, Value>(node.props.size)
    for ((k, e) in node.props) out[k] = evalOrNull(e) ?: return null
    return out
}

/** [extra] wraps the whole node (shared bounds from an enclosing AnimatedVisibility). */
@Composable
internal fun NodeView(node: Node, parentCtx: RenderContext, extra: Modifier = Modifier) {
    val inst = parentCtx.store.extras[node.id]?.inst
    var ctx = if (inst != null) parentCtx.withInstance(node.id + parentCtx.keyPath, inst) else parentCtx
    val source = if (node.on["tap"] != null || node.on["longPress"] != null) remember { MutableInteractionSource() } else null
    if (source != null) ctx = ctx.copy(pressed = source.collectIsPressedAsState())
    val evaluated = ctx.evalProps(node) ?: return
    // the screen container already paints the root background (behind the system bars too)
    val p = animateProps(node, if (node === ctx.store.doc.root) evaluated - "background" else evaluated, ctx)
    val box = BoxProps.from(p).let {
        when (node.type) {
            "shape", "image", "scroll" -> it.greedy()
            "grid", "textField" -> it.greedy(vertical = false)
            else -> it
        }
    }
    // scroll padding pads the scrolled content, not the viewport (parity ruling)
    val m = extra.boxOuter(node, box, p, ctx).interactions(node, ctx, source).dragGesture(node, ctx).boxInner(box, p, ctx, padding = node.type != "scroll")
    when (node.type) {
        "stack" -> StackView(node, p, ctx, m)
        "scroll" -> ScrollView(node, p, ctx, m, box.padding.let { PaddingValues(start = it.start.toFloat().dp, top = it.top.toFloat().dp, end = it.end.toFloat().dp, bottom = it.bottom.toFloat().dp) })
        "grid" -> GridView(node, p, ctx, m)
        "spacer" -> SpacerView(p, ctx, m)
        "text" -> TextView(p, ctx, m)
        "image" -> ImageView(p, ctx, m)
        "icon" -> IconView(p, ctx, m)
        "shape" -> ShapeView(p, box, ctx, m)
        "textField" -> TextFieldView(node, p, ctx, m)
        "native" -> NativeView(p, m)
        else -> {
            val renderer = WishboneRenderers[node.type]
            if (renderer != null) {
                val wishbone = LocalWishbone.current
                val props = p.str("url")?.let { u -> p + ("url" to Value.Str(wishbone?.resolve(u) ?: u)) } ?: p
                renderer(props, m) { event, payload -> node.on[event]?.let { ctx.fire(it, payload) } }
            } else if (node.type != "lottie" && node.type != "rive") {
                ctx.store.warnOnce("t:${node.id}", "unknown primitive '${node.type}' (${node.id}) skipped")
            }
        }
    }
}
