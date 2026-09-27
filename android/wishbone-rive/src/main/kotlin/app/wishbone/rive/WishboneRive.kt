// ponytail: rive-android 11 deprecates RiveAnimationView/inputs/events in favour of data binding (app.rive);
// renderer-decisions.md specifies this legacy view API — migrate when screens use data-bound Rive files.
@file:Suppress("DEPRECATION")

package app.wishbone.rive

import android.content.Context
import android.util.Log
import android.view.MotionEvent
import android.widget.FrameLayout
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import app.rive.runtime.kotlin.RiveAnimationView
import app.rive.runtime.kotlin.controllers.RiveFileController
import app.rive.runtime.kotlin.core.Fit
import app.rive.runtime.kotlin.core.RendererType
import app.rive.runtime.kotlin.core.Rive
import app.rive.runtime.kotlin.core.RiveEvent
import app.wishbone.core.Value
import app.wishbone.ui.LocalWishbone
import app.wishbone.ui.WishboneRenderers
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.LifecycleRegistry
import androidx.lifecycle.setViewTreeLifecycleOwner
import kotlinx.coroutines.CancellationException

/** `rive` primitive (spec §4.4) on rive-android: call [install] once, before rendering screens. */
object WishboneRive {
    /** [renderer]: Rive's GPU renderer by default; [RendererType.Canvas] where GLES output is unavailable (e.g. some emulators). */
    fun install(context: Context, renderer: RendererType = RendererType.Rive) {
        Rive.init(context.applicationContext, renderer)
        WishboneRenderers.register("rive") { props, modifier, emit -> RiveNode(props, modifier, emit) }
    }
}

/**
 * Taps pass through to ancestors (renderer-decisions: rive views don't intercept taps in V1). Each node has
 * its own lifecycle, destroyed when the node leaves composition, so the Rive controller is released then
 * rather than with the Activity.
 */
private class PassThrough(context: Context) : FrameLayout(context), LifecycleOwner {
    private val registry = LifecycleRegistry(this).apply { currentState = Lifecycle.State.RESUMED }
    override val lifecycle: Lifecycle get() = registry

    init {
        setViewTreeLifecycleOwner(this)
    }

    val rive = RiveAnimationView(context).also { addView(it, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT)) }
    var loaded: ByteArray? = null
    override fun dispatchTouchEvent(ev: MotionEvent) = false

    fun release() {
        registry.currentState = Lifecycle.State.DESTROYED
    }
}

/** Loads the file from `url`, runs `stateMachine`, pushes `inputs` (bool/number) on every change, Rive events → `event` `{name}`. */
@Composable
private fun RiveNode(p: Map<String, Value>, modifier: Modifier, emit: (String, Value) -> Unit) {
    val url = (p["url"] as? Value.Str)?.value ?: return
    val machine = (p["stateMachine"] as? Value.Str)?.value?.ifEmpty { null }
    val artboard = (p["artboard"] as? Value.Str)?.value?.ifEmpty { null }
    val inputs = (p["inputs"] as? Value.Rec)?.fields ?: emptyMap()
    val client = LocalWishbone.current?.client
    val bytes by produceState<ByteArray?>(null, url, client) {
        value = try {
            client?.bytes(url)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            Log.w("Wishbone", "rive $url: $e")
            null
        }
    }
    val onEvent by rememberUpdatedState(emit)
    AndroidView(
        factory = { context ->
            PassThrough(context).also { frame ->
                frame.rive.addEventListener(object : RiveFileController.RiveEventListener {
                    override fun notifyEvent(event: RiveEvent) {
                        frame.post { onEvent("event", Value.rec("name" to Value.Str(event.name))) }
                    }
                })
            }
        },
        modifier = modifier,
        onRelease = { it.release() },
        update = { frame ->
            val file = bytes
            if (file != null && frame.loaded !== file) {
                frame.loaded = file
                runCatching { frame.rive.setRiveBytes(file, artboardName = artboard, stateMachineName = machine, autoplay = true, fit = Fit.CONTAIN) }
                    .onFailure { Log.w("Wishbone", "rive $url: $it") }
            }
            if (frame.loaded != null && machine != null) {
                for ((name, v) in inputs) when (v) {
                    is Value.Bool -> frame.rive.setBooleanState(machine, name, v.value)
                    is Value.Num -> frame.rive.setNumberState(machine, name, v.value.toFloat())
                    else -> Unit
                }
            }
        },
    )
}
