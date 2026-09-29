package app.wishbone.ui

import app.wishbone.core.Value
import app.wishbone.core.toJson
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Headers.Companion.toHeaders
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.Dispatcher
import okhttp3.OkHttpClient
import okhttp3.Response
import kotlin.coroutines.resumeWithException
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.Locale
import java.util.UUID
import java.util.concurrent.TimeUnit

const val WISHBONE_RUNTIME = "1.1.0"

class WishboneException(val code: String, message: String, val status: Int, val retryable: Boolean) : Exception(message)

/** `GET /v1/screens/{id}` body (contract/api.md); `document` is null when If-None-Match matched. */
data class ScreenResponse(val etag: String, val revision: Int, val document: JsonObject?, val data: JsonObject) {
    companion object {
        fun parse(text: String): ScreenResponse {
            val o = Json.parseToJsonElement(text).jsonObject
            return ScreenResponse(
                etag = o["etag"]?.jsonPrimitive?.contentOrNull ?: "",
                revision = o["revision"]?.jsonPrimitive?.intOrNull ?: 0,
                document = o["document"]?.takeIf { it !is JsonNull }?.jsonObject,
                data = o["data"]?.takeIf { it !is JsonNull }?.jsonObject ?: JsonObject(emptyMap()),
            )
        }
    }
}

/** Wire data `{name: {ok, value?|error?}}` → values; a failed source reads as null. */
fun dataValues(data: JsonObject): Map<String, Value> = data.mapValues { (_, r) ->
    val o = r as? JsonObject
    if (o?.get("ok")?.jsonPrimitive?.booleanOrNull == true) o["value"]?.let { Value.fromJson(it) } ?: Value.Null else Value.Null
}

private val screenId = Regex("[a-z0-9][a-z0-9._-]{0,63}")

internal fun parseError(status: Int, body: String): WishboneException {
    val e = runCatching { Json.parseToJsonElement(body).jsonObject["error"]!!.jsonObject }.getOrNull()
        ?: return WishboneException("HTTP", "HTTP $status", status, status >= 500)
    return WishboneException(
        e["code"]?.jsonPrimitive?.contentOrNull ?: "HTTP",
        e["message"]?.jsonPrimitive?.contentOrNull ?: "HTTP $status",
        status,
        e["retryable"]?.jsonPrimitive?.booleanOrNull ?: false,
    )
}

internal class ThemeFetch(val body: String, val etag: String?)

/** HTTP client for the public API (contract/api.md): OkHttp; answers are read and parsed on its threads, a cancelled caller cancels the call. */
class Client(
    baseUrl: String,
    private val brand: String,
    private val host: WishboneHost,
    // screen / data / call share the host with /assets/* downloads: OkHttp's default 5 per host would queue a tap's call behind them
    private val http: OkHttpClient = OkHttpClient.Builder().dispatcher(Dispatcher().apply { maxRequestsPerHost = 16 }).build(),
    private val locale: () -> String = { Locale.getDefault().language.lowercase() },
) {
    private val base = baseUrl.trimEnd('/').toHttpUrl()
    private val jsonType = "application/json; charset=utf-8".toMediaType()

    private fun sdkHeaders(): Map<String, String> = linkedMapOf(
        "X-Wishbone-Runtime" to WISHBONE_RUNTIME,
        "X-Wishbone-Platform" to "android",
        "X-Wishbone-Brand" to brand,
    )

    internal suspend fun headers(): Map<String, String> = sdkHeaders() + ("Accept-Language" to locale()) + host.authHeaders()

    /** Host headers (auth) go only to the server's own origin; any other host gets just the X-Wishbone-* ones. */
    internal suspend fun assetHeaders(url: HttpUrl): Map<String, String> =
        if (url.scheme == base.scheme && url.host == base.host && url.port == base.port) headers() else sdkHeaders()

    internal fun screenRequest(id: String, params: Map<String, Value>, etag: String?, headers: Map<String, String>): Request {
        val url = base.newBuilder().addPathSegments("v1/screens").addPathSegment(id)
            .apply { if (params.isNotEmpty()) addQueryParameter("params", Value.Rec(params).toJson().toString()) }
            .build()
        return Request.Builder().url(url).headers(headers.toHeaders()).apply { etag?.let { header("If-None-Match", it) } }.build()
    }

    internal fun callRequest(screen: String, revision: Int, op: String, args: Value, key: String, headers: Map<String, String>) =
        post(
            "v1/call",
            JsonObject(mapOf("screen" to JsonPrimitive(screen), "revision" to JsonPrimitive(revision), "op" to JsonPrimitive(op), "args" to args.toJson(), "idempotencyKey" to JsonPrimitive(key))),
            headers,
        )

    internal fun dataRequest(screen: String, revision: Int, params: Map<String, Value>, sources: List<String>, headers: Map<String, String>) =
        post(
            "v1/data",
            JsonObject(mapOf("screen" to JsonPrimitive(screen), "revision" to JsonPrimitive(revision), "params" to Value.Rec(params).toJson(), "sources" to JsonArray(sources.map { JsonPrimitive(it) }))),
            headers,
        )

    private fun post(path: String, body: JsonObject, headers: Map<String, String>) =
        Request.Builder().url(base.newBuilder().addPathSegments(path).build()).headers(headers.toHeaders()).post(body.toString().toRequestBody(jsonType)).build()

    internal fun url(path: String) = base.newBuilder().addPathSegments(path).build()

    suspend fun screen(id: String, params: Map<String, Value>, etag: String?): ScreenResponse {
        // the server's id rule: anything else (e.g. `..` from a deeplink) would resolve to another path
        if (!screenId.matches(id)) throw WishboneException("SCREEN_NOT_FOUND", "invalid screen id \"$id\"", 404, false)
        return execute(screenRequest(id, params, etag, headers())) { code, body ->
            if (code != 200) throw parseError(code, body)
            ScreenResponse.parse(body)
        }
    }

    suspend fun data(screen: String, revision: Int, params: Map<String, Value>, sources: List<String>): Map<String, Value> =
        execute(dataRequest(screen, revision, params, sources, headers())) { code, body ->
            if (code != 200) throw parseError(code, body)
            dataValues(Json.parseToJsonElement(body).jsonObject)
        }

    /** Never throws: transport failure → `{ok: false, error: {code: "NETWORK", message}}`, protocol errors likewise. */
    suspend fun call(screen: String, revision: Int, op: String, args: Value): Value = try {
        execute(callRequest(screen, revision, op, args, newKey(), headers())) { code, body ->
            if (code == 200) Value.fromJsonString(body) else parseError(code, body).let { failure(it.code, it.message ?: "") }
        }
    } catch (e: IOException) {
        failure("NETWORK", e.message ?: "network error")
    } catch (e: IllegalArgumentException) {
        failure("NETWORK", e.message ?: "bad response")
    }

    internal suspend fun theme(etag: String?): ThemeFetch? {
        val url = base.newBuilder().addPathSegments("v1/theme").addQueryParameter("brand", brand).build()
        val req = Request.Builder().url(url).headers(headers().toHeaders()).apply { etag?.let { header("If-None-Match", it) } }.build()
        return send(req) { r ->
            when (r.code) {
                304 -> null
                200 -> ThemeFetch(r.body!!.string(), r.header("ETag"))
                else -> throw parseError(r.code, r.body?.string().orEmpty())
            }
        }
    }

    suspend fun previewSessions(): List<PreviewSession> =
        execute(Request.Builder().url(url("v1/preview/sessions")).headers(headers().toHeaders()).build()) { code, body ->
            if (code != 200) throw parseError(code, body)
            parseSessions(body)
        }

    /** GET an asset (e.g. a Rive file) with the SDK headers (+ host headers on the server's origin); cancelled with the caller. */
    suspend fun bytes(url: String): ByteArray {
        val u = url.toHttpUrl()
        return send(Request.Builder().url(u).headers(assetHeaders(u).toHeaders()).build()) {
            if (it.isSuccessful) it.body!!.bytes() else throw IOException("HTTP ${it.code} $url")
        }
    }

    /** Preview socket with keep-alive pings, so dead connections are noticed and reconnected. */
    internal fun webSocket(request: Request, listener: okhttp3.WebSocketListener) =
        http.newBuilder().pingInterval(20, TimeUnit.SECONDS).build().newWebSocket(request, listener)

    private suspend fun <T> execute(r: Request, read: (code: Int, body: String) -> T): T = send(r) { read(it.code, it.body?.string().orEmpty()) }

    /** Runs [r] on OkHttp's threads, where [read] turns the response into the result; a cancelled caller cancels the call. */
    private suspend fun <T> send(r: Request, read: (Response) -> T): T {
        val call = http.newCall(r)
        return suspendCancellableCoroutine { cont ->
            cont.invokeOnCancellation { call.cancel() }
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) = cont.resumeWithException(e)
                override fun onResponse(call: Call, response: Response) = cont.resumeWith(runCatching { response.use(read) })
            })
        }
    }

    companion object {
        fun newKey(): String = UUID.randomUUID().toString()

        private fun failure(code: String, message: String) = Value.rec(
            "ok" to Value.FALSE,
            "error" to Value.rec("code" to Value.Str(code), "message" to Value.Str(message)),
        )
    }
}
