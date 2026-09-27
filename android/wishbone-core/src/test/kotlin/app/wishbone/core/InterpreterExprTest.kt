package app.wishbone.core

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Hand-written wire AST per area, transcribed from contract/conformance/expr.json — fast unit tests kept alongside the full real vectors in ConformanceTest. */
class InterpreterExprTest {
    private fun eval(exprWire: String, vararg env: Pair<String, Value>) = TestHarness.eval(exprWire, env = env.toMap())

    // --- arithmetic / comparisons -----------------------------------------------------------

    @Test
    fun `arithmetic precedence and associativity`() {
        assertEquals(Value.Num(7.0), eval("""["op","+",["v",1],["op","*",["v",2],["v",3]]]"""))
        assertEquals(Value.Num(-5.0), eval("""["op","-",["op","-",["v",2],["v",3]],["v",4]]"""))
    }

    @Test
    fun `division by zero and mod sign follows the dividend`() {
        assertEquals(Value.Num(Double.POSITIVE_INFINITY), eval("""["op","/",["v",1],["v",0]]"""))
        assertEquals(Value.Num(-1.0), eval("""["op","%",["un","-",["v",7]],["v",3]]"""))
    }

    @Test
    fun `short circuit && and coalesce`() {
        assertEquals(Value.FALSE, eval("""["op","&&",["v",false],["op",">",["op","/",["v",1],["v",0]],["v",0]]]"""))
        assertEquals(Value.Num(3.0), eval("""["op","??",["v",null],["op","??",["v",null],["v",3]]]"""))
    }

    @Test
    fun `string comparison is code point order`() {
        assertEquals(Value.TRUE, eval("""["op","<",["v","B"],["v","a"]]"""))
    }

    @Test
    fun `record and list equality`() {
        assertEquals(Value.TRUE, eval("""["op","==",["list",[["v",1],["v",2]]],["list",[["v",1],["v",2]]]]"""))
        assertEquals(Value.TRUE, eval("""["op","==",["rec",{"a":["v",1],"b":["v",2]}],["rec",{"b":["v",2],"a":["v",1]}]]"""))
        assertEquals(Value.FALSE, eval("""["op","==",["rec",{"a":["v",1]}],["rec",{"a":["v",2]}]]"""))
    }

    // --- strings -------------------------------------------------------------------------------

    @Test
    fun `string interpolation concatenates str()`() {
        assertEquals(Value.Str("n=3 f=2.5 b=true"), eval("""["str",[["v","n="],["v",3],["v"," f="],["v",2.5],["v"," b="],["v",true]]]"""))
    }

    @Test
    fun `length is a code point count, astral and combining aware`() {
        assertEquals(Value.Num(2.0), eval("""["f","length",[["v","😀a"]]]"""))
        assertEquals(Value.Num(2.0), eval("""["f","length",[["v","é"]]]"""))
    }

    @Test
    fun `substring is code point indexed and clamps`() {
        assertEquals(Value.Str("bc"), eval("""["f","substring",[["v","abc"],["v",1]]]"""))
        assertEquals(Value.Str("ab"), eval("""["f","substring",[["v","abc"],["v",-5],["v",2]]]"""))
        assertEquals(Value.Str(""), eval("""["f","substring",[["v","abc"],["v",2],["v",1]]]"""))
        assertEquals(Value.Str("a"), eval("""["f","substring",[["v","😀ab"],["v",1],["v",2]]]"""))
    }

    @Test
    fun `split keeps empty parts, matches is whole-string unicode-aware`() {
        assertEquals(Value.ListV(listOf(Value.Str("a"), Value.Str(""), Value.Str("b"))), eval("""["f","split",[["v","a,,b"],["v",","]]]"""))
        assertEquals(Value.TRUE, eval("""["f","matches",[["v","Привет"],["v","\\w+"]]]"""))
        assertEquals(Value.FALSE, eval("""["f","matches",[["v","abc"],["v","b"]]]"""))
    }

    @Test
    fun `num parses trimmed numbers and rejects garbage`() {
        assertEquals(Value.Num(42.5), eval("""["f","num",[["v"," 42.5 "]]]"""))
        assertEquals(Value.Num(-1.0), eval("""["op","??",["f","num",[["v","abc"]]],["v",-1]]"""))
        assertEquals(Value.Num(1000.0), eval("""["f","num",[["v","1e3"]]]"""))
    }

    // --- lists ---------------------------------------------------------------------------------

    @Test
    fun `map filter with index`() {
        assertEquals(
            Value.ListV(listOf(Value.Num(2.0), Value.Num(4.0), Value.Num(6.0))),
            eval("""["f","map",[["list",[["v",1],["v",2],["v",3]]],["lam",["x"],["op","*",["l","x"],["v",2]]]]]"""),
        )
        assertEquals(
            Value.ListV(listOf(Value.Str("a0"), Value.Str("b1"))),
            eval("""["f","map",[["list",[["v","a"],["v","b"]]],["lam",["x","i"],["op","+",["l","x"],["f","str",[["l","i"]]]]]]]"""),
        )
    }

    @Test
    fun `reduce sort chain and range`() {
        assertEquals(Value.Num(6.0), eval("""["f","reduce",[["list",[["v",1],["v",2],["v",3]]],["v",0],["lam",["acc","x"],["op","+",["l","acc"],["l","x"]]]]]"""))
        assertEquals(
            Value.ListV(listOf(Value.Num(9.0), Value.Num(3.0), Value.Num(4.0))),
            eval("""["f","set",[["f","removeAt",[["f","append",[["list",[["v",1],["v",2],["v",3]]],["v",4]]],["v",0]]],["v",0],["v",9]]]"""),
        )
        assertEquals(Value.ListV(listOf(Value.Num(0.0), Value.Num(1.0), Value.Num(2.0), Value.Num(3.0))), eval("""["f","range",[["v",4]]]"""))
        assertEquals(Value.ListV(emptyList()), eval("""["f","range",[["v",-1]]]"""))
    }

    @Test
    fun `sortBy is stable for numbers and code-point ordered for strings`() {
        assertEquals(
            Value.ListV(listOf(Value.Str("a"), Value.Str("c"), Value.Str("b"))),
            eval(
                """["f","map",[["f","sortBy",[["list",[
                        ["rec",{"n":["v","b"],"v":["v",2]}],
                        ["rec",{"n":["v","a"],"v":["v",1]}],
                        ["rec",{"n":["v","c"],"v":["v",1]}]
                    ]],["lam",["x"],["get",["l","x"],"v"]]]],["lam",["x"],["get",["l","x"],"n"]]]]""",
            ),
        )
        assertEquals(
            Value.ListV(listOf(Value.Str("B"), Value.Str("a"), Value.Str("b"))),
            eval("""["f","sortBy",[["list",[["v","b"],["v","a"],["v","B"]]],["lam",["x"],["l","x"]]]]"""),
        )
    }

    @Test
    fun `list indexing is null-safe on out-of-range and fractional index`() {
        assertEquals(Value.Num(2.0), eval("""["at",["list",[["v",1],["v",2],["v",3]]],["v",1]]"""))
        assertEquals(Value.Num(0.0), eval("""["op","??",["at",["list",[["v",1],["v",2],["v",3]]],["v",5]],["v",0]]"""))
        assertEquals(Value.Num(-1.0), eval("""["op","??",["at",["list",[["v",1],["v",2],["v",3]]],["v",1.5]],["v",-1]]"""))
    }

    // --- records / map<T> ------------------------------------------------------------------------

    @Test
    fun `record with() replaces only patched fields`() {
        assertEquals(
            Value.Rec(linkedMapOf("name" to Value.Str("Ann"), "age" to Value.Num(31.0))),
            eval("""["f","with",[["rec",{"name":["v","Ann"],"age":["v",30]}],["rec",{"age":["v",31]}]]]"""),
        )
    }

    @Test
    fun `map index by string key`() {
        assertEquals(Value.Num(1.0), eval("""["op","??",["at",["rec",{"a":["v",1]}],["v","a"]],["v",0]]"""))
        assertEquals(Value.Num(0.0), eval("""["op","??",["at",["rec",{"a":["v",1]}],["v","z"]],["v",0]]"""))
    }

    @Test
    fun `field access on null-typed optional is null-safe`() {
        assertEquals(Value.Str("none"), eval("""["op","??",["get",["v",null],"name"],["v","none"]]"""))
    }

    // --- math / tokens ---------------------------------------------------------------------------

    @Test
    fun `round is half away from zero`() {
        assertEquals(
            Value.ListV(listOf(Value.Num(3.0), Value.Num(-3.0), Value.Num(2.0), Value.Num(0.0))),
            eval("""["list",[["f","round",[["v",2.5]]],["f","round",[["v",-2.5]]],["f","round",[["v",2.4]]],["f","round",[["v",-0.4]]]]]"""),
        )
    }

    @Test
    fun `min and max return NaN if either argument is NaN`() {
        val nan = eval("""["f","min",[["op","/",["v",0],["v",0]],["v",1]]]""") as Value.Num
        assertTrue(nan.value.isNaN())
        val nan2 = eval("""["f","max",[["v",1],["op","/",["v",0],["v",0]]]]""") as Value.Num
        assertTrue(nan2.value.isNaN())
    }

    @Test
    fun `token space and radius resolve theme numbers`() {
        assertEquals(Value.Num(20.0), eval("""["op","+",["t","space","lg"],["t","space","xs"]]"""))
        assertEquals(Value.Num(9999.0), eval("""["t","radius","pill"]"""))
    }

    @Test
    fun `color and motion tokens are symbolic records`() {
        assertEquals(Value.rec("\$color" to Value.Str("el16")), eval("""["t","color","el16"]"""))
        assertEquals(Value.rec("\$motion" to Value.Str("snappy")), eval("""["t","motion","snappy"]"""))
    }

    @Test
    fun `alpha clamps and multiplies existing alpha`() {
        assertEquals(
            Value.rec("\$color" to Value.Str("bg1"), "alpha" to Value.Num(0.25)),
            eval("""["f","alpha",[["f","alpha",[["t","color","bg1"],["v",0.5]]],["v",0.5]]]"""),
        )
        assertEquals(Value.rec("\$color" to Value.Str("el16")), eval("""["f","alpha",[["t","color","el16"],["v",2]]]"""))
    }

    @Test
    fun `spring and ease build motion records`() {
        assertEquals(
            Value.rec("\$motion" to Value.rec("kind" to Value.Str("spring"), "response" to Value.Num(300.0), "damping" to Value.Num(0.8))),
            eval("""["f","spring",[["v",300],["v",0.8]]]"""),
        )
        assertEquals(
            Value.rec("\$motion" to Value.rec("kind" to Value.Str("curve"), "duration" to Value.Num(200.0), "bezier" to Value.ListV(listOf(Value.Num(0.42), Value.Num(0.0), Value.Num(0.58), Value.Num(1.0))))),
            eval("""["f","ease",[["v",200]]]"""),
        )
    }

    // --- env / declarations -----------------------------------------------------------------------

    @Test
    fun `env defaults and overrides`() {
        assertEquals(Value.Str("test"), eval("""["e","platform"]"""))
        assertEquals(Value.TRUE, eval("""["op","&&",["op","==",["e","colorScheme"],["v","dark"]],["e","reduceMotion"]]""", "colorScheme" to Value.Str("dark"), "reduceMotion" to Value.TRUE))
    }

    @Test
    fun `state order, let, fn declarations`() {
        val doc = TestHarness.document(
            state = """[["a",["v",1]],["b",["op","+",["s","a"],["v",2]]]]""",
        )
        assertEquals(Value.Num(3.0), TestHarness.eval("""["s","b"]""", doc))

        val doc2 = TestHarness.document(
            state = """[["a",["v",2]]]""",
            computed = """{"b":["op","*",["s","a"],["v",10]]}""",
            fns = """{"twice":{"params":["x"],"body":["op","*",["l","x"],["v",2]]}}""",
        )
        assertEquals(Value.Num(41.0), TestHarness.eval("""["op","+",["u","twice",[["c","b"]]],["v",1]]""", doc2))
    }

    @Test
    fun `param default vs override`() {
        val doc = TestHarness.document(params = """{"id":["v","x"]}""")
        assertEquals(Value.Str("id=x"), TestHarness.eval("""["op","+",["v","id="],["p","id"]]""", doc))
        assertEquals(Value.Str("id=42"), TestHarness.eval("""["op","+",["v","id="],["p","id"]]""", doc, params = mapOf("id" to Value.Str("42"))))
    }

    @Test
    fun `data present, missing, and required`() {
        val doc = TestHarness.document(data = """[{"name":"me","op":"demo.profile","args":{},"required":false}]""")
        assertEquals(Value.Str("Ann"), TestHarness.eval("""["op","??",["get",["d","me"],"name"],["v","?"]]""", doc, data = mapOf("me" to Value.rec("name" to Value.Str("Ann")))))
        assertEquals(Value.Str("?"), TestHarness.eval("""["op","??",["get",["d","me"],"name"],["v","?"]]""", doc))
    }

    @Test
    fun `optional method call is null-propagating (real compiler output has no null-check wrapper, just Stdlib short-circuiting)`() {
        // Confirmed against contract/conformance/compiled/expr.json "opt.method"/"opt.method.present":
        // the compiler lowers `xs.count ?? -1` straight to this wire form, no ["if", ...] wrapper.
        val doc = TestHarness.document(data = """[{"name":"xs","op":"demo.offers","args":{},"required":false}]""")
        val countCall = """["f","count",[["d","xs"]]]"""
        assertEquals(Value.Num(-1.0), TestHarness.eval("""["op","??",$countCall,["v",-1]]""", doc))
        assertEquals(Value.Num(0.0), TestHarness.eval("""["op","??",$countCall,["v",-1]]""", doc, data = mapOf("xs" to Value.ListV(emptyList()))))
    }
}
