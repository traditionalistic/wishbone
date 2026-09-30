package app.wishbone.ui

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.util.Base64

class PreviewTest {
    @Test
    fun helloOffersToRender() {
        val o = Json.parseToJsonElement(PreviewClient.hello("Pixel 9")).jsonObject
        assertEquals("android", o["platform"]?.jsonPrimitive?.content)
        assertEquals("Pixel 9", o["device"]?.jsonPrimitive?.content)
        assertEquals(WISHBONE_RUNTIME, o["runtime"]?.jsonPrimitive?.content)
        assertEquals(true, o["render"]?.jsonPrimitive?.boolean)
    }

    @Test
    fun renderJobIsParsedAtTheBoundary() {
        val data = """{"me":{"ok":true,"value":{"id":1}}}"""
        val m = PreviewClient.parse("""{"type":"render","job":17,"screen":"raffle.tickets","document":{"screen":"raffle.tickets"},"data":$data}""")
        val expected = RenderJob(17, "raffle.tickets", JsonObject(mapOf("screen" to JsonPrimitive("raffle.tickets"))), Json.parseToJsonElement(data).jsonObject)
        assertEquals(PreviewMessage.Render(expected), m)
        assertEquals(JsonObject(emptyMap()), (PreviewClient.parse("""{"type":"render","job":1,"screen":"x","document":{},"data":null}""") as PreviewMessage.Render).job.data)
        assertNull(PreviewClient.parse("""{"type":"render","screen":"x","document":{}}""")) // no job id: nothing to answer
        assertNull(PreviewClient.parse("""{"type":"frame","screen":"x"}"""))
    }

    @Test
    fun shotRepliesCarryTheJobAndAJpegWithoutLineBreaks() {
        val jpeg = ByteArray(100) { it.toByte() } // 136 base64 chars: past the 76-char MIME line
        val o = PreviewClient.shot(17, jpeg, 411, 914, settled = true)
        assertEquals("shot", o["type"]?.jsonPrimitive?.content)
        assertEquals(17, o["job"]?.jsonPrimitive?.int)
        assertEquals(411, o["width"]?.jsonPrimitive?.int)
        assertEquals(914, o["height"]?.jsonPrimitive?.int)
        assertEquals(true, o["settled"]?.jsonPrimitive?.boolean)
        val image = o["image"]!!.jsonPrimitive.content
        assertFalse(image.contains('\n') || image.contains('\r'))
        assertArrayEquals(jpeg, Base64.getDecoder().decode(image))

        val e = PreviewClient.shotError(17, "app is in the background")
        assertEquals(setOf("type", "job", "error"), e.keys)
        assertEquals(17, e["job"]?.jsonPrimitive?.int)
        assertEquals("app is in the background", e["error"]?.jsonPrimitive?.content)
    }
}
