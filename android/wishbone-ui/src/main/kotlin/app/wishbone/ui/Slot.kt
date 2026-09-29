package app.wishbone.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.EnterTransition
import androidx.compose.animation.ExitTransition
import androidx.compose.animation.core.MutableTransitionState
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.wrapContentHeight
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.unit.dp
import app.wishbone.core.Value

/**
 * Inline server-driven content inside a host screen (spec 2026-09-28 §2): the width the host offers, the height of
 * its content, and nothing at all (zero height) while loading, unpublished (the cache is dropped), failed without a
 * cache, or when the root's `when` is false. `navigate` and `present` go to [onNavigate] (`present` with transition
 * "present"); `back`/`dismiss` do nothing. Entering the composition shows the [wishbone]'s last network answer for
 * this [id]/[params] from memory (else the disk cache) and asks the server again only once that answer is 30 s old;
 * a new [id]/[params] loads that one.
 * Content that comes or goes after the first frame animates on `@motion.smooth` (fade + height from the top edge, so
 * the host's rows below slide; reduce motion: fade only); a cached slot is there in its first frame. At most
 * [SLOT_MAX_HEIGHT] dp tall: content below is clipped.
 */
@OptIn(ExperimentalComposeUiApi::class)
@Composable
fun WishboneSlot(wishbone: Wishbone, id: String, params: Map<String, Value> = emptyMap(), onNavigate: (WishboneRoute) -> Unit, modifier: Modifier = Modifier) {
    val nav by rememberUpdatedState(onNavigate)
    val navigator = remember(wishbone, id, params) {
        Navigator(id, params).apply {
            forward = { nav(it) }
            forwardsPresent = true
            exit = {}
        }
    }
    var width by remember { mutableIntStateOf(0) }
    // spec §2.3: the slot's own width, no height (it follows the content), no safe area
    val env = rememberEnv(wishbone) + mapOf(
        "screenWidth" to Value.Num((width / LocalDensity.current.density).toDouble()),
        "screenHeight" to Value.Num(0.0),
        "safeTop" to Value.Num(0.0),
        "safeBottom" to Value.Num(0.0),
    )
    val holder = remember(navigator) {
        ScreenHolder(wishbone, navigator, navigator.entries.first(), slot = true).also { h ->
            h.env = env
            // Warm content (the last network answer in memory, else the disk cache) goes on screen in the first frame
            // (as on iOS), so a warm slot doesn't animate in; the loader then revalidates it (ruling R1: after 30 s).
            h.load()
        }
    }
    DisposableEffect(holder) { onDispose { holder.dispose() } }
    holder.env = env
    holder.view = LocalView.current
    val store = holder.store
    // derived: the root's `when` may read `env.now`, whose 1 s tick then re-renders the slot only when the answer flips
    val visible by remember(store) { derivedStateOf { slotVisible(store) } }
    // Shown in the first frame: no enter animation; later comings and goings animate.
    val presence = remember(holder) { MutableTransitionState(visible) }
    presence.targetState = visible
    // A leaving slot keeps its last content: its root's `when` may be false already, or the load failed.
    var last by remember(holder) { mutableStateOf<Store?>(null) }
    SideEffect { if (visible) last = store }
    val shown = if (visible) store else last
    val spec = wishbone.theme.motions["smooth"] ?: MotionSpec.SMOOTH_FALLBACK
    val reduce = env["reduceMotion"] == Value.TRUE
    CompositionLocalProvider(LocalWishbone provides wishbone) {
        // measured even while empty, so the first content already reads its width
        Box(modifier.fillMaxWidth().onSizeChanged { width = it.width }.semantics { testTagsAsResourceId = true }, contentAlignment = Alignment.Center) {
            // Once a leaving slot has finished its exit, its last store is let go: a looping timeline stops asking for frames.
            val wired = store ?: last.takeUnless { presence.isIdle && !presence.currentState }
            if (wired != null) {
                StoreContent(wired, env, wishbone.theme, Modifier) {
                    // The size animates with the content, so the host's rows below slide; reduce motion: fade only.
                    AnimatedVisibility(
                        presence,
                        enter = fadeIn(spec.animationSpec()) + if (reduce) EnterTransition.None else expandVertically(spec.animationSpec(), expandFrom = Alignment.Top),
                        exit = fadeOut(spec.animationSpec()) + if (reduce) ExitTransition.None else shrinkVertically(spec.animationSpec(), shrinkTowards = Alignment.Top),
                    ) {
                        shown?.let {
                            // measured at its full height, shown up to the cap from the top
                            Box(Modifier.heightIn(max = SLOT_MAX_HEIGHT.dp).clipToBounds().wrapContentHeight(Alignment.Top, unbounded = true)) {
                                NodeView(it.doc.root, it.root)
                            }
                        }
                    }
                }
            }
        }
    }
}

/**
 * A slot is a row of the host's feed, often a lazy one, where a row taller than the screen can livelock SwiftUI's
 * `LazyVStack` (decisions/renderers.md): a slot is at most this tall (dp), content below is clipped (iOS: the same).
 */
internal const val SLOT_MAX_HEIGHT = 480

/**
 * The slot has content: a store whose root's `when` is absent or true — read in the root's own scope, as NodeView
 * renders it (a template root's `when` may read its instance state; the same instance the root then renders with).
 */
internal fun slotVisible(store: Store?): Boolean = store != null && store.root.scoped(store.doc.root).shows(store.doc.root)
