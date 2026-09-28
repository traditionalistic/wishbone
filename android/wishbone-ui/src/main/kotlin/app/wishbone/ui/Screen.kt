package app.wishbone.ui

import android.provider.Settings
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.ExperimentalSharedTransitionApi
import androidx.compose.animation.SharedTransitionLayout
import androidx.compose.animation.ContentTransform
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.WindowInsetsSides
import androidx.compose.foundation.layout.only
import androidx.compose.foundation.layout.displayCutout
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.systemBars
import androidx.compose.foundation.layout.union
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.ExperimentalComposeUiApi
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.LocalWindowInfo
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTagsAsResourceId
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import app.wishbone.core.Value
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.util.TimeZone

/**
 * Renders server screen [id] with its own navigation stack (spec §6.3), unless [onNavigate] is given.
 * [onExit] runs when the screen asks to leave the SDK — `back()`/`dismiss()` at the root; system back
 * at the root is the host's own.
 *
 * [onNavigate] switches on host-stack mode (spec §1.2): top-level `navigate` calls it with a
 * [WishboneRoute] instead of pushing the SDK's own stack, and the host is expected to push
 * `WishboneScreen(wishbone, route = …)` onto its own back stack; `back()` at the root still calls
 * [onExit] so the host can pop. A sheet opened by `present` keeps its own stack regardless.
 */
@Composable
fun WishboneScreen(
    wishbone: Wishbone,
    id: String,
    params: Map<String, Value> = emptyMap(),
    modifier: Modifier = Modifier,
    onExit: () -> Unit = {},
    onNavigate: ((WishboneRoute) -> Unit)? = null,
) {
    WishboneRoot(wishbone, id, params, null, modifier, onExit, onNavigate)
}

/** Renders [route] (as pushed by [onNavigate]) — same as the `id`/`params` overload. */
@Composable
fun WishboneScreen(
    wishbone: Wishbone,
    route: WishboneRoute,
    modifier: Modifier = Modifier,
    onExit: () -> Unit = {},
    onNavigate: ((WishboneRoute) -> Unit)? = null,
) {
    WishboneRoot(wishbone, route.screen, route.params, null, modifier, onExit, onNavigate)
}

/** Renders a compiled document in place (fixtures, preview); a new [document] keeps the values of surviving state keys. */
@Composable
fun WishboneDocument(wishbone: Wishbone, document: JsonObject, data: JsonObject = JsonObject(emptyMap()), modifier: Modifier = Modifier) {
    val inline = remember(wishbone) { InlineDocument() }
    inline.document = document
    inline.data = data
    WishboneRoot(wishbone, (document["screen"] as? JsonPrimitive)?.content ?: "inline", emptyMap(), inline, modifier)
}

/** Live preview session [code]: renders every document the admin pushes, sends runtime errors back as logs. */
@Composable
fun WishbonePreview(wishbone: Wishbone, code: String, modifier: Modifier = Modifier) {
    val inline = remember(wishbone, code) { InlineDocument() }
    DisposableEffect(wishbone, code) {
        val client = PreviewClient(wishbone, code) { m ->
            when (m) {
                is PreviewMessage.Doc -> {
                    inline.data = m.data
                    inline.document = m.document
                }
                is PreviewMessage.Play -> inline.holder?.store?.play(m.timeline)
            }
        }
        wishbone.previewLog = client::log
        client.connect()
        onDispose {
            client.close()
            wishbone.previewLog = null
        }
    }
    val doc = inline.document
    if (doc == null) {
        Box(modifier.fillMaxSize(), contentAlignment = Alignment.Center) { Text("$code · ${stringResource(R.string.wb_waiting)}") }
    } else {
        WishboneRoot(wishbone, (doc["screen"] as? JsonPrimitive)?.content ?: code, emptyMap(), inline, modifier)
    }
}

internal class InlineDocument {
    var document: JsonObject? by mutableStateOf(null)
    var data: JsonObject by mutableStateOf(JsonObject(emptyMap()))
    var holder: ScreenHolder? = null
}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalComposeUiApi::class)
@Composable
private fun WishboneRoot(
    wishbone: Wishbone,
    rootId: String,
    rootParams: Map<String, Value>,
    inline: InlineDocument?,
    modifier: Modifier,
    onExit: () -> Unit = {},
    onNavigate: ((WishboneRoute) -> Unit)? = null,
) {
    val navigator = remember(wishbone, rootId, rootParams) { Navigator(rootId, rootParams) }
    val exit by rememberUpdatedState(onExit)
    navigator.exit = { exit() }
    val nav by rememberUpdatedState(onNavigate)
    navigator.forward = onNavigate?.let { { r: WishboneRoute -> nav?.invoke(r) } }
    val holders = remember(navigator) { HashMap<Long, ScreenHolder>() }
    val saveable = rememberSaveableStateHolder() // scroll/pager positions survive a push on top, like NavigationStack
    // A host back stack (Compose Navigation, a SaveableStateHolder list) disposes a covered screen and keeps only its
    // saveable state: the root screen's DSL state goes there and back into the store built on return (iOS keeps the view).
    val saved = rememberSaveable(saver = SavedRoot.Saver) { SavedRoot(null) }
    val env = rememberEnv(wishbone)
    val dark = env["colorScheme"] == Value.Str("dark")
    DisposableEffect(navigator) { onDispose { holders.values.forEach { it.dispose() } } }

    fun holder(entry: Entry) = holders.getOrPut(entry.key) {
        ScreenHolder(wishbone, navigator, entry).also { h ->
            h.env = env
            if (entry.mode == Mode.Root) saved.attach(h)
            val doc = inline?.document
            if (entry.mode != Mode.Root || inline == null) {
                h.load()
            } else {
                inline.holder = h
                if (doc != null && !h.apply(doc, inline.data, doc.revision())) h.state = LoadState.Failed
            }
        }
    }

    @Composable
    fun Content(entry: Entry, inSheet: Boolean) {
        val h = holder(entry)
        if (entry.mode == Mode.Root && inline != null) {
            val doc = inline.document
            val data = inline.data
            LaunchedEffect(doc, data) { if (doc != null && !h.isApplied(doc, data)) h.apply(doc, data, doc.revision()) }
        }
        saveable.SaveableStateProvider(entry.key) { ScreenContent(h, env, wishbone.theme, safeArea = !inSheet) }
    }

    val sheet = navigator.sheet
    var lastSheet by remember { mutableStateOf(emptyList<Entry>()) }
    if (sheet.isNotEmpty() && sheet != lastSheet) SideEffect { lastSheet = sheet }
    val shownSheet = sheet.ifEmpty { lastSheet }
    val live = (navigator.entries + shownSheet).map { it.key }.toSet()
    LaunchedEffect(live) {
        // ponytail: popped screens are disposed after the 320 ms transition; key off the transition state if timings change
        delay(500)
        (holders.keys - live).forEach {
            holders.remove(it)?.dispose()
            saveable.removeState(it)
        }
    }

    CompositionLocalProvider(LocalWishbone provides wishbone) {
        Box(modifier.fillMaxSize().background(if (dark) Color.Black else Color.White).semantics { testTagsAsResourceId = true }) {
            AnimatedContent(navigator.base.last(), transitionSpec = { navTransition(initialState, targetState) }, contentKey = { it.key }, label = "wb-nav") { Content(it, false) }
            BackHandler(enabled = navigator.base.size > 1 && sheet.isEmpty()) { navigator.back() }
            if (shownSheet.isNotEmpty()) {
                val state = rememberModalBottomSheetState(skipPartiallyExpanded = true)
                if (sheet.isEmpty()) {
                    LaunchedEffect(Unit) {
                        state.hide()
                        lastSheet = emptyList()
                    }
                }
                // ponytail: system back / swipe / scrim close the whole sheet; DSL back() pops inside it
                ModalBottomSheet(
                    // only while a sheet is open: a back press during the hide animation must not fall through to the root exit
                    onDismissRequest = { if (navigator.sheet.isNotEmpty()) navigator.dismiss() },
                    modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars).padding(top = 10.dp), // large detent: below the status bar
                    sheetState = state,
                    containerColor = holders[shownSheet.last().key]?.store?.let { rootColor(it) }
                        ?: wishbone.theme.colors["bg1Elevated"]?.let { if (dark) it.second else it.first } ?: Color.White,
                ) {
                    AnimatedContent(shownSheet.last(), transitionSpec = { navTransition(initialState, targetState) }, contentKey = { it.key }, label = "wb-sheet") { Content(it, true) }
                }
            }
        }
    }
}

/** The root screen's DSL state as saveable state: restored into the first root holder, saved from the current one. */
private class SavedRoot(private var pending: StoreSnapshot?) {
    private var holder: ScreenHolder? = null

    fun attach(h: ScreenHolder) {
        h.restored = pending
        pending = null
        holder = h
    }

    companion object {
        // ponytail: the whole DSL state goes into the saved-instance Bundle too (binder limit ~1 MB); cap it if screens keep big lists in state
        val Saver = Saver<SavedRoot, String>(
            save = { (it.holder?.snapshot() ?: it.pending)?.encode() },
            restore = { SavedRoot(StoreSnapshot.decode(it)) },
        )
    }
}

private fun JsonObject.revision() = (this["revision"] as? JsonPrimitive)?.content?.toIntOrNull() ?: 0

/** The root node's solid background, so a sheet's handle area matches its content. */
private fun rootColor(store: Store): Color? =
    (store.doc.root.props["background"]?.let { store.root.evalOrNull(it) }?.let { store.theme.paint(it, store.dark) } as? Paint.Solid)?.color

/** iOS-like push (new from the right, old shifts left by a third), fade, and their reverses on pop. */
private fun navTransition(from: Entry, to: Entry): ContentTransform {
    val forward = to.key > from.key
    val mode = if (forward) to.mode else from.mode
    val t = when {
        mode == Mode.Fade -> fadeIn(tween(250)) togetherWith fadeOut(tween(250))
        forward -> slideInHorizontally(tween(320)) { it } togetherWith slideOutHorizontally(tween(320)) { -it / 3 }
        else -> slideInHorizontally(tween(320)) { -it / 3 } togetherWith slideOutHorizontally(tween(320)) { it }
    }
    t.targetContentZIndex = if (forward) 1f else -1f
    return t
}

/**
 * The screen lays out inside the safe area (like SwiftUI); the root node's `background` also paints
 * behind the system bars (like a SwiftUI ShapeStyle background). `env.safeTop/safeBottom` report the insets.
 */
@Composable
private fun ScreenContent(holder: ScreenHolder, env: Map<String, Value>, theme: Theme, safeArea: Boolean) {
    holder.env = env
    holder.view = LocalView.current
    when (val s = holder.state) {
        LoadState.Loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        LoadState.Failed -> Fallback { holder.load() }
        is LoadState.Ready -> {
            val store = s.store
            val rootBg = store.doc.root.props["background"]?.let { store.root.evalOrNull(it) }
            val bg = theme.brush(rootBg, store.dark)
            val ignore = (store.doc.root.props["ignoreSafeArea"]?.let { store.root.evalOrNull(it) } as? Value.Str)?.value
            StoreContent(store, env, theme, Modifier.fillMaxSize().then(if (bg != null) Modifier.background(bg) else Modifier)) {
                Box(
                    Modifier.fillMaxSize().then(if (safeArea) Modifier.windowInsetsPadding(safeInsets(ignore)) else Modifier),
                    contentAlignment = Alignment.Center,
                ) { NodeView(store.doc.root, store.root) }
            }
        }
    }
}

/** A loaded store's wiring (screen and slot): env/theme sync, the timeline frame clock, the shared-transition scope around [content]. */
@OptIn(ExperimentalSharedTransitionApi::class)
@Composable
internal fun StoreContent(store: Store, env: Map<String, Value>, theme: Theme, modifier: Modifier, content: @Composable () -> Unit) {
    SideEffect {
        store.env = env
        store.theme = theme
    }
    // Timelines: advance the frame clock only while one is running (restarted by play/stop).
    LaunchedEffect(store) {
        store.timelines.firstFrame(withFrameNanos { it })
        snapshotFlow { store.timelines.version }.collectLatest {
            do {
                val t = withFrameNanos { it }
                store.frameNanos.longValue = t
            } while (store.timelines.running(t, store.reduceMotion))
        }
    }
    SharedTransitionLayout(modifier) {
        CompositionLocalProvider(LocalSharedScope provides this) { content() }
    }
}

/** System bars + display cutout — what `env.safeTop/safeBottom` report (never the keyboard). */
private val safeArea: WindowInsets @Composable get() = WindowInsets.systemBars.union(WindowInsets.displayCutout)

/** Root `ignoreSafeArea` drops the system-bar padding on those edges; the keyboard is always avoided. */
@Composable
private fun safeInsets(ignore: String?): WindowInsets {
    val sides = when (ignore) {
        "top" -> WindowInsetsSides.Bottom + WindowInsetsSides.Horizontal
        "bottom" -> WindowInsetsSides.Top + WindowInsetsSides.Horizontal
        "all" -> null // edge-to-edge on every side (parity with iOS `.all`)
        else -> WindowInsetsSides.Top + WindowInsetsSides.Bottom + WindowInsetsSides.Horizontal
    }
    return if (sides == null) WindowInsets.ime else safeArea.only(sides).union(WindowInsets.ime)
}

@Composable
private fun Fallback(onRetry: () -> Unit) {
    Column(Modifier.fillMaxSize(), Arrangement.Center, Alignment.CenterHorizontally) {
        Text(stringResource(R.string.wb_error))
        TextButton(onClick = onRetry) { Text(stringResource(R.string.wb_retry)) }
    }
}

/**
 * What every Wishbone root (screen or slot) sets up: theme refresh, light/dark ([Wishbone.colorScheme], else the
 * system), the shared 1 s clock behind `env.now` (ticks while started), and the env values.
 */
@Composable
internal fun rememberEnv(wishbone: Wishbone): Map<String, Value> {
    val dark = when (wishbone.colorScheme) {
        "dark" -> true
        "light" -> false
        else -> isSystemInDarkTheme()
    }
    LaunchedEffect(wishbone) { wishbone.refreshTheme() }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (true) {
                val t = System.currentTimeMillis()
                Clock.now.longValue = t
                delay(1000 - t % 1000)
            }
        }
    }
    val density = LocalDensity.current
    val locale = LocalConfiguration.current.locales[0].language.lowercase()
    val window = LocalWindowInfo.current.containerSize
    val insets = safeArea
    val context = LocalContext.current
    val reduceMotion = remember { Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f }
    fun dp(px: Int) = Value.Num((px / density.density).toDouble())
    return mapOf(
        "platform" to Value.Str("android"),
        "locale" to Value.Str(locale),
        "brand" to Value.Str(wishbone.brand),
        "colorScheme" to Value.Str(if (dark) "dark" else "light"),
        "reduceMotion" to Value.of(reduceMotion),
        "screenWidth" to dp(window.width),
        "screenHeight" to dp(window.height),
        "safeTop" to dp(insets.getTop(density)),
        "safeBottom" to dp(insets.getBottom(density)),
        "timeZone" to Value.Str(TimeZone.getDefault().id),
    )
}
