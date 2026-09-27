package app.wishbone.ui

import android.net.Uri
import android.os.Build
import android.view.HapticFeedbackConstants
import android.view.View
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Value
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
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
internal class ScreenHolder(val wishbone: Wishbone, val navigator: Navigator, val entry: Entry) {
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    var state: LoadState by mutableStateOf(LoadState.Loading)
    var env: Map<String, Value> = emptyMap()
    var view: View? = null
    private val effects = ScreenEffects(this)

    val store: Store? get() = (state as? LoadState.Ready)?.store

    fun load() {
        state = if (store == null) LoadState.Loading else state
        scope.launch {
            val cached = withContext(Dispatchers.IO) { wishbone.cache.readScreen(entry.screen, entry.params) }
            if (cached != null && store == null) apply(cached.document, cached.data, cached.revision)
            try {
                val r = wishbone.client.screen(entry.screen, entry.params, cached?.etag?.takeIf { store != null })
                val doc = r.document ?: cached?.document ?: throw WishboneException("EMPTY", "no document", 200, true)
                if (r.document != null || store == null) apply(doc, r.data, r.revision) else store?.data?.putAll(dataValues(r.data))
                withContext(Dispatchers.IO) { wishbone.cache.writeScreen(entry.screen, entry.params, CachedScreen(r.etag, r.revision, doc, r.data)) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                wishbone.log("warn", "screen ${entry.screen}: ${e.message}")
                if (store == null) state = LoadState.Failed
            }
        }
    }

    /** A document that fails to decode counts as a network error: the current one (if any) stays. */
    private var applied: Pair<JsonObject, JsonObject>? = null

    fun isApplied(document: JsonObject, data: JsonObject) = applied?.let { it.first === document && it.second === data } == true

    fun apply(document: JsonObject, data: JsonObject, revision: Int) {
        applied = document to data
        val doc = try {
            Document.decode(document)
        } catch (e: Exception) {
            wishbone.log("error", "screen ${entry.screen}: bad document: ${e.message}")
            if (store == null) state = LoadState.Failed
            return
        }
        state = LoadState.Ready(Store(doc, entry.params, dataValues(data), wishbone.theme, env, effects, scope, revision, store))
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
                val ownServer = raw.startsWith("/") // server-relative (dev servers may be plain http)
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
                    .onSuccess { store.data.putAll(it) }
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
