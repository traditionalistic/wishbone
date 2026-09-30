package app.wishbone.ui

import android.graphics.Bitmap
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asAndroidBitmap
import androidx.compose.ui.graphics.layer.GraphicsLayer
import androidx.compose.ui.graphics.rememberGraphicsLayer
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.hideFromAccessibility
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import app.wishbone.core.Value
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import java.io.ByteArrayOutputStream
import kotlin.math.roundToInt

/** Spec 2026-09-30 §2.2, the three calibration knobs: the floor every shot waits, the poll period, and the deadline. */
internal const val SETTLE_FLOOR_MS = 700L
internal const val SETTLE_POLL_MS = 50L
internal const val SETTLE_QUIET_POLLS = 2
internal const val SETTLE_DEADLINE_MS = 6_000L

internal sealed interface Settle {
    /** Poll again in [SETTLE_POLL_MS]; [quietPolls] is the streak to pass back next time. */
    data class Wait(val quietPolls: Int) : Settle
    data class Capture(val settled: Boolean) : Settle
}

/**
 * The settle rule, one poll at a time: the first [SETTLE_FLOOR_MS] only wait (the longest motion token and the
 * platform's own transitions, which no counter sees); after that, [SETTLE_QUIET_POLLS] polls in a row with no image
 * load in flight and no finite timeline playing capture `settled`; at [SETTLE_DEADLINE_MS] the shot is taken as it is.
 * [quietPolls] is the streak the previous poll returned (0 for the first).
 */
internal fun settle(elapsedMs: Long, imagesInFlight: Int, finiteTimelineRunning: Boolean, quietPolls: Int = 0): Settle {
    if (elapsedMs >= SETTLE_DEADLINE_MS) return Settle.Capture(settled = false)
    if (elapsedMs < SETTLE_FLOOR_MS) return Settle.Wait(0)
    val quiet = if (imagesInFlight == 0 && !finiteTimelineRunning) quietPolls + 1 else 0
    return if (quiet >= SETTLE_QUIET_POLLS) Settle.Capture(settled = true) else Settle.Wait(quiet)
}

/** Shot pixels per dp (spec §2.3). */
private const val SHOT_SCALE = 2
private const val SHOT_JPEG_QUALITY = 70

/**
 * A `render` job's screen, laid out like the live one in the same window (size, insets, scheme) but never drawn to
 * it: the content is recorded into a [GraphicsLayer] instead, and read back as a JPEG once the screen settles
 * ([settle]). Not touchable, hidden from accessibility, params = the document's defaults, data = the job's; the
 * store runs in capture mode ([CaptureEffects]). [onShot] gets the one reply and is expected to drop the layer.
 */
@Composable
internal fun CaptureLayer(wishbone: Wishbone, job: RenderJob, onShot: (JsonObject) -> Unit) {
    val env = rememberEnv(wishbone)
    val dark = env["colorScheme"] == Value.Str("dark")
    val holder = remember(job) {
        val navigator = Navigator(job.screen, emptyMap())
        ScreenHolder(wishbone, navigator, navigator.entries.first(), capture = true).also { h ->
            h.env = env
            if (!h.apply(job.document, job.data, job.document.revision())) h.state = LoadState.Failed
        }
    }
    DisposableEffect(holder) { onDispose { holder.dispose() } }
    val layer = rememberGraphicsLayer()
    val loads = remember { ImageLoads() }
    val density = LocalDensity.current.density
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(holder) {
        if (holder.state == LoadState.Failed) return@LaunchedEffect onShot(PreviewClient.shotError(job.job, "bad document"))
        // no frames come while the window isn't shown, so the layer would stay blank
        if (!lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) return@LaunchedEffect onShot(PreviewClient.shotError(job.job, "app is in the background"))
        val start = System.nanoTime()
        var quiet = 0
        while (true) {
            delay(SETTLE_POLL_MS)
            val store = holder.store
            val now = System.nanoTime()
            when (val s = settle((now - start) / 1_000_000, loads.inFlight, store != null && store.timelines.finiteRunning(now, store.reduceMotion), quiet)) {
                is Settle.Wait -> quiet = s.quietPolls
                is Settle.Capture -> return@LaunchedEffect onShot(
                    if (!lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) PreviewClient.shotError(job.job, "app is in the background")
                    else layer.shot(job.job, density, s.settled),
                )
            }
        }
    }
    CompositionLocalProvider(LocalWishbone provides wishbone, LocalImageLoads provides loads) {
        Box(
            Modifier.fillMaxSize().untouchable().clearAndSetSemantics { hideFromAccessibility() }
                .drawWithContent { layer.record { this@drawWithContent.drawContent() } }
                .background(if (dark) Color.Black else Color.White),
        ) { ScreenContent(holder, env, wishbone.theme, safeArea = true) }
    }
}

/** Consumes every pointer event before the content sees it: a touch that fell through the live screen must not act here. */
private fun Modifier.untouchable() = pointerInput(Unit) {
    awaitPointerEventScope {
        while (true) awaitPointerEvent(PointerEventPass.Initial).changes.forEach { it.consume() }
    }
}

/** The recorded layer as a `shot`: [SHOT_SCALE] px per dp, JPEG [SHOT_JPEG_QUALITY]; an error when nothing was recorded. */
private suspend fun GraphicsLayer.shot(job: Int, density: Float, settled: Boolean): JsonObject {
    val width = (size.width / density).roundToInt()
    val height = (size.height / density).roundToInt()
    if (width == 0 || height == 0) return PreviewClient.shotError(job, "nothing drawn")
    val image = toImageBitmap().asAndroidBitmap()
    val jpeg = withContext(Dispatchers.Default) {
        val software = if (image.config == Bitmap.Config.HARDWARE) image.copy(Bitmap.Config.ARGB_8888, false) else image
        val scaled = Bitmap.createScaledBitmap(software, width * SHOT_SCALE, height * SHOT_SCALE, true)
        ByteArrayOutputStream().also { scaled.compress(Bitmap.CompressFormat.JPEG, SHOT_JPEG_QUALITY, it) }.toByteArray()
    }
    return PreviewClient.shot(job, jpeg, width, height, settled)
}
