package app.wishbone.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.requiredHeight
import androidx.compose.foundation.layout.requiredWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.dropShadow
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.shadow.Shadow
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.DpOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.zIndex
import app.wishbone.core.Expr
import app.wishbone.core.Node
import app.wishbone.core.Value

private val Double.dpv: Dp get() = toFloat().dp

private val TRANSFORM = listOf("opacity", "offsetX", "offsetY", "scale", "rotation")

/**
 * Per node id: the transform props the layer block evaluates (draw phase), so a change — every scroll frame for
 * `scroll.*`-driven ones — redraws the layer without recomposing the node. Only write-free ones without an implicit
 * `anim`: what a handler writes (state, data, …) may animate in an `animate(…)` transaction, and a prop reading it
 * may fail on a missing value, which drops the node (as on iOS) — both need the composed value.
 */
internal fun layerProps(root: Node, extras: Map<String, NodeExtras>): Map<String, Map<String, Expr>> {
    val out = HashMap<String, Map<String, Expr>>()
    fun walk(n: Node) {
        val anim = extras[n.id]?.anim
        if (anim?.get("*") == null) {
            val layer = TRANSFORM.mapNotNull { k -> n.props[k]?.takeIf { anim?.get(k) == null && it.writeFree() }?.let { k to it } }
            if (layer.isNotEmpty()) out[n.id] = layer.toMap()
        }
        n.children.forEach(::walk)
    }
    walk(root)
    return out
}

/**
 * Nothing a handler writes, and no possibly-missing value, feeds it: literals, env, tokens, drivers and pure operations
 * on them (fns see only their params). Params stay out: optional ones read null, and they never change anyway.
 */
private fun Expr.writeFree(): Boolean = when (this) {
    is Expr.Lit, is Expr.EnvRef, is Expr.TokenRef, is Expr.DriverRef -> true
    is Expr.Get -> obj.writeFree()
    is Expr.At -> obj.writeFree() && key.writeFree()
    is Expr.BinOp -> a.writeFree() && b.writeFree()
    is Expr.UnOp -> a.writeFree()
    is Expr.If -> cond.writeFree() && then.writeFree() && otherwise.writeFree()
    is Expr.Call -> args.all { it.writeFree() }
    is Expr.UserCall -> args.all { it.writeFree() }
    is Expr.ListLit -> items.all { it.writeFree() }
    is Expr.RecLit -> fields.values.all { it.writeFree() }
    is Expr.StrInterp -> parts.all { it.writeFree() }
    else -> false // state, instance state, computed, data, params, locals, self.pressed, lambdas
}

/**
 * Canonical box model, outer half: zIndex → transform layer → shadow → size/fill (renderer-decisions.md).
 * The transform layer composes props with active timelines (×/+) and the drag translation; it reads
 * those per frame inside the layer block, so only drawing is redone — as it does the node's [layerProps].
 */
internal fun Modifier.boxOuter(node: Node, b: BoxProps, p: Map<String, Value>, ctx: RenderContext): Modifier {
    var m = this
    if (b.zIndex != 0.0) m = m.zIndex(b.zIndex.toFloat())
    val store = ctx.store
    val timeline = store.timelines.isTarget(node.id)
    val drag = store.extras[node.id]?.drag
    val dx = drag?.let { store.driverCell("drag", node.id + ctx.keyPath, "x") }
    val dy = drag?.let { store.driverCell("drag", node.id + ctx.keyPath, "y") }
    val layer = store.layerProps[node.id]
    if (b.hasTransform || timeline || drag != null || layer != null) {
        m = m.graphicsLayer {
            // like BoxProps: a non-number or non-finite value is the prop's default (an eval error is logged)
            fun prop(name: String, fallback: Double) = (layer?.get(name)?.let { ctx.evalOrNull(it) } as? Value.Num)?.value?.takeIf { it.isFinite() } ?: fallback
            val t = if (timeline) store.timelines.transform(node.id, ctx.index, store.frameNanos.longValue, store.reduceMotion) else LayerTransform.IDENTITY
            alpha = (prop("opacity", b.opacity) * t.opacity).toFloat().coerceIn(0f, 1f)
            scaleX = (prop("scale", b.scale) * t.scale).toFloat()
            scaleY = scaleX
            rotationZ = (prop("rotation", b.rotation) + t.rotation).toFloat()
            translationX = (prop("offsetX", b.offsetX) + t.offsetX + (dx?.doubleValue ?: 0.0)).dpv.toPx()
            translationY = (prop("offsetY", b.offsetY) + t.offsetY + (dy?.doubleValue ?: 0.0)).dpv.toPx()
        }
    }
    val shadow = p.rec("shadow")
    val shadowColor = shadow?.get("color")?.let { ctx.store.theme.color(it, ctx.store.dark) }
    if (shadow != null && shadowColor != null) {
        fun n(k: String) = (shadow[k] as? Value.Num)?.value?.takeIf { it.isFinite() } ?: 0.0
        m = m.dropShadow(cornerShape(b.corner), Shadow(n("radius").coerceAtLeast(0.0).dpv, shadowColor, offset = DpOffset(n("x").dpv, n("y").dpv)))
    }
    return m.dims(b, ctx.parent)
}

/** Numbers are exact (`required*`: a child may overflow a clipped parent); `fill` along a Row/Column main axis = `weight(1f)`. */
internal fun Modifier.dims(b: BoxProps, parent: Parent): Modifier {
    var m = this
    m = when (val w = b.width) {
        is Dim.Fixed -> m.requiredWidth(w.dp.dpv)
        Dim.Fill -> if (parent is Parent.Row) with(parent.scope) { m.weight(1f) } else m.fillMaxWidth()
        Dim.Wrap -> m
    }
    m = when (val h = b.height) {
        is Dim.Fixed -> m.requiredHeight(h.dp.dpv)
        Dim.Fill -> if (parent is Parent.Column) with(parent.scope) { m.weight(1f) } else m.fillMaxHeight()
        Dim.Wrap -> m
    }
    if (b.minWidth != null || b.maxWidth != null) m = m.widthIn(b.minWidth?.dpv ?: Dp.Unspecified, b.maxWidth?.dpv ?: Dp.Unspecified)
    if (b.minHeight != null || b.maxHeight != null) m = m.heightIn(b.minHeight?.dpv ?: Dp.Unspecified, b.maxHeight?.dpv ?: Dp.Unspecified)
    b.aspect?.takeIf { it > 0 }?.let { m = m.aspectRatio(it.toFloat()) }
    return m
}

/** Inner half: clip → background / material → border → padding. */
internal fun Modifier.boxInner(b: BoxProps, p: Map<String, Value>, ctx: RenderContext, padding: Boolean = true): Modifier {
    val theme = ctx.store.theme
    val dark = ctx.store.dark
    val shape = cornerShape(b.corner)
    var m = this
    if (b.clip) m = m.clip(shape)
    // ponytail: `material` is a translucent bg1 fill, not a backdrop blur (no native backdrop blur in Compose)
    p.str("material")?.let { mat ->
        val base = theme.colors["bg1"]?.let { if (dark) it.second else it.first }
        val a = when (mat) { "thin" -> 0.6f; "thick" -> 0.92f; else -> 0.78f }
        if (base != null) m = m.background(base.copy(alpha = a), shape)
    }
    theme.brush(p["background"], dark)?.let { m = m.background(it, shape) }
    val border = p.rec("border")
    val borderColor = border?.get("color")?.let { theme.color(it, dark) }
    val borderWidth = (border?.get("width") as? Value.Num)?.value
    if (borderColor != null && borderWidth != null && borderWidth > 0) m = m.border(borderWidth.dpv, borderColor, shape)
    val pad = b.padding
    if (padding && pad != Insets.ZERO) m = m.padding(start = pad.start.dpv, top = pad.top.dpv, end = pad.end.dpv, bottom = pad.bottom.dpv)
    return m
}

/** Identifier `wb.<screen>.<id>[.<repeatKey>…]`, label, role, hidden; tap/longPress without indication; appear/disappear. */
@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun Modifier.interactions(node: Node, ctx: RenderContext, source: MutableInteractionSource?): Modifier {
    val ex = ctx.store.extras[node.id]
    val id = remember(node, ctx.keyPath) { ctx.identifier(node) }
    var m = if (ex?.a11yHidden?.let { ctx.evalOrNull(it) } == Value.TRUE) {
        clearAndSetSemantics { testTag = id }
    } else {
        val label = ex?.a11yLabel?.let { ctx.evalOrNull(it) as? Value.Str }?.value
        semantics {
            testTag = id
            label?.let { contentDescription = it }
            when (ex?.a11yRole) {
                "button" -> role = Role.Button
                "header" -> heading()
                "image" -> role = Role.Image
            }
        }
    }
    val tap = node.on["tap"]
    val longPress = node.on["longPress"]
    if (tap != null || longPress != null) {
        m = m.combinedClickable(
            interactionSource = source ?: remember { MutableInteractionSource() },
            indication = null,
            onLongClick = longPress?.let { { ctx.fire(it) } },
            onClick = { tap?.let { ctx.fire(it) } },
        )
    }
    val appear = node.on["appear"]
    val disappear = node.on["disappear"]
    if (appear != null || disappear != null) {
        val current by rememberUpdatedState(ctx)
        DisposableEffect(Unit) {
            appear?.let { current.fire(it) }
            onDispose { disappear?.let { current.fire(it) } }
        }
    }
    return m
}
