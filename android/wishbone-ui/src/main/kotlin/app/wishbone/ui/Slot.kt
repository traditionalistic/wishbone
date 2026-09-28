package app.wishbone.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import app.wishbone.core.Value

/**
 * Inline server-driven content inside a host screen (spec 2026-09-28 §2): the width the host offers, the height of
 * its content, and nothing at all (zero height) while loading, unpublished (the cache is dropped), failed without a
 * cache, or when the root's `when` is false. `navigate` and `present` go to [onNavigate] (`present` with transition
 * "present"); `back`/`dismiss` do nothing. Loads when it enters the composition; a new [id]/[params] reloads.
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
    val holder = remember(navigator) { ScreenHolder(wishbone, navigator, navigator.entries.first(), slot = true).also { it.load() } }
    DisposableEffect(holder) { onDispose { holder.dispose() } }
    var width by remember { mutableIntStateOf(0) }
    // spec §2.3: the slot's own width, no height (it follows the content), no safe area
    val env = rememberEnv(wishbone) + mapOf(
        "screenWidth" to Value.Num((width / LocalDensity.current.density).toDouble()),
        "screenHeight" to Value.Num(0.0),
        "safeTop" to Value.Num(0.0),
        "safeBottom" to Value.Num(0.0),
    )
    holder.env = env
    holder.view = LocalView.current
    val store = holder.store
    CompositionLocalProvider(LocalWishbone provides wishbone) {
        // measured even while empty, so the first content already reads its width
        Box(modifier.fillMaxWidth().onSizeChanged { width = it.width }.semantics { testTagsAsResourceId = true }, contentAlignment = Alignment.Center) {
            if (store != null) {
                StoreContent(store, env, wishbone.theme, Modifier) {
                    val root = store.doc.root
                    if (root.whenExpr?.let { store.root.evalOrNull(it) == Value.TRUE } != false) NodeView(root, store.root)
                }
            }
        }
    }
}
