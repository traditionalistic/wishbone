package app.wishbone.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.ExperimentalSharedTransitionApi
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.animation.expandHorizontally
import androidx.compose.animation.expandVertically
import androidx.compose.animation.shrinkHorizontally
import androidx.compose.animation.shrinkVertically
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.MutableTransitionState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalDensity
import app.wishbone.core.Format
import app.wishbone.core.Node
import app.wishbone.core.Value
import androidx.compose.animation.Animatable as ColorAnimatable

/** contract/primitives.json `animatable`. */
private val ANIMATABLE = listOf("width", "height", "padding", "background", "corner", "opacity", "offsetX", "offsetY", "scale", "rotation", "color", "fill", "progress")

@OptIn(ExperimentalSharedTransitionApi::class)
internal val LocalSharedScope = staticCompositionLocalOf<SharedTransitionScope?> { null }

/**
 * Implicit `animate[prop|*]` and transactions: a changed animatable prop animates with the node's
 * spec, else with the running `animate(m) { … }` transaction's, else snaps. Numbers and solid colors
 * animate; strings ("fill"), gradients and padding records snap.
 */
@Composable
internal fun animateProps(node: Node, p: Map<String, Value>, ctx: RenderContext): Map<String, Value> {
    val anim = ctx.store.extras[node.id]?.anim
    if (anim == null && !ctx.store.hasTransactions) return p
    var out: HashMap<String, Value>? = null
    for (name in ANIMATABLE) {
        val v = p[name] ?: continue
        key(name) {
            val implicit = (anim?.get(name) ?: anim?.get("*"))?.let { ctx.evalOrNull(it) }?.let { ctx.store.theme.motionOf(it) }
            val a = animateValue(v, implicit ?: ctx.store.transaction, ctx)
            if (a !== v) (out ?: HashMap(p).also { out = it })[name] = a
        }
    }
    return out ?: p
}

@Composable
private fun animateValue(v: Value, spec: MotionSpec?, ctx: RenderContext): Value {
    if (v is Value.Num) {
        val target = v.value.toFloat()
        val a = remember { Animatable(target) }
        LaunchedEffect(target) { if (spec != null) a.animateTo(target, spec.animationSpec()) else a.snapTo(target) }
        val shown = if (a.isRunning || (spec != null && a.value != target)) a.value else target
        return if (shown == target) v else Value.Num(shown.toDouble())
    }
    val color = ctx.store.theme.color(v, ctx.store.dark) ?: return v
    val a = remember { ColorAnimatable(color) }
    LaunchedEffect(color) { if (spec != null) a.animateTo(color, spec.animationSpec()) else a.snapTo(color) }
    val shown: Color = if (a.isRunning || (spec != null && a.value != color)) a.value else color
    return if (shown == color) v else Value.rec("\$argb" to Value.Num(shown.toArgb().toDouble()))
}

/**
 * Presence of a container's children: tracks exiting "ghosts" (removed repeat items with an `out`
 * transition, kept at their last position until the exit finishes). One instance per container.
 */
@Stable
internal class Presence {
    private val ghosts = LinkedHashMap<String, Pair<Int, Item>>()
    private var previous = emptyList<Item>()
    private var removed by mutableIntStateOf(0)

    /** True during the container's first composition: children present then don't animate in. */
    var initial = true
        private set

    fun merge(items: List<Item>): List<Item> {
        removed // recompose when a ghost finishes
        val keys = items.mapTo(HashSet()) { it.key }
        previous.forEachIndexed { i, it ->
            if (it.key !in keys && it.visible && it.ctx.store.extras[it.node.id]?.trOut?.isNotEmpty() == true) ghosts[it.key] = i to it.copy(visible = false)
        }
        ghosts.keys.removeAll(keys)
        val out = items.toMutableList()
        for ((i, g) in ghosts.values.sortedBy { it.first }) out.add(minOf(i, out.size), g)
        return out
    }

    fun committed(items: List<Item>) {
        previous = items
        initial = false
    }

    fun gone(key: String) {
        if (ghosts.remove(key) != null) removed++
    }
}

@Composable
internal fun rememberPresence(items: List<Item>): Pair<Presence, List<Item>> {
    val presence = remember { Presence() }
    val merged = presence.merge(items)
    SideEffect { presence.committed(items) }
    return presence to merged
}

/**
 * A child with `transition`/`matchId` renders inside AnimatedVisibility: nodes present at the
 * container's first render don't animate in; the motion is the transaction's, else `animate["*"]`,
 * else `@motion.smooth`; reduceMotion turns slide/scale/offset into fades.
 */
@OptIn(ExperimentalSharedTransitionApi::class)
@Composable
internal fun ItemView(item: Item, parent: Parent, presence: Presence) {
    val ex = item.ctx.store.extras[item.node.id]
    if (ex == null || !ex.animatedPresence) {
        if (item.visible) NodeView(item.node, item.ctx.withParent(parent))
        return
    }
    val store = item.ctx.store
    val state = remember { MutableTransitionState(item.visible && presence.initial) }
    state.targetState = item.visible
    if (!item.visible && state.isIdle && !state.currentState) SideEffect { presence.gone(item.key) }

    val spec = store.transaction
        ?: ex.anim?.get("*")?.let { item.ctx.evalOrNull(it) }?.let { store.theme.motionOf(it) }
        ?: store.theme.motions["smooth"] ?: MotionSpec.SMOOTH_FALLBACK
    val density = LocalDensity.current.density
    val parts = (ex.trIn ?: emptyList()) to (ex.trOut ?: emptyList())
    // Siblings in a stack move with the insertion/removal (SwiftUI animates the layout in the same transaction).
    val vertical = parent is Parent.Column || (parent is Parent.Scroll && !parent.horizontal)
    val horizontal = parent is Parent.Row || (parent is Parent.Scroll && parent.horizontal)
    var enter = enterOf(parts.first.reduced(store.reduceMotion), spec, density)
    var exit = exitOf(parts.second.reduced(store.reduceMotion), spec, density)
    if (vertical) {
        enter += expandVertically(spec.animationSpec(), expandFrom = Alignment.Top)
        exit += shrinkVertically(spec.animationSpec(), shrinkTowards = Alignment.Top)
    } else if (horizontal) {
        enter += expandHorizontally(spec.animationSpec(), expandFrom = Alignment.Start)
        exit += shrinkHorizontally(spec.animationSpec(), shrinkTowards = Alignment.Start)
    }

    AnimatedVisibility(state, modifier = fillWeight(item, parent), enter = enter, exit = exit) {
        val shared = LocalSharedScope.current
        val matchKey = ex.match?.let { item.ctx.evalOrNull(it) }?.let { Format.str(it) }
        val extra = if (shared != null && matchKey != null) {
            with(shared) { Modifier.sharedBounds(rememberSharedContentState(matchKey), this@AnimatedVisibility) }
        } else {
            Modifier
        }
        NodeView(item.node, item.ctx.withParent(Parent.None), extra)
    }
}

/** The wrapper is the stack's direct child, so it carries the `fill` weight along a Row/Column axis. */
private fun fillWeight(item: Item, parent: Parent): Modifier {
    fun fills(prop: String) = item.node.props[prop]?.let { item.ctx.evalOrNull(it) } == Value.Str("fill")
    return when {
        parent is Parent.Row && fills("width") -> with(parent.scope) { Modifier.weight(1f) }
        parent is Parent.Column && fills("height") -> with(parent.scope) { Modifier.weight(1f) }
        else -> Modifier
    }
}
