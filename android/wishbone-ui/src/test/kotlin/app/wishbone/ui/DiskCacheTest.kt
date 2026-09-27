package app.wishbone.ui

import app.wishbone.core.Value
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir
import java.io.File

class DiskCacheTest {
    @TempDir
    lateinit var dir: File

    private val doc = Json.parseToJsonElement("""{"screen":"s","root":{"t":"stack","id":"root"}}""").jsonObject
    private val data = Json.parseToJsonElement("""{"me":{"ok":true,"value":{"name":"Ivan"}}}""").jsonObject

    @Test
    fun screenRoundTripPerParams() {
        val cache = DiskCache(dir)
        val a = mapOf("id" to Value.Str("1"))
        val b = mapOf("id" to Value.Str("2"))
        cache.writeScreen("s", a, CachedScreen("\"4-abc\"", 4, doc, data))
        assertEquals(CachedScreen("\"4-abc\"", 4, doc, data), cache.readScreen("s", a))
        assertNull(cache.readScreen("s", b))
        assertNull(cache.readScreen("other", a))
    }

    @Test
    fun paramOrderDoesNotMatter() {
        val k1 = DiskCache.screenKey("s", linkedMapOf("a" to Value.Num(1.0), "b" to Value.Num(2.0)))
        val k2 = DiskCache.screenKey("s", linkedMapOf("b" to Value.Num(2.0), "a" to Value.Num(1.0)))
        assertEquals(k1, k2)
        assertNotEquals(k1, DiskCache.screenKey("s", emptyMap()))
    }

    @Test
    fun corruptOrMissingIsNull() {
        val cache = DiskCache(dir)
        assertNull(cache.readScreen("s", emptyMap()))
        cache.write(DiskCache.screenKey("s", emptyMap()), "{not json")
        assertNull(cache.readScreen("s", emptyMap()))
    }

    @Test
    fun rawTextRoundTrip() {
        val cache = DiskCache(File(dir, "nested/deeper"))
        cache.write("theme-korpu", "{\"x\":1}")
        assertEquals("{\"x\":1}", cache.read("theme-korpu"))
    }
}
