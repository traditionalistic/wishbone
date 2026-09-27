package app.wishbone.core

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/** Thrown when a document/expression/statement's wire JSON is structurally malformed (spec §3.8: "malformed JSON / missing required document fields fail decoding of the whole document"). An unrecognized tag is NOT malformed — it decodes to `.Unknown`. */
class AstDecodeException(message: String) : Exception(message)

/**
 * DSL expressions, wire form per spec §3.8. `["emit", name]` never appears on the wire (inlined
 * by the compiler) and has no case here.
 */
sealed interface Expr {
    data class Lit(val value: Value) : Expr
    data class StateRef(val name: String) : Expr
    data class InstRef(val name: String) : Expr
    data class ComputedRef(val name: String) : Expr
    data class DataRef(val name: String) : Expr
    data class LocalRef(val name: String) : Expr
    data class ParamRef(val name: String) : Expr
    data class EnvRef(val name: String) : Expr
    data class TokenRef(val group: String, val name: String) : Expr
    data class DriverRef(val kind: String, val id: String, val axis: String) : Expr
    object SelfPressed : Expr
    data class Get(val obj: Expr, val field: String) : Expr
    data class At(val obj: Expr, val key: Expr) : Expr
    data class BinOp(val op: String, val a: Expr, val b: Expr) : Expr
    data class UnOp(val op: String, val a: Expr) : Expr
    data class If(val cond: Expr, val then: Expr, val otherwise: Expr) : Expr
    data class Call(val name: String, val args: List<Expr>) : Expr
    data class UserCall(val name: String, val args: List<Expr>) : Expr
    data class Lam(val params: List<String>, val body: Expr) : Expr
    data class ListLit(val items: List<Expr>) : Expr
    data class RecLit(val fields: Map<String, Expr>) : Expr
    data class StrInterp(val parts: List<Expr>) : Expr
    data class Unknown(val tag: String) : Expr

    companion object {
        fun decode(json: JsonElement): Expr {
            val arr = json.asArray("expression")
            val tag = arr.tagAt(0)
            return when (tag) {
                "v" -> Lit(Value.fromJson(arr.at(1, tag)))
                "s" -> StateRef(arr.stringAt(1, tag))
                "i" -> InstRef(arr.stringAt(1, tag))
                "c" -> ComputedRef(arr.stringAt(1, tag))
                "d" -> DataRef(arr.stringAt(1, tag))
                "l" -> LocalRef(arr.stringAt(1, tag))
                "p" -> ParamRef(arr.stringAt(1, tag))
                "e" -> EnvRef(arr.stringAt(1, tag))
                "t" -> TokenRef(arr.stringAt(1, tag), arr.stringAt(2, tag))
                "dr" -> DriverRef(arr.stringAt(1, tag), arr.stringAt(2, tag), arr.stringAt(3, tag))
                "self" -> if (arr.stringAt(1, tag) == "pressed") SelfPressed else Unknown(tag)
                "get" -> Get(decode(arr.at(1, tag)), arr.stringAt(2, tag))
                "at" -> At(decode(arr.at(1, tag)), decode(arr.at(2, tag)))
                "op" -> BinOp(arr.stringAt(1, tag), decode(arr.at(2, tag)), decode(arr.at(3, tag)))
                "un" -> UnOp(arr.stringAt(1, tag), decode(arr.at(2, tag)))
                "if" -> If(decode(arr.at(1, tag)), decode(arr.at(2, tag)), decode(arr.at(3, tag)))
                "f" -> Call(arr.stringAt(1, tag), arr.exprListAt(2, tag))
                "u" -> UserCall(arr.stringAt(1, tag), arr.exprListAt(2, tag))
                "lam" -> Lam(arr.at(1, tag).asArray(tag).map { it.jsonPrimitive.content }, decode(arr.at(2, tag)))
                "list" -> ListLit(arr.at(1, tag).asArray(tag).map { decode(it) })
                "rec" -> RecLit(arr.at(1, tag).asObject(tag).mapValues { (_, v) -> decode(v) })
                "str" -> StrInterp(arr.exprListAt(1, tag))
                else -> Unknown(tag)
            }
        }
    }
}

/** DSL statements (handler/event bodies), wire form per spec §3.8. */
sealed interface Stmt {
    data class Set(val scope: String, val name: String, val path: List<Expr>, val value: Expr) : Stmt
    data class Let(val name: String, val value: Expr) : Stmt
    data class If(val cond: Expr, val then: List<Stmt>, val otherwise: List<Stmt>) : Stmt
    data class Anim(val motion: Expr, val body: List<Stmt>) : Stmt
    data class CallOp(val op: String, val args: Expr, val resultName: String?, val await: Boolean) : Stmt
    data class Do(val effect: String, val args: List<Expr>, val await: Boolean) : Stmt
    data class Run(val handler: String, val args: List<Expr>) : Stmt
    data class Unknown(val tag: String) : Stmt

    companion object {
        fun decode(json: JsonElement): Stmt {
            val arr = json.asArray("statement")
            val tag = arr.tagAt(0)
            return when (tag) {
                "set" -> Set(
                    arr.stringAt(1, tag),
                    arr.stringAt(2, tag),
                    arr.at(3, tag).asArray(tag).map { Expr.decode(it) },
                    Expr.decode(arr.at(4, tag)),
                )
                "let" -> Let(arr.stringAt(1, tag), Expr.decode(arr.at(2, tag)))
                "if" -> If(
                    Expr.decode(arr.at(1, tag)),
                    arr.at(2, tag).asArray(tag).map { decode(it) },
                    arr.at(3, tag).asArray(tag).map { decode(it) },
                )
                "anim" -> Anim(Expr.decode(arr.at(1, tag)), arr.at(2, tag).asArray(tag).map { decode(it) })
                "call" -> CallOp(
                    arr.stringAt(1, tag),
                    Expr.decode(arr.at(2, tag)),
                    arr.at(3, tag).let { if (it is JsonNull) null else it.jsonPrimitive.content },
                    arr.at(4, tag).jsonPrimitive.boolean,
                )
                "do" -> Do(
                    arr.stringAt(1, tag),
                    arr.at(2, tag).asArray(tag).map { Expr.decode(it) },
                    arr.at(3, tag).jsonPrimitive.boolean,
                )
                "run" -> Run(arr.stringAt(1, tag), arr.at(2, tag).asArray(tag).map { Expr.decode(it) })
                else -> Unknown(tag)
            }
        }
    }
}

// --- decode helpers -------------------------------------------------------

private fun JsonElement.asArray(what: String): JsonArray =
    this as? JsonArray ?: throw AstDecodeException("expected a JSON array for $what, got $this")

private fun JsonElement.asObject(what: String): JsonObject =
    this as? JsonObject ?: throw AstDecodeException("expected a JSON object for $what, got $this")

private fun JsonArray.at(index: Int, tag: String): JsonElement =
    getOrNull(index) ?: throw AstDecodeException("'$tag' node missing element at index $index")

private fun JsonArray.tagAt(index: Int): String =
    at(index, "tag").jsonPrimitive.content

private fun JsonArray.stringAt(index: Int, tag: String): String =
    at(index, tag).jsonPrimitive.content

private fun JsonArray.exprListAt(index: Int, tag: String): List<Expr> =
    at(index, tag).asArray(tag).map { Expr.decode(it) }

/** An immutable linked chain of local bindings (let, handler/lambda params, repeat vars, `event`). Not part of [Scope]. */
class Locals(val name: String, val value: Value, val parent: Locals?)

fun Locals?.lookup(name: String): Value? {
    var cur = this
    while (cur != null) {
        if (cur.name == name) return cur.value
        cur = cur.parent
    }
    return null
}

fun Locals?.bind(name: String, value: Value): Locals = Locals(name, value, this)
