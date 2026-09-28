package app.wishbone.ui

import app.wishbone.core.Value
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okio.Buffer
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.util.UUID
import java.util.concurrent.TimeUnit

class ClientTest {
    private val host = object : WishboneHost {
        override suspend fun authHeaders() = mapOf("Authorization" to "Bearer t")
    }
    private val client = Client("http://10.0.2.2:8080/", "kz", host, locale = { "ru" })

    private fun body(r: okhttp3.Request) = Buffer().also { r.body!!.writeTo(it) }.readUtf8()

    @Test
    fun headersIncludeRuntimePlatformBrandLanguageAndHost() = runBlocking {
        val h = client.headers()
        assertEquals("1.0.0", h["X-Wishbone-Runtime"])
        assertEquals("android", h["X-Wishbone-Platform"])
        assertEquals("kz", h["X-Wishbone-Brand"])
        assertEquals("ru", h["Accept-Language"])
        assertEquals("Bearer t", h["Authorization"])
    }

    @Test
    fun screenIdsOutsideTheServerRuleFailWithoutARequest() = runBlocking {
        for (id in listOf("..", ".", "", "a/b", "Showcase")) {
            val e = runCatching { Client("http://127.0.0.1:1/", "kz", host).screen(id, emptyMap(), null) }.exceptionOrNull()
            assertEquals("SCREEN_NOT_FOUND", (e as? WishboneException)?.code, id)
        }
    }

    @Test
    fun assetHeadersCarryHostAuthOnlyToTheServerOrigin() = runBlocking {
        assertEquals("Bearer t", client.assetHeaders("http://10.0.2.2:8080/assets/a.riv".toHttpUrl())["Authorization"])
        for (other in listOf("http://cdn.example.com:8080/a.riv", "http://10.0.2.2:9090/a.riv", "https://10.0.2.2:8080/a.riv")) {
            val h = client.assetHeaders(other.toHttpUrl())
            assertEquals(setOf("X-Wishbone-Runtime", "X-Wishbone-Platform", "X-Wishbone-Brand"), h.keys, other)
        }
    }

    @Test
    fun screenRequestEncodesParamsAndEtag() {
        val r = client.screenRequest("showcase.promo", mapOf("id" to Value.Str("o 1")), "\"4-abc\"", mapOf("X-Wishbone-Brand" to "kz"))
        assertEquals("GET", r.method)
        assertEquals("/v1/screens/showcase.promo", r.url.encodedPath)
        assertEquals("{\"id\":\"o 1\"}", r.url.queryParameter("params"))
        assertEquals("\"4-abc\"", r.header("If-None-Match"))
        assertEquals("kz", r.header("X-Wishbone-Brand"))
        val plain = client.screenRequest("x", emptyMap(), null, emptyMap())
        assertNull(plain.url.queryParameter("params"))
        assertNull(plain.header("If-None-Match"))
    }

    @Test
    fun callRequestCarriesIdempotencyKey() {
        val r = client.callRequest("s", 4, "demo.subscribe", Value.rec("email" to Value.Str("a@b.c")), "k-1", emptyMap())
        assertEquals("POST", r.method)
        assertEquals("/v1/call", r.url.encodedPath)
        val json = Json.parseToJsonElement(body(r)).jsonObject
        assertEquals("s", json["screen"]!!.jsonPrimitive.content)
        assertEquals(4, json["revision"]!!.jsonPrimitive.content.toDouble().toInt())
        assertEquals("demo.subscribe", json["op"]!!.jsonPrimitive.content)
        assertEquals("a@b.c", json["args"]!!.jsonObject["email"]!!.jsonPrimitive.content)
        assertEquals("k-1", json["idempotencyKey"]!!.jsonPrimitive.content)
        assertNotEquals(Client.newKey(), Client.newKey())
        UUID.fromString(Client.newKey())
    }

    @Test
    fun dataRequestListsSources() {
        val r = client.dataRequest("s", 2, mapOf("id" to Value.Str("1")), listOf("me"), emptyMap())
        val json = Json.parseToJsonElement(body(r)).jsonObject
        assertEquals("/v1/data", r.url.encodedPath)
        assertEquals("[\"me\"]", json["sources"].toString())
        assertEquals("{\"id\":\"1\"}", json["params"].toString())
    }

    @Test
    fun parsesScreenResponse() {
        val text = """{"etag":"\"4-abc\"","revision":4,"document":{"screen":"s"},"data":{"me":{"ok":true,"value":{"name":"Ivan"}},"offers":{"ok":false,"error":{"code":"X","message":"m","retryable":true}}}}"""
        val r = ScreenResponse.parse(text)
        assertEquals("\"4-abc\"", r.etag)
        assertEquals(4, r.revision)
        assertEquals("s", r.document!!["screen"]!!.jsonPrimitive.content)
        val values = dataValues(r.data)
        assertEquals(Value.rec("name" to Value.Str("Ivan")), values["me"])
        assertEquals(Value.Null, values["offers"])
        assertNull(ScreenResponse.parse("""{"etag":"e","revision":1,"document":null,"data":{}}""").document)
    }

    @Test
    fun parsesErrorBody() {
        val e = parseError(404, """{"error":{"code":"SCREEN_NOT_FOUND","message":"no","retryable":false}}""")
        assertEquals("SCREEN_NOT_FOUND", e.code)
        assertEquals(404, e.status)
        assertEquals("HTTP", parseError(500, "<html>").code)
    }

    @Test
    fun callTransportFailureIsNetworkResult() = runBlocking {
        val offline = Client(
            "http://127.0.0.1:1", "korpu", object : WishboneHost {},
            http = OkHttpClient.Builder().connectTimeout(1, TimeUnit.SECONDS).build(),
        )
        val r = offline.call("s", 1, "demo.x", Value.rec())
        val rec = (r as Value.Rec).fields
        assertEquals(Value.FALSE, rec["ok"])
        assertEquals(Value.Str("NETWORK"), (rec["error"] as Value.Rec).fields["code"])
    }

    @Test
    fun previewMessages() {
        val hello = Json.parseToJsonElement(PreviewClient.hello("Pixel")).jsonObject
        assertEquals("hello", hello["type"]!!.jsonPrimitive.content)
        assertEquals("android", hello["platform"]!!.jsonPrimitive.content)
        assertEquals("1.0.0", hello["runtime"]!!.jsonPrimitive.content)
        val doc = PreviewClient.parse("""{"type":"document","screen":"s","document":{"screen":"s"},"data":{}}""")
        assertTrue(doc is PreviewMessage.Doc)
        assertEquals(PreviewMessage.Play("intro"), PreviewClient.parse("""{"type":"play","timeline":"intro"}"""))
        assertNull(PreviewClient.parse("""{"type":"weird"}"""))
        assertNull(PreviewClient.parse("nope"))
        val sessions = parseSessions("""[{"code":"K7Q2","screen":"showcase.promo","title":"Промо","devices":1,"updatedAt":"2026-09-28T00:00:00Z"}]""")
        assertEquals(listOf(PreviewSession("K7Q2", "showcase.promo", "Промо", 1)), sessions)
    }
}
