package app.wishbone.ui

import android.net.Uri
import android.os.Build
import android.view.HapticFeedbackConstants
import android.view.View
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.AndroidUiDispatcher
import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineExceptionHandler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject

internal sealed interface LoadState {
    data object Loading : LoadState
    data object Failed : LoadState
    data class Ready(val store: Store) : LoadState
}

/**
 * One navigation entry's screen: stale-while-revalidate load (disk cache first, then
 * `If-None-Match` fetch), the [Store] (rebuilt on a new document, keeping state), and the effects
 * sink. Lives as long as the entry is on the stack, so state survives pushes on top of it.
 */
internal class ScreenHolder(val wishbone: Wishbone, val navigator: Navigator, val entry: Entry, private val slot: Boolean = false) {
    /** Main thread + the UI frame clock (transactions and `await play` count frames); a throwing host call is logged, not fatal. */
    val scope = CoroutineScope(
        SupervisorJob() + AndroidUiDispatcher.Main + CoroutineExceptionHandler { _, e -> wishbone.log("error", "screen ${entry.screen}: ${e.message ?: e}") },
    )
    var state: LoadState by mutableStateOf(LoadState.Loading)
    var env: Map<String, Value> = emptyMap()
    var view: View? = null
    private val effects = ScreenEffects(this)
    private val loader = ScreenLoader(wishbone.cache, entry.screen, entry.params, wishbone.cacheScope, slot)

    val store: Store? get() = (state as? LoadState.Ready)?.store

    /** DSL state for the first store: the screen came back into the composition from a host back stack. */
    var restored: StoreSnapshot? = null

    fun snapshot(): StoreSnapshot? = store?.savedSnapshot() ?: restored

    fun load() {
        if (store == null) state = LoadState.Loading
        scope.launch {
            val ok = loader.load(
                hasContent = { store != null },
                fetch = { etag -> wishbone.client.screen(entry.screen, entry.params, etag) },
                show = ::apply,
                refreshData = { data -> store?.putData(dataValues(data)) },
                log = { wishbone.log("warn", it) },
            )
            if (!ok) state = LoadState.Failed
        }
    }

    private var applied: Pair<JsonObject, JsonObject>? = null

    fun isApplied(document: JsonObject, data: JsonObject) = applied?.let { it.first === document && it.second === data } == true

    /** Rebuilds the store for a new document (keeping state); false when it doesn't decode (the current one stays). */
    fun apply(document: JsonObject, data: JsonObject, revision: Int): Boolean {
        applied = document to data
        val doc = try {
            Document.decode(document)
        } catch (e: Exception) {
            wishbone.log("error", "screen ${entry.screen}: bad document: ${e.message}")
            return false
        }
        state = LoadState.Ready(Store(doc, entry.params, dataValues(data), wishbone.theme, env, effects, scope, revision, store, slot, restored))
        restored = null
        return true
    }

    fun dispose() = scope.cancel()
}

/** Everything a handler can do beyond state/timelines/wait (those live in [Store]). */
private class ScreenEffects(private val holder: ScreenHolder) : EffectSink {
    private val wishbone get() = holder.wishbone
    private val screen get() = holder.store?.doc?.screen ?: holder.entry.screen

    override suspend fun call(op: String, args: Value, awaited: Boolean): Value =
        wishbone.client.call(screen, holder.store?.revision ?: 0, op, args)

    override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {
        val nav = holder.navigator
        fun str(i: Int) = (args.getOrNull(i) as? Value.Str)?.value
        fun rec(i: Int) = (args.getOrNull(i) as? Value.Rec)?.fields ?: emptyMap()
        when (effect) {
            "navigate" -> str(0)?.let { nav.navigate(it, rec(1), str(2)) }
            "back" -> nav.back()
            "present" -> str(0)?.let { nav.present(it, rec(1)) }
            "dismiss" -> nav.dismiss()
            "open" -> str(0)?.let { raw ->
                val uri = Uri.parse(wishbone.resolve(raw))
                val scheme = uri.scheme?.lowercase()
                val ownServer = raw.startsWith("/") && !raw.startsWith("//") // server-relative (dev servers may be plain http)
                if (scheme == "https" || ownServer || scheme in wishbone.host.allowedSchemes) wishbone.host.open(uri)
            }
            "haptic" -> holder.view?.performHapticFeedback(haptic(str(0)))
            "track" -> str(0)?.let { wishbone.host.track(it, rec(1).mapValues { e -> e.value.toAny() }) }
            "host" -> str(0)?.let { name ->
                val a = rec(1).mapValues { it.value.toAny() }
                if (await) wishbone.host.action(name, a) else holder.scope.launch { wishbone.host.action(name, a) }
            }
            "refresh" -> {
                val store = holder.store ?: return
                val name = str(0) ?: return
                runCatching { wishbone.client.data(store.doc.screen, store.revision, store.params, listOf(name)) }
                    .onSuccess { store.putData(it) }
                    .onFailure { wishbone.log("warn", "refresh $name: ${it.message}") }
            }
            else -> error("unknown effect '$effect'")
        }
    }

    override fun error(message: String) = wishbone.log("error", "$screen: $message")

    private fun haptic(kind: String?) = when (kind) {
        "light" -> HapticFeedbackConstants.CLOCK_TICK
        "medium" -> HapticFeedbackConstants.KEYBOARD_TAP
        "heavy" -> HapticFeedbackConstants.LONG_PRESS
        "selection" -> if (Build.VERSION.SDK_INT >= 34) HapticFeedbackConstants.SEGMENT_TICK else HapticFeedbackConstants.CLOCK_TICK
        "success" -> HapticFeedbackConstants.CONFIRM
        else -> HapticFeedbackConstants.REJECT // warning, error
    }
}
