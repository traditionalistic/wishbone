package app.wishbone.ui

import app.wishbone.core.Value
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject

/**
 * Stale-while-revalidate for one screen (spec §6.3): show the cached response, fetch with
 * `If-None-Match`, swap in the fresh document. The cache is written only after a document decoded
 * (a bad server document never replaces the last good one); a `document: null` answer refreshes data.
 */
internal class ScreenLoader(private val cache: DiskCache, private val id: String, private val params: Map<String, Value>, private val brand: String) {
    /**
     * [show] renders a document and returns false when it doesn't decode; [hasContent] tells whether
     * something is on screen. Returns false when nothing can be shown (→ fallback view).
     */
    suspend fun load(
        hasContent: () -> Boolean,
        fetch: suspend (etag: String?) -> ScreenResponse,
        show: (document: JsonObject, data: JsonObject, revision: Int) -> Boolean,
        refreshData: (JsonObject) -> Unit,
        log: (String) -> Unit,
    ): Boolean {
        val cached = withContext(Dispatchers.IO) { cache.readScreen(id, params, brand) }
        if (cached != null && !hasContent()) show(cached.document, cached.data, cached.revision)
        val r = try {
            fetch(cached?.etag?.takeIf { hasContent() })
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("screen $id: ${e.message}")
            return hasContent()
        }
        val fresh = r.document
        val stored = when {
            fresh != null -> if (show(fresh, r.data, r.revision)) CachedScreen(r.etag, r.revision, fresh, r.data) else null.also { log("screen $id: bad document, keeping the cached one") }
            cached != null && hasContent() -> CachedScreen(cached.etag, cached.revision, cached.document, r.data).also { refreshData(r.data) }
            else -> null
        }
        if (stored != null) withContext(Dispatchers.IO) { cache.writeScreen(id, params, brand, stored) }
        return hasContent()
    }
}
