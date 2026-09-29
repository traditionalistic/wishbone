package app.wishbone.ui

import app.wishbone.core.Value
import app.wishbone.core.toJson
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File
import java.security.MessageDigest

/** Last good screen response (document + etag + data), per screen id + params. */
data class CachedScreen(val etag: String, val revision: Int, val document: JsonObject, val data: JsonObject)

/**
 * File cache: one file per key, written atomically (tmp + rename), at most [MAX_ENTRIES] files (ruling R2: after a
 * write the oldest by modification time go — keys include params, so they'd grow without bound). Unreadable/corrupt
 * entries read as absent.
 */
class DiskCache(private val dir: File) {
    fun read(key: String): String? = runCatching { file(key).takeIf { it.exists() }?.readText() }.getOrNull()

    fun write(key: String, text: String) {
        runCatching {
            dir.mkdirs()
            val f = file(key)
            val tmp = File(dir, f.name + ".tmp")
            tmp.writeText(text)
            tmp.renameTo(f)
            val entries = dir.listFiles { e -> e.name.endsWith(".json") } ?: return
            if (entries.size > MAX_ENTRIES) {
                entries.map { it.lastModified() to it }.sortedByDescending { it.first }.drop(MAX_ENTRIES).forEach { it.second.delete() }
            }
        }
    }

    /** Deletes the entry; a missing one is fine. */
    fun remove(key: String) {
        runCatching { file(key).delete() }
    }

    /** [scope] = the Wishbone's cache partition ([DiskCache.scope]). */
    fun readScreen(id: String, params: Map<String, Value>, scope: String): CachedScreen? = runCatching {
        val o = Json.parseToJsonElement(read(screenKey(id, params, scope)) ?: return null).jsonObject
        CachedScreen(o["etag"]!!.jsonPrimitive.content, o["revision"]!!.jsonPrimitive.int, o["document"]!!.jsonObject, o["data"]!!.jsonObject)
    }.getOrNull()

    fun writeScreen(id: String, params: Map<String, Value>, scope: String, s: CachedScreen) {
        val o = JsonObject(mapOf("etag" to JsonPrimitive(s.etag), "revision" to JsonPrimitive(s.revision), "document" to s.document, "data" to s.data))
        write(screenKey(id, params, scope), o.toString())
    }

    private fun file(key: String) = File(dir, MessageDigest.getInstance("SHA-256").digest(key.toByteArray()).toHexString(0, 16) + ".json")

    companion object {
        const val MAX_ENTRIES = 64

        /** Server × host scope (e.g. the account) × brand: one cache partition. */
        fun scope(baseUrl: String, hostScope: String, brand: String) = "$baseUrl\n$hostScope\n$brand"

        fun screenKey(id: String, params: Map<String, Value>, scope: String): String =
            "screen\n$scope\n$id\n" + canonical(Value.Rec(params).toJson())

        private fun canonical(e: JsonElement): JsonElement = when (e) {
            is JsonObject -> JsonObject(e.toSortedMap().mapValues { canonical(it.value) })
            is JsonArray -> JsonArray(e.map { canonical(it) })
            else -> e
        }
    }
}
