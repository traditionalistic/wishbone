package app.wishbone.core

import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ValueTest {

    @Test
    fun `fromJson maps every JSON shape`() {
        assertEquals(Value.Null, Value.fromJson(Json.parseToJsonElement("null")))
        assertEquals(Value.Bool(true), Value.fromJson(Json.parseToJsonElement("true")))
        assertEquals(Value.Num(1.5), Value.fromJson(Json.parseToJsonElement("1.5")))
        assertEquals(Value.Str("hi"), Value.fromJson(Json.parseToJsonElement("\"hi\"")))
        assertEquals(
            Value.ListV(listOf(Value.Num(1.0), Value.Num(2.0))),
            Value.fromJson(Json.parseToJsonElement("[1,2]")),
        )
        assertEquals(
            Value.Rec(linkedMapOf("a" to Value.Num(1.0))),
            Value.fromJson(Json.parseToJsonElement("""{"a":1}""")),
        )
    }

    @Test
    fun `toJson round-trips through fromJson`() {
        val v = Value.Rec(linkedMapOf("a" to Value.ListV(listOf(Value.Num(1.0), Value.Str("x"), Value.Null))))
        assertEquals(v, Value.fromJson(v.toJson()))
    }

    @Test
    fun `numeric equality is IEEE (NaN not equal to itself, -0 equal to 0)`() {
        assertFalse(Value.Num(Double.NaN) == Value.Num(Double.NaN))
        assertTrue(Value.Num(-0.0) == Value.Num(0.0))
    }

    @Test
    fun `record equality ignores key order`() {
        val a = Value.Rec(linkedMapOf("a" to Value.Num(1.0), "b" to Value.Num(2.0)))
        val b = Value.Rec(linkedMapOf("b" to Value.Num(2.0), "a" to Value.Num(1.0)))
        assertEquals(a, b)
    }

    @Test
    fun `list equality is order sensitive`() {
        val a = Value.ListV(listOf(Value.Num(1.0), Value.Num(2.0)))
        val b = Value.ListV(listOf(Value.Num(2.0), Value.Num(1.0)))
        assertFalse(a == b)
    }
}
