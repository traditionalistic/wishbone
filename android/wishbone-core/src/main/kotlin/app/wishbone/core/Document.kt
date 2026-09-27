package app.wishbone.core

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.double
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** A user-defined pure function (`fn`): body sees only its own params, other fns, stdlib, env, tokens. */
data class Fn(val params: List<String>, val body: Expr)

/** A `handler` declaration: a statement list run to completion (or aborted) per invocation. */
data class Handler(val params: List<String>, val body: List<Stmt>)

/** `data` arguments may reference only literals or a screen `param` — never a general expression. */
sealed interface DataArg {
    data class Lit(val value: Value) : DataArg
    data class Param(val name: String) : DataArg
}

data class DataDecl(val name: String, val op: String, val args: Map<String, DataArg>, val required: Boolean)

/** `repeat` on a node: `over` a list expr, materialized under locals `as`/`index` (default item/index). */
data class Repeat(val over: Expr, val asName: String, val indexName: String, val key: Expr?)

/**
 * A tree node. Only the fields the interpreter/event-dispatch harness need are typed
 * (`props`/`children`/`when`/`rep`/`on`); `anim`, `tr`, `match`, `drag`, `inst`, `a11y` are
 * rendering/template concerns out of scope for `:wishbone-core` (core-decisions.md) and are kept
 * verbatim in [raw] so nothing is lost.
 */
data class Node(
    val type: String,
    val id: String,
    val props: Map<String, Expr>,
    val children: List<Node>,
    val whenExpr: Expr?,
    val rep: Repeat?,
    val on: Map<String, List<Stmt>>,
    val raw: JsonObject,
)

data class Timeline(
    val duration: Double,
    val repeat: Int,
    val autoreverse: Boolean,
    val autoplay: Boolean,
    val tracks: List<Track>,
)

data class Track(val target: String, val prop: String, val stagger: Double, val keys: List<Key>)

data class Key(val t: Double, val v: Double, val ease: List<Double>?)

data class Document(
    val format: Int,
    val runtimeMin: String,
    val screen: String,
    val revision: Int,
    val params: Map<String, Expr>,
    val state: List<Pair<String, Expr>>,
    val computed: Map<String, Expr>,
    val fns: Map<String, Fn>,
    val handlers: Map<String, Handler>,
    val data: List<DataDecl>,
    val timelines: Map<String, Timeline>,
    val root: Node,
) {
    companion object {
        fun parse(json: String): Document = decode(Json.parseToJsonElement(json).jsonObject)

        fun decode(obj: JsonObject): Document {
            val root = obj["root"]?.jsonObject ?: throw AstDecodeException("document missing 'root'")
            return Document(
                format = obj["format"]?.jsonPrimitive?.int ?: 1,
                runtimeMin = obj["runtimeMin"]?.jsonPrimitive?.contentOrNull ?: "",
                screen = obj["screen"]?.jsonPrimitive?.contentOrNull
                    ?: throw AstDecodeException("document missing 'screen'"),
                revision = obj["revision"]?.jsonPrimitive?.int ?: 0,
                params = (obj["params"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> Expr.decode(v) },
                state = (obj["state"]?.jsonArray ?: JsonArray(emptyList())).map { decodeStateEntry(it) },
                computed = (obj["computed"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> Expr.decode(v) },
                fns = (obj["fns"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> decodeFn(v.jsonObject) },
                handlers = (obj["handlers"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> decodeHandler(v.jsonObject) },
                data = (obj["data"]?.jsonArray ?: JsonArray(emptyList())).map { decodeDataDecl(it.jsonObject) },
                timelines = (obj["timelines"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> decodeTimeline(v.jsonObject) },
                root = decodeNode(root),
            )
        }

        private fun decodeStateEntry(el: JsonElement): Pair<String, Expr> {
            val arr = el.jsonArray
            return arr[0].jsonPrimitive.content to Expr.decode(arr[1])
        }

        private fun decodeFn(obj: JsonObject): Fn =
            Fn(
                params = (obj["params"]?.jsonArray ?: JsonArray(emptyList())).map { it.jsonPrimitive.content },
                body = Expr.decode(obj["body"] ?: throw AstDecodeException("fn missing 'body'")),
            )

        private fun decodeHandler(obj: JsonObject): Handler =
            Handler(
                params = (obj["params"]?.jsonArray ?: JsonArray(emptyList())).map { it.jsonPrimitive.content },
                body = (obj["body"]?.jsonArray ?: JsonArray(emptyList())).map { Stmt.decode(it) },
            )

        private fun decodeDataDecl(obj: JsonObject): DataDecl =
            DataDecl(
                name = obj["name"]?.jsonPrimitive?.content ?: throw AstDecodeException("data decl missing 'name'"),
                op = obj["op"]?.jsonPrimitive?.content ?: throw AstDecodeException("data decl missing 'op'"),
                args = (obj["args"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> decodeDataArg(v) },
                required = obj["required"]?.jsonPrimitive?.boolean ?: false,
            )

        private fun decodeDataArg(el: JsonElement): DataArg {
            if (el is JsonObject) {
                val paramName = el["\$param"]?.jsonPrimitive?.contentOrNull
                if (paramName != null) return DataArg.Param(paramName)
            }
            return DataArg.Lit(Value.fromJson(el))
        }

        private fun decodeRepeat(obj: JsonObject): Repeat =
            Repeat(
                over = Expr.decode(obj["over"] ?: throw AstDecodeException("rep missing 'over'")),
                asName = obj["as"]?.jsonPrimitive?.content ?: "item",
                indexName = obj["index"]?.jsonPrimitive?.content ?: "index",
                key = obj["key"]?.let { Expr.decode(it) },
            )

        private fun decodeNode(obj: JsonObject): Node =
            Node(
                type = obj["t"]?.jsonPrimitive?.content ?: throw AstDecodeException("node missing 't'"),
                id = obj["id"]?.jsonPrimitive?.content ?: throw AstDecodeException("node missing 'id'"),
                props = (obj["p"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> Expr.decode(v) },
                children = (obj["c"]?.jsonArray ?: JsonArray(emptyList())).map { decodeNode(it.jsonObject) },
                whenExpr = obj["when"]?.let { Expr.decode(it) },
                rep = obj["rep"]?.let { decodeRepeat(it.jsonObject) },
                on = (obj["on"]?.jsonObject ?: JsonObject(emptyMap())).mapValues { (_, v) -> v.jsonArray.map { Stmt.decode(it) } },
                raw = obj,
            )

        /** Exposed (not `private`) so the test-only ConformanceTest can decode a vector's standalone `timeline` field the same way. */
        internal fun decodeTimeline(obj: JsonObject): Timeline =
            Timeline(
                duration = obj["duration"]?.jsonPrimitive?.double ?: 0.0,
                repeat = obj["repeat"]?.jsonPrimitive?.int ?: 1,
                autoreverse = obj["autoreverse"]?.jsonPrimitive?.boolean ?: false,
                autoplay = obj["autoplay"]?.jsonPrimitive?.boolean ?: false,
                tracks = (obj["tracks"]?.jsonArray ?: JsonArray(emptyList())).map { decodeTrack(it.jsonObject) },
            )

        private fun decodeTrack(obj: JsonObject): Track =
            Track(
                target = obj["target"]?.jsonPrimitive?.content ?: throw AstDecodeException("track missing 'target'"),
                prop = obj["prop"]?.jsonPrimitive?.content ?: throw AstDecodeException("track missing 'prop'"),
                stagger = obj["stagger"]?.jsonPrimitive?.double ?: 0.0,
                keys = (obj["keys"]?.jsonArray ?: JsonArray(emptyList())).map { decodeKey(it.jsonObject) },
            )

        private fun decodeKey(obj: JsonObject): Key =
            Key(
                t = obj["t"]?.jsonPrimitive?.double ?: throw AstDecodeException("key missing 't'"),
                v = obj["v"]?.jsonPrimitive?.double ?: throw AstDecodeException("key missing 'v'"),
                ease = obj["ease"]?.jsonArray?.map { it.jsonPrimitive.double },
            )
    }
}
