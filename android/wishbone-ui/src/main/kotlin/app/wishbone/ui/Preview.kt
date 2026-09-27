package app.wishbone.ui

import android.os.Build
import android.os.Handler
import android.os.Looper
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

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

sealed interface PreviewMessage {
    data class Doc(val screen: String, val document: JsonObject, val data: JsonObject) : PreviewMessage
    data class Play(val timeline: String) : PreviewMessage
}

/**
 * Device side of the preview protocol (spec §5.4): `hello` on connect, `document`/`play` in,
 * `log` out; reconnects with exponential backoff (0.5 s … 10 s) until [close]. Callbacks on main.
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

    fun log(level: String, message: String) {
        socket?.send(JsonObject(mapOf("type" to JsonPrimitive("log"), "level" to JsonPrimitive(level), "message" to JsonPrimitive(message))).toString())
    }

    fun close() {
        closed = true
        main.removeCallbacksAndMessages(null)
        socket?.close(1000, null)
    }

    companion object {
        fun hello(device: String): String = JsonObject(
            mapOf("type" to JsonPrimitive("hello"), "platform" to JsonPrimitive("android"), "device" to JsonPrimitive(device), "runtime" to JsonPrimitive(WISHBONE_RUNTIME)),
        ).toString()

        fun parse(text: String): PreviewMessage? = runCatching {
            val o = Json.parseToJsonElement(text).jsonObject
            when (o["type"]?.jsonPrimitive?.contentOrNull) {
                "document" -> PreviewMessage.Doc(
                    o["screen"]?.jsonPrimitive?.contentOrNull ?: "",
                    o["document"]!!.jsonObject,
                    o["data"]?.takeIf { it !is JsonNull }?.jsonObject ?: JsonObject(emptyMap()),
                )
                "play" -> PreviewMessage.Play(o["timeline"]!!.jsonPrimitive.content)
                else -> null
            }
        }.getOrNull()
    }
}
