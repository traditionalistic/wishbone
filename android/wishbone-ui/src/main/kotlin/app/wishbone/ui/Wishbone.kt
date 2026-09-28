package app.wishbone.ui

import android.content.Context
import android.net.Uri
import android.util.Log
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import app.wishbone.core.Value
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/** What the host app provides (spec §6.3). Every member has a no-op default. */
interface WishboneHost {
    suspend fun authHeaders(): Map<String, String> = emptyMap()

    /** Called for `open(url)` with https or a scheme from [allowedSchemes]; anything else is ignored by the SDK. */
    fun open(url: Uri) {}
    val allowedSchemes: Set<String> get() = emptySet()

    fun track(event: String, params: Map<String, Any?>) {}
    suspend fun action(name: String, args: Map<String, Any?>) {}

    /** Content for a `native` node, or null to render nothing. */
    fun nativeView(name: String, props: Map<String, Any?>): (@Composable () -> Unit)? = null

    /**
     * Partitions the disk cache (with the server address and brand), e.g. the signed-in account id,
     * so an account switch never shows another user's cached screens.
     */
    val cacheScope: String get() = ""
}

/**
 * SDK entry point: server, brand, host, optional scheme override (`"light"`/`"dark"`, null = system).
 * Holds the client, the disk cache and the current theme (disk-cached, bundled `korpu` fallback).
 */
class Wishbone(
    context: Context,
    val baseUrl: String,
    val brand: String = "korpu",
    val host: WishboneHost = object : WishboneHost {},
    colorScheme: String? = null,
) {
    /** `"light"`/`"dark"` overrides the system scheme (e.g. the host's own theme setting), null follows the system. */
    var colorScheme: String? by mutableStateOf(colorScheme)

    val client = Client(baseUrl, brand, host)
    internal val cache = DiskCache(File(context.cacheDir, "wishbone"))

    /** Cache partition of screen and theme answers: server, host scope, brand. */
    internal val cacheScope: String get() = DiskCache.scope(baseUrl, host.cacheScope, brand)

    // ponytail: the cached/bundled theme is read on the calling thread (a few KB, once per instance)
    var theme: Theme by mutableStateOf(
        cache.read("theme-$cacheScope")?.let { runCatching { Theme.parse(it) }.getOrNull() }
            ?: Theme.parse(context.assets.open("wishbone/korpu.json").bufferedReader().use { it.readText() }),
    )
        private set

    internal var previewLog: ((level: String, message: String) -> Unit)? = null
    private var themeRequested = false

    /** `GET /v1/theme` with the cached ETag; keeps the current theme on 304 or failure. */
    internal suspend fun refreshTheme() {
        if (themeRequested) return
        themeRequested = true
        val scope = cacheScope
        val etag = withContext(Dispatchers.IO) { cache.read("theme-etag-$scope") }
        val fetched = runCatching { client.theme(etag) }.onFailure { log("warn", "theme: ${it.message}") }.getOrNull() ?: return
        val parsed = runCatching { Theme.parse(fetched.body) }.getOrNull() ?: return
        theme = parsed
        withContext(Dispatchers.IO) {
            cache.write("theme-$scope", fetched.body)
            fetched.etag?.let { cache.write("theme-etag-$scope", it) }
        }
    }

    /** `/assets/…`-style URLs are relative to the server. */
    fun resolve(url: String): String = if (url.startsWith("/") && !url.startsWith("//")) baseUrl.trimEnd('/') + url else url

    internal fun log(level: String, message: String) {
        Log.println(if (level == "error") Log.ERROR else Log.WARN, "Wishbone", message)
        previewLog?.invoke(level, message)
    }
}

/** The [Wishbone] rendering the current screen (adapters use its client for assets). */
val LocalWishbone = staticCompositionLocalOf<Wishbone?> { null }

fun Value.toAny(): Any? = when (this) {
    Value.Null -> null
    is Value.Bool -> value
    is Value.Num -> value
    is Value.Str -> value
    is Value.ListV -> items.map { it.toAny() }
    is Value.Rec -> fields.mapValues { it.value.toAny() }
    is Value.Lambda -> null
}
