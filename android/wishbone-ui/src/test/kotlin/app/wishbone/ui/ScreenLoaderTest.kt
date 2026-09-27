package app.wishbone.ui

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
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

    /** Fake screen: records what was shown; documents with "bad" fail to decode. */
    private inner class Screen {
        val shown = mutableListOf<JsonObject>()
        val etags = mutableListOf<String?>()
        var refreshed: JsonObject? = null
        fun show(d: JsonObject, data: JsonObject, rev: Int) = if (d.containsKey("bad")) false else shown.add(d)
    }

    private val cache get() = DiskCache(dir)
    private fun loader() = ScreenLoader(cache, "s", emptyMap(), "korpu")

    private fun run(screen: Screen, response: () -> ScreenResponse) = runBlocking {
        loader().load({ screen.shown.isNotEmpty() }, { etag -> screen.etags += etag; response() }, screen::show, { screen.refreshed = it }, {})
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
}
