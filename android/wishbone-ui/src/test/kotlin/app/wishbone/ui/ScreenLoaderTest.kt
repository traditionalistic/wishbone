package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.Value
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.File
import java.io.IOException

class ScreenLoaderTest {
    @TempDir
    lateinit var dir: File

    private fun obj(s: String) = Json.parseToJsonElement(s).jsonObject
    private val v1 = obj("""{"screen":"s","v":1}""")
    private val v2 = obj("""{"screen":"s","v":2}""")
    private val bad = obj("""{"screen":"s","bad":true}""")
    private val data1 = obj("""{"me":{"ok":true,"value":1}}""")
    private val data2 = obj("""{"me":{"ok":true,"value":2}}""")

    /** What a fake screen shows once decoded (disk/network) or taken from the memo. */
    private val decoded = Document.parse("""{"screen":"s","root":{"t":"stack","id":"root"}}""")

    /** Fake screen: records what was shown; documents with "bad" fail to decode. */
    private inner class Screen {
        val shown = mutableListOf<JsonObject>()
        var fromMemo: Document? = null
        val etags = mutableListOf<String?>()
        var refreshed: JsonObject? = null
        fun show(d: JsonObject, data: JsonObject, rev: Int) = if (d.containsKey("bad")) false else shown.add(d)
        fun current(): Document? = fromMemo ?: decoded.takeIf { shown.isNotEmpty() }
    }

    private val cache get() = DiskCache(dir)
    private var now = 0L
    private val memo = SlotMemo { now }
    private var requests = 0

    /** A screen opening, or (with [slot]) a slot entering the composition: warm content on the calling thread, then the loader. */
    private fun run(screen: Screen, slot: Boolean = false, params: Map<String, Value> = emptyMap(), response: () -> ScreenResponse): Boolean {
        val loader = ScreenLoader(cache, "s", params, "korpu", memo.takeIf { slot })
        val cached = loader.warm({ d, _, _ -> screen.fromMemo = d }, screen::show)
        return runBlocking { loader.load(cached, screen::current, { etag -> requests++; screen.etags += etag; response() }, screen::show, { screen.refreshed = it }, {}) }
    }

    @Test
    fun aWarmScreenIsOnScreenBeforeTheLoaderRuns() {
        cache.writeScreen("s", emptyMap(), "korpu", CachedScreen("\"1-a\"", 1, v1, data1))
        val s = Screen()
        val cached = ScreenLoader(cache, "s", emptyMap(), "korpu").warm({ _, _, _ -> error("no memo for a screen") }, s::show)
        assertEquals(listOf(v1), s.shown) // synchronously, on the calling (main) thread: the first frame has it
        assertEquals("\"1-a\"", cached?.etag)
    }

    @Test
    fun freshLoadShowsAndCaches() {
        val s = Screen()
        assertTrue(run(s) { ScreenResponse("\"1-a\"", 1, v1, data1) })
        assertEquals(listOf(v1), s.shown)
        assertEquals(listOf<String?>(null), s.etags)
        assertEquals(CachedScreen("\"1-a\"", 1, v1, data1), cache.readScreen("s", emptyMap(), "korpu"))
    }

    @Test
    fun cachedFirstThenSwapWithEtag() {
        cache.writeScreen("s", emptyMap(), "korpu", CachedScreen("\"1-a\"", 1, v1, data1))
        val s = Screen()
        assertTrue(run(s) { ScreenResponse("\"2-b\"", 2, v2, data2) })
        assertEquals(listOf(v1, v2), s.shown)
        assertEquals(listOf<String?>("\"1-a\""), s.etags)
        assertEquals(v2, cache.readScreen("s", emptyMap(), "korpu")!!.document)
    }

    @Test
    fun badServerDocumentKeepsTheCache() {
        cache.writeScreen("s", emptyMap(), "korpu", CachedScreen("\"1-a\"", 1, v1, data1))
        val s = Screen()
        assertTrue(run(s) { ScreenResponse("\"2-x\"", 2, bad, data2) })
        assertEquals(listOf(v1), s.shown)
        assertEquals(CachedScreen("\"1-a\"", 1, v1, data1), cache.readScreen("s", emptyMap(), "korpu"))
    }

    @Test
    fun notModifiedRefreshesDataOnly() {
        cache.writeScreen("s", emptyMap(), "korpu", CachedScreen("\"1-a\"", 1, v1, data1))
        val s = Screen()
        assertTrue(run(s) { ScreenResponse("\"1-a\"", 1, null, data2) })
        assertEquals(listOf(v1), s.shown)
        assertEquals(data2, s.refreshed)
        assertEquals(CachedScreen("\"1-a\"", 1, v1, data2), cache.readScreen("s", emptyMap(), "korpu"))
    }

    @Test
    fun failureWithoutCacheFallsBackAndBadFirstDocumentShowsNothing() {
        assertFalse(run(Screen()) { throw IOException("offline") })
        assertFalse(run(Screen()) { ScreenResponse("\"1-x\"", 1, bad, data1) })
        assertEquals(null, cache.readScreen("s", emptyMap(), "korpu"))
    }

    private val notFound = WishboneException("SCREEN_NOT_FOUND", "not found", 404, false)

    @Test
    fun unpublishedSlotDropsItsCacheAndShowsNothing() {
        cache.writeScreen("s", emptyMap(), "korpu", CachedScreen("\"1-a\"", 1, v1, data1))
        val s = Screen()
        assertFalse(run(s, slot = true) { throw notFound }) // → Failed: the slot renders nothing
        assertEquals(listOf(v1), s.shown) // the cached one was on screen until the answer came
        assertNull(cache.readScreen("s", emptyMap(), "korpu"))
    }

    @Test
    fun otherSlotFailuresKeepTheCache() {
        cache.writeScreen("s", emptyMap(), "korpu", CachedScreen("\"1-a\"", 1, v1, data1))
        assertTrue(run(Screen(), slot = true) { throw IOException("offline") })
        assertEquals(CachedScreen("\"1-a\"", 1, v1, data1), cache.readScreen("s", emptyMap(), "korpu"))
    }

    @Test
    fun unpublishedScreenKeepsItsCache() {
        cache.writeScreen("s", emptyMap(), "korpu", CachedScreen("\"1-a\"", 1, v1, data1))
        assertTrue(run(Screen()) { throw notFound })
        assertEquals(CachedScreen("\"1-a\"", 1, v1, data1), cache.readScreen("s", emptyMap(), "korpu"))
    }

    @Test
    fun unchangedAnswerIsNotWrittenAgainButMarkedUsed() {
        // spaced JSON: a rewrite would come out compact, so equal bytes afterwards mean no rewrite
        cache.write(DiskCache.screenKey("s", emptyMap(), "korpu"), """{ "etag": "\"1-a\"", "revision": 1, "document": $v1, "data": $data1 }""")
        val file = dir.listFiles()!!.single().apply { setLastModified(1_000_000) }
        val bytes = file.readBytes()
        assertTrue(run(Screen()) { ScreenResponse("\"1-a\"", 1, null, data1) })
        assertArrayEquals(bytes, file.readBytes())
        assertTrue(file.lastModified() > 1_000_000) // newest again for the cap: the entries revalidated most stay
    }

    private val key = DiskCache.screenKey("s", emptyMap(), "korpu")

    @Test
    fun aSlotReappearsFromMemoryForThirtySeconds() {
        assertTrue(run(Screen(), slot = true) { ScreenResponse("\"1-a\"", 1, v1, data1) }) // cold: network
        assertEquals(1, requests)
        now = 29_999
        val again = Screen()
        assertTrue(run(again, slot = true) { ScreenResponse("\"1-a\"", 1, null, data2) })
        assertEquals(1, requests) // no request
        assertEquals(decoded, again.fromMemo) // on screen from memory…
        assertTrue(again.shown.isEmpty()) // …without reading or decoding the disk cache
        now = 30_000
        val stale = Screen()
        assertTrue(run(stale, slot = true) { ScreenResponse("\"1-a\"", 1, null, data2) })
        assertEquals(2, requests)
        assertEquals(listOf<String?>("\"1-a\""), stale.etags)
        assertEquals(data2, stale.refreshed)
        assertEquals(data2, memo[key]!!.cached.data)
        now = 59_999 // the revalidation counts as the last network load
        run(Screen(), slot = true) { ScreenResponse("\"1-a\"", 1, null, data2) }
        assertEquals(2, requests)
    }

    @Test
    fun aFailedRevalidationKeepsTheMemoStale() {
        run(Screen(), slot = true) { ScreenResponse("\"1-a\"", 1, v1, data1) }
        now = 30_000
        assertTrue(run(Screen(), slot = true) { throw IOException("offline") }) // the memo's content stays on screen
        run(Screen(), slot = true) { ScreenResponse("\"1-a\"", 1, null, data1) }
        assertEquals(3, requests) // asked again: the failed load was no network answer
    }

    @Test
    fun unpublishedSlotDropsItsMemoAndDiskCache() {
        run(Screen(), slot = true) { ScreenResponse("\"1-a\"", 1, v1, data1) }
        assertNotNull(memo[key])
        now = 30_000
        assertFalse(run(Screen(), slot = true) { throw notFound })
        assertNull(memo[key])
        assertNull(cache.readScreen("s", emptyMap(), "korpu"))
        val next = Screen()
        assertFalse(run(next, slot = true) { throw notFound }) // the next appearance starts empty and asks again
        assertNull(next.current())
        assertEquals(3, requests)
    }

    @Test
    fun slotsWithOtherParamsHaveTheirOwnEntries() {
        val a = mapOf("id" to Value.Str("a"))
        val b = mapOf("id" to Value.Str("b"))
        run(Screen(), slot = true, params = a) { ScreenResponse("\"1-a\"", 1, v1, data1) }
        run(Screen(), slot = true, params = b) { ScreenResponse("\"2-b\"", 2, v2, data2) }
        assertEquals(2, requests)
        assertEquals(v1, memo[DiskCache.screenKey("s", a, "korpu")]!!.cached.document)
        assertEquals(v2, memo[DiskCache.screenKey("s", b, "korpu")]!!.cached.document)
        run(Screen(), slot = true, params = a) { ScreenResponse("\"1-a\"", 1, null, data1) }
        assertEquals(2, requests)
    }
}
