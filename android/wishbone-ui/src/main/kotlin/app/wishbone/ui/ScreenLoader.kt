package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.Value
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject

/**
 * Ruling R1: a [Wishbone]'s last network answer per slot cache key (screen id + params): the decoded document, data,
 * etag and when it came (monotonic ms from [clock]). A slot scrolling back into a feed shows it at once, without disk
 * or decoding, and asks the server again only once it is [FRESH_MS] old. Main thread only.
 * ponytail: unbounded — one entry per distinct slot id + params this instance has loaded; cap it if hosts vary params per row.
 */
internal class SlotMemo(private val clock: () -> Long = { System.nanoTime() / 1_000_000 }) {
    class Entry(val cached: CachedScreen, val doc: Document, val loadedAt: Long)

    private val entries = HashMap<String, Entry>()

    operator fun get(key: String): Entry? = entries[key]

    fun put(key: String, cached: CachedScreen, doc: Document) {
        entries[key] = Entry(cached, doc, clock())
    }

    fun remove(key: String) {
        entries.remove(key)
    }

    fun fresh(key: String): Boolean = entries[key]?.let { clock() - it.loadedAt < FRESH_MS } == true

    companion object {
        const val FRESH_MS = 30_000L
    }
}

/**
 * Stale-while-revalidate for one screen (spec §6.3): show the cached response, fetch with
 * `If-None-Match`, swap in the fresh document. The cache is written only after a document decoded
 * (a bad server document never replaces the last good one) and only when the etag or data changed;
 * a `document: null` answer refreshes data. A slot ([memo] set) remembers its network answers (ruling R1);
 * its `SCREEN_NOT_FOUND` drops the memo entry and the disk cache and shows nothing, even over the cached content.
 */
internal class ScreenLoader(
    private val cache: DiskCache,
    private val id: String,
    private val params: Map<String, Value>,
    private val scope: String,
    private val memo: SlotMemo? = null,
) {
    private val key = DiskCache.screenKey(id, params, scope)

    /** A screen's disk cache entry, read off the main thread. */
    suspend fun read(): CachedScreen? = withContext(Dispatchers.IO) { cache.readScreen(id, params, scope) }

    /**
     * A slot entering the composition, on the calling thread so warm content is there in its first frame: the memo's
     * decoded document ([show]), else the disk cache ([apply]; ponytail: a main-thread read and decode of one small
     * file, only when the memo has nothing). Returns the entry [load] revalidates with.
     */
    fun warm(show: (Document, JsonObject, Int) -> Unit, apply: (JsonObject, JsonObject, Int) -> Boolean): CachedScreen? {
        memo?.get(key)?.let {
            show(it.doc, it.cached.data, it.cached.revision)
            return it.cached
        }
        return cache.readScreen(id, params, scope)?.also { apply(it.document, it.data, it.revision) }
    }

    /**
     * [cached] is the entry already read ([read]/[warm]), [current] the document on screen (null: nothing), [show]
     * decodes and renders a document, false when it doesn't decode. Returns false when nothing can be shown (→ fallback view).
     */
    suspend fun load(
        cached: CachedScreen?,
        current: () -> Document?,
        fetch: suspend (etag: String?) -> ScreenResponse,
        show: suspend (document: JsonObject, data: JsonObject, revision: Int) -> Boolean,
        refreshData: (JsonObject) -> Unit,
        log: (String) -> Unit,
    ): Boolean {
        if (memo?.fresh(key) == true) return current() != null // R1: the network answered less than 30 s ago
        if (cached != null && current() == null) show(cached.document, cached.data, cached.revision)
        val r = try {
            fetch(cached?.etag?.takeIf { current() != null })
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("screen $id: ${e.message}")
            if (memo != null && (e as? WishboneException)?.code == "SCREEN_NOT_FOUND") {
                // unpublished: a cached banner must not outlive it (spec 2026-09-28 §2.3)
                memo.remove(key)
                withContext(Dispatchers.IO) { cache.remove(key) }
                return false
            }
            return current() != null
        }
        val fresh = r.document
        val stored = when {
            fresh != null -> if (show(fresh, r.data, r.revision)) CachedScreen(r.etag, r.revision, fresh, r.data) else null.also { log("screen $id: bad document, keeping the cached one") }
            cached != null && current() != null -> CachedScreen(cached.etag, cached.revision, cached.document, r.data).also { refreshData(r.data) }
            else -> null
        }
        if (stored != null) {
            current()?.let { memo?.put(key, stored, it) }
            if (cached == null || stored.etag != cached.etag || stored.data != cached.data) withContext(Dispatchers.IO) { cache.writeScreen(id, params, scope, stored) }
        }
        return current() != null
    }
}
