package app.wishbone.core

import kotlinx.serialization.json.Json
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class AstDecodeTest {

    @Test
    fun `unknown expression tag decodes to Unknown, not a decode failure`() {
        val e = Expr.decode(Json.parseToJsonElement("""["nope","x"]"""))
        assertEquals(Expr.Unknown("nope"), e)
    }

    @Test
    fun `unknown statement tag decodes to Unknown`() {
        val s = Stmt.decode(Json.parseToJsonElement("""["nope"]"""))
        assertEquals(Stmt.Unknown("nope"), s)
    }

    @Test
    fun `malformed known-tag shape throws (missing required element)`() {
        assertThrows(AstDecodeException::class.java) {
            Expr.decode(Json.parseToJsonElement("""["op","+",["v",1]]""")) // missing the second operand
        }
    }

    @Test
    fun `list and rec literal constant folding both decode`() {
        assertEquals(
            Expr.ListLit(listOf(Expr.Lit(Value.Num(1.0)), Expr.Lit(Value.Num(2.0)))),
            Expr.decode(Json.parseToJsonElement("""["list",[["v",1],["v",2]]]""")),
        )
        assertEquals(
            Expr.Lit(Value.ListV(listOf(Value.Num(1.0), Value.Num(2.0)))),
            Expr.decode(Json.parseToJsonElement("""["v",[1,2]]""")),
        )
    }

    @Test
    fun `document decode round-trips a minimal document`() {
        val doc = TestHarness.document(state = """[["n",["v",0]]]""")
        assertEquals("test", doc.screen)
        assertEquals(1, doc.state.size)
        assertEquals("stack", doc.root.type)
        assertEquals("root", doc.root.id)
    }

    @Test
    fun `data arg decodes the dollar-param marker distinctly from a literal`() {
        // data args "may reference only literals and params" (contract/stdlib.json language.data):
        // a literal is the raw JSON value, not an Expr-wrapped ["v", ...] form.
        val doc = TestHarness.document(data = """[{"name":"me","op":"demo.profile","args":{"id":{"${'$'}param":"id"},"n":1},"required":true}]""")
        val decl = doc.data.single()
        assertEquals(DataArg.Param("id"), decl.args["id"])
        assertEquals(DataArg.Lit(Value.Num(1.0)), decl.args["n"])
    }
}
