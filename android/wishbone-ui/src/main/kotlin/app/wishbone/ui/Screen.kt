package app.wishbone.ui

import android.provider.Settings
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedContent
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
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
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
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.runtime.setValue
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
import app.wishbone.core.Value
import kotlinx.coroutines.delay
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.util.TimeZone

/** Renders server screen [id] with its own navigation stack (spec §6.3). */
@Composable
fun WishboneScreen(wishbone: Wishbone, id: String, params: Map<String, Value> = emptyMap(), modifier: Modifier = Modifier) {
    WishboneRoot(wishbone, id, params, null, modifier)
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
                is PreviewMessage.Play -> inline.holder?.store?.let { s -> s.timelines[m.timeline] = System.currentTimeMillis() }
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
private fun WishboneRoot(wishbone: Wishbone, rootId: String, rootParams: Map<String, Value>, inline: InlineDocument?, modifier: Modifier) {
    val navigator = remember(wishbone, rootId, rootParams) { Navigator(rootId, rootParams) }
    val holders = remember(navigator) { HashMap<Long, ScreenHolder>() }
    val saveable = rememberSaveableStateHolder() // scroll/pager positions survive a push on top, like NavigationStack
    val dark = when (wishbone.colorScheme) {
        "dark" -> true
        "light" -> false
        else -> isSystemInDarkTheme()
    }
    val env = rememberEnv(wishbone.brand, dark)
    LaunchedEffect(wishbone) { wishbone.refreshTheme() }
    LaunchedEffect(Unit) {
        while (true) {
            val t = System.currentTimeMillis()
            Clock.now.longValue = t
            delay(1000 - t % 1000)
        }
    }
    DisposableEffect(navigator) { onDispose { holders.values.forEach { it.dispose() } } }

    fun holder(entry: Entry) = holders.getOrPut(entry.key) {
        ScreenHolder(wishbone, navigator, entry).also { h ->
            h.env = env
            val doc = inline?.document
            if (entry.mode != Mode.Root || inline == null) {
                h.load()
            } else {
                inline.holder = h
                if (doc != null) h.apply(doc, inline.data, doc.revision())
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
                    onDismissRequest = { navigator.dismiss() },
                    modifier = Modifier.windowInsetsPadding(WindowInsets.statusBars).padding(top = 10.dp), // large detent: below the status bar
                    sheetState = state,
                    containerColor = wishbone.theme.colors["bg1Elevated"]?.let { if (dark) it.second else it.first } ?: Color.White,
                ) {
                    AnimatedContent(shownSheet.last(), transitionSpec = { navTransition(initialState, targetState) }, contentKey = { it.key }, label = "wb-sheet") { Content(it, true) }
                }
            }
        }
    }
}

private fun JsonObject.revision() = (this["revision"] as? JsonPrimitive)?.content?.toIntOrNull() ?: 0

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
            SideEffect {
                store.env = env
                store.theme = theme
            }
            val rootBg = store.doc.root.props["background"]?.let { store.root.evalOrNull(it) }
            val bg = theme.brush(rootBg, store.dark)
            Box(
                Modifier.fillMaxSize().then(if (bg != null) Modifier.background(bg) else Modifier)
                    .then(if (safeArea) Modifier.windowInsetsPadding(WindowInsets.safeDrawing) else Modifier),
                contentAlignment = Alignment.Center,
            ) { NodeView(store.doc.root, store.root) }
        }
    }
}

@Composable
private fun Fallback(onRetry: () -> Unit) {
    Column(Modifier.fillMaxSize(), Arrangement.Center, Alignment.CenterHorizontally) {
        Text(stringResource(R.string.wb_error))
        TextButton(onClick = onRetry) { Text(stringResource(R.string.wb_retry)) }
    }
}

@Composable
private fun rememberEnv(brand: String, dark: Boolean): Map<String, Value> {
    val density = LocalDensity.current
    val locale = LocalConfiguration.current.locales[0].language.lowercase()
    val window = LocalWindowInfo.current.containerSize
    val insets = WindowInsets.safeDrawing
    val context = LocalContext.current
    val reduceMotion = remember { Settings.Global.getFloat(context.contentResolver, Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f }
    fun dp(px: Int) = Value.Num((px / density.density).toDouble())
    return mapOf(
        "platform" to Value.Str("android"),
        "locale" to Value.Str(locale),
        "brand" to Value.Str(brand),
        "colorScheme" to Value.Str(if (dark) "dark" else "light"),
        "reduceMotion" to Value.of(reduceMotion),
        "screenWidth" to dp(window.width),
        "screenHeight" to dp(window.height),
        "safeTop" to dp(insets.getTop(density)),
        "safeBottom" to dp(insets.getBottom(density)),
        "timeZone" to Value.Str(TimeZone.getDefault().id),
    )
}
