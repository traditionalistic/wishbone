package app.wishbone.core

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * A runtime DSL value. Immutable: every "mutation" (path assignment, `with`, list ops, ...)
 * produces a new [Value] rather than changing one in place. [ListV.items] and [Rec.fields] must
 * therefore never be mutated after construction — always build a new List/Map.
 */
sealed interface Value {
    object Null : Value {
        override fun toString(): String = "Null"
    }

    data class Bool(val value: Boolean) : Value

    /**
     * IEEE-754 equality (`NaN != NaN`, `-0.0 == 0.0`) — NOT the default `data class equals()`,
     * which Kotlin generates via boxed `Double.equals()` (structural: `NaN.equals(NaN) == true`,
     * `(-0.0).equals(0.0) == false`). contract/stdlib.json `notes`: "numbers by value (NaN != NaN)".
     */
    class Num(val value: Double) : Value {
        override fun equals(other: Any?): Boolean = other is Num && this.value == other.value
        override fun hashCode(): Int = value.hashCode()
        override fun toString(): String = "Num(value=$value)"
    }

    data class Str(val value: String) : Value
    data class ListV(val items: List<Value>) : Value
    data class Rec(val fields: Map<String, Value>) : Value
    data class Lambda(val params: List<String>, val body: Expr, val closure: Locals?) : Value

    companion object {
        val TRUE: Value = Bool(true)
        val FALSE: Value = Bool(false)

        fun of(b: Boolean): Value = if (b) TRUE else FALSE
        fun of(n: Double): Value = Num(n)
        fun of(s: String): Value = Str(s)
        fun of(items: List<Value>): Value = ListV(items)
        fun rec(fields: Map<String, Value>): Value = Rec(fields)
        fun rec(vararg pairs: Pair<String, Value>): Value = Rec(linkedMapOf(*pairs))

        /** Parses a JSON document into a [Value] (`Document.parse` does the same for a whole document). */
        fun fromJsonString(text: String): Value = fromJson(Json.parseToJsonElement(text))

        /** JSON numbers become [Num] (`Double`); this is the only numeric representation. */
        fun fromJson(element: JsonElement): Value = when (element) {
            is JsonNull -> Null
            is JsonArray -> ListV(element.map { fromJson(it) })
            is JsonObject -> Rec(element.mapValues { (_, v) -> fromJson(v) })
            is JsonPrimitive -> {
                if (element.isString) {
                    Str(element.content)
                } else {
                    val b = element.booleanOrNull
                    if (b != null) {
                        Bool(b)
                    } else {
                        Num(element.doubleOrNull ?: element.content.toDouble())
                    }
                }
            }
        }
    }
}

/** Inverse of [Value.fromJson]. Lambdas have no JSON form and encode as `null`. */
fun Value.toJson(): JsonElement = when (this) {
    Value.Null -> JsonNull
    is Value.Bool -> JsonPrimitive(value)
    is Value.Num -> JsonPrimitive(value)
    is Value.Str -> JsonPrimitive(value)
    is Value.ListV -> JsonArray(items.map { it.toJson() })
    is Value.Rec -> JsonObject(fields.mapValues { (_, v) -> v.toJson() })
    is Value.Lambda -> JsonNull
}

/** Convenience accessors used throughout the interpreter/stdlib. */
fun Value.asDoubleOrNull(): Double? = (this as? Value.Num)?.value
fun Value.asStringOrNull(): String? = (this as? Value.Str)?.value
fun Value.asBoolOrNull(): Boolean? = (this as? Value.Bool)?.value
fun Value.asListOrNull(): List<Value>? = (this as? Value.ListV)?.items
fun Value.asRecOrNull(): Map<String, Value>? = (this as? Value.Rec)?.fields
val Value.isNull: Boolean get() = this is Value.Null

/**
 * Deep structural equality per contract/stdlib.json: numbers by IEEE value (so `NaN != NaN`,
 * matching Kotlin's `Double`-typed `==`), strings by content, lists elementwise (order matters),
 * records by key set + values (order does NOT matter — `Map.equals` already ignores iteration
 * order), token values (plain records) by the same record rule. This is exactly what the
 * generated `data class` `equals()` does for every case, so plain `==` is deep-equal already;
 * this function exists to make that contract explicit and to be the one place callers use.
 */
fun deepEquals(a: Value, b: Value): Boolean = a == b
