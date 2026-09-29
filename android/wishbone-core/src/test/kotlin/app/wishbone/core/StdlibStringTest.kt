package app.wishbone.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/** String functions on ASCII, Cyrillic and surrogate pairs (code-point semantics, not UTF-16 units). */
class StdlibStringTest {
    private fun call(fn: String, vararg args: String) =
        TestHarness.eval("""["f","$fn",[${args.joinToString(",")}]]""")

    private fun s(v: String) = """["v","$v"]"""
    private fun n(v: Int) = """["v",$v]"""

    @Test
    fun `trim on ascii cyrillic emoji and unicode whitespace`() {
        assertEquals(Value.Str("ab c"), call("trim", s("  ab c\\t\\n")))
        assertEquals(Value.Str("привет"), call("trim", s("\\u00a0 привет\\u3000\\u2003")))
        assertEquals(Value.Str("😀 a 😀"), call("trim", s(" 😀 a 😀 ")))
        assertEquals(Value.Str("😀"), call("trim", s("\\u2028😀\\u2029")))
        assertEquals(Value.Str(""), call("trim", s("  \\u00a0 ")))
        assertEquals(Value.Str(""), call("trim", s("")))
    }

    @Test
    fun `substring by code points`() {
        assertEquals(Value.Str("ривет"), call("substring", s("привет"), n(1)))
        assertEquals(Value.Str("a😀"), call("substring", s("😀a😀b"), n(1), n(3)))
        assertEquals(Value.Str("😀"), call("substring", s("a😀b"), n(1), n(2)))
        assertEquals(Value.Str("bc"), call("substring", s("abc"), n(1)))
        assertEquals(Value.Str("ab"), call("substring", s("abc"), n(-5), n(2)))
        assertEquals(Value.Str(""), call("substring", s("😀😀"), n(2), n(1)))
        assertEquals(Value.Str("😀😀"), call("substring", s("😀😀"), n(0), n(99)))
    }

    @Test
    fun `upper and lower`() {
        assertEquals(Value.Str("ПРИВЕТ ABC"), call("upper", s("привет abc")))
        assertEquals(Value.Str("привет abc"), call("lower", s("ПРИВЕТ ABC")))
        assertEquals(Value.Str("STRASSE 😀"), call("upper", s("straße 😀")))
        assertEquals(Value.Str("i̇"), call("lower", s("İ")))
        assertEquals(Value.Str("σ😀σ"), call("lower", s("Σ😀Σ")))
        assertEquals(Value.Str("HELLO, 1!"), call("upper", s("Hello, 1!")))
        assertEquals(Value.Str("𐐨"), call("lower", s("𐐀")))
    }

    @Test
    fun `num trims unicode whitespace`() {
        assertEquals(Value.Num(12.5), call("num", s("\\u00a0 12.5\\u2003")))
        assertEquals(Value.Null, call("num", s("😀")))
        assertEquals(Value.Num(-3.0), call("num", s(" -3 ")))
        assertEquals(Value.Null, call("num", s("1e999")))
    }

    @Test
    fun `eval error carries no stack trace and caps the embedded value text`() {
        val long = "x".repeat(500)
        val e = org.junit.jupiter.api.assertThrows<InterpreterException.EvalError> {
            TestHarness.eval("""["f","abs",[["v","$long"]]]""")
        }
        assertEquals(0, e.stackTrace.size)
        assertEquals(true, e.message!!.length < 260)
    }
}
