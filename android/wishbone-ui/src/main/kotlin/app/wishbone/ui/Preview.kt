package app.wishbone.ui

import android.os.Build
import android.os.Handler
import android.os.Looper
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.int
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.Base64

data class PreviewSession(val code: String, val screen: String, val title: String, val devices: Int)

internal fun parseSessions(text: String): List<PreviewSession> = Json.parseToJsonElement(text).jsonArray.map {
    val o = it.jsonObject
    PreviewSession(
        o["code"]!!.jsonPrimitive.content,
        o["screen"]?.jsonPrimitive?.contentOrNull ?: "",
        o["title"]?.jsonPrimitive?.contentOrNull ?: "",
        o["devices"]?.jsonPrimitive?.intOrNull ?: 0,
    )
}

/** A `render` job (spec 2026-09-30 §3): draw [document] with [data] off screen and answer with exactly one `shot`. */
data class RenderJob(val job: Int, val screen: String, val document: JsonObject, val data: JsonObject)

sealed interface PreviewMessage {
    data class Doc(val screen: String, val document: JsonObject, val data: JsonObject) : PreviewMessage
    data class Play(val timeline: String) : PreviewMessage
    data class Render(val job: RenderJob) : PreviewMessage
}

/**
 * Device side of the preview protocol (spec §5.4, 2026-09-30 §3): `hello` (with `render: true`) on connect,
 * `document`/`play`/`render` in, `log`/`shot` out; reconnects with exponential backoff (0.5 s … 10 s) until
 * [close]. Callbacks on main.
 */
class PreviewClient(
    private val wishbone: Wishbone,
    private val code: String,
    private val onMessage: (PreviewMessage) -> Unit,
) {
    private val main = Handler(Looper.getMainLooper())
    private var socket: WebSocket? = null
    private var closed = false
    private var attempt = 0

    fun connect() {
        if (closed) return
        val url = wishbone.client.url("v1/preview/ws").newBuilder().addQueryParameter("session", code).build()
        socket = wishbone.client.webSocket(Request.Builder().url(url).build(), object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                main.post { attempt = 0 }
                webSocket.send(hello("${Build.MANUFACTURER} ${Build.MODEL}"))
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                parse(text)?.let { m -> main.post { if (!closed) onMessage(m) } }
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                retry()
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                retry()
            }
        })
    }

    private fun retry() {
        main.post {
            if (!closed) main.postDelayed({ connect() }, minOf(10_000L, 500L shl minOf(attempt++, 5)))
        }
    }

    fun log(level: String, message: String) = send(JsonObject(mapOf("type" to JsonPrimitive("log"), "level" to JsonPrimitive(level), "message" to JsonPrimitive(message))))

    /** A [shot] or [shotError] reply; dropped while disconnected (the server times the job out and re-issues it). */
    fun send(message: JsonObject) {
        socket?.send(message.toString())
    }

    fun close() {
        closed = true
        main.removeCallbacksAndMessages(null)
        socket?.close(1000, null)
    }

    companion object {
        fun hello(device: String): String = JsonObject(
            mapOf(
                "type" to JsonPrimitive("hello"), "platform" to JsonPrimitive("android"), "device" to JsonPrimitive(device),
                "runtime" to JsonPrimitive(WISHBONE_RUNTIME), "render" to JsonPrimitive(true),
            ),
        ).toString()

        fun parse(text: String): PreviewMessage? = runCatching {
            val o = Json.parseToJsonElement(text).jsonObject
            fun data() = o["data"]?.takeIf { it !is JsonNull }?.jsonObject ?: JsonObject(emptyMap())
            when (o["type"]?.jsonPrimitive?.contentOrNull) {
                "document" -> PreviewMessage.Doc(o["screen"]?.jsonPrimitive?.contentOrNull ?: "", o["document"]!!.jsonObject, data())
                "play" -> PreviewMessage.Play(o["timeline"]!!.jsonPrimitive.content)
                "render" -> PreviewMessage.Render(
                    RenderJob(o["job"]!!.jsonPrimitive.int, o["screen"]?.jsonPrimitive?.contentOrNull ?: "", o["document"]!!.jsonObject, data()),
                )
                else -> null
            }
        }.getOrNull()

        /** `shot` for [job]: [jpeg] as base64 without line breaks, [width]/[height] in dp, [settled] per spec §2.2. */
        fun shot(job: Int, jpeg: ByteArray, width: Int, height: Int, settled: Boolean): JsonObject = JsonObject(
            mapOf(
                "type" to JsonPrimitive("shot"), "job" to JsonPrimitive(job), "image" to JsonPrimitive(Base64.getEncoder().encodeToString(jpeg)),
                "width" to JsonPrimitive(width), "height" to JsonPrimitive(height), "settled" to JsonPrimitive(settled),
            ),
        )

        fun shotError(job: Int, message: String): JsonObject =
            JsonObject(mapOf("type" to JsonPrimitive("shot"), "job" to JsonPrimitive(job), "error" to JsonPrimitive(message)))
    }
}
