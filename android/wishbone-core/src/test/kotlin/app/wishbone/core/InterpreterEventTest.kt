package app.wishbone.core

import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * Hand-written wire AST per area, transcribed from contract/conformance/events.json.
 * `:wishbone-core` does not materialize the node tree (core-decisions.md); `runStatements` is the
 * entry point, and the caller (here: the test) binds the event payload / repeat vars as locals —
 * on the wire `event` is just the local `["l","event"]` (contract/stdlib.json `language.event`).
 */
class InterpreterEventTest {

    @Test
    fun `bound textField writes event value into state`() = runTest {
        val doc = TestHarness.document(state = """[["email",["v",""]]]""")
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        val onChange = listOf(Stmt.decode(kotlinx.serialization.json.Json.parseToJsonElement(
            """["set","s","email",[],["get",["l","event"],"value"]]""",
        )))
        interp.runStatements(onChange, Locals("event", Value.rec("value" to Value.Str("a@b.c")), null))
        assertEquals(Value.Str("a@b.c"), scope.state("email"))
    }

    @Test
    fun `explicit change handler trims and counts edits`() = runTest {
        val doc = TestHarness.document(state = """[["name",["v",""]],["edits",["v",0]]]""")
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        val stmts = decodeStmts(
            """["set","s","name",[],["f","trim",[["get",["l","event"],"value"]]]]""",
            """["set","s","edits",[],["op","+",["s","edits"],["v",1]]]""",
        )
        interp.runStatements(stmts, Locals("event", Value.rec("value" to Value.Str("  Ann ")), null))
        assertEquals(Value.Str("Ann"), scope.state("name"))
        assertEquals(Value.Num(1.0), scope.state("edits"))
    }

    @Test
    fun `tap handler runs a named handler`() = runTest {
        val doc = TestHarness.document(
            state = """[["n",["v",0]]]""",
            handlers = """{"inc":{"params":["by"],"body":[["set","s","n",[],["op","+",["s","n"],["l","by"]]]]}}""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        interp.runStatements(decodeStmts("""["run","inc",[["v",3]]]"""), null)
        assertEquals(Value.Num(3.0), scope.state("n"))
    }

    @Test
    fun `dragEnd threshold on distance or velocity`() = runTest {
        val doc = TestHarness.document(state = """[["dismissed",["v",false]]]""")
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        val stmts = decodeStmts(
            """["if",["op","||",
                ["op",">",["f","abs",[["get",["l","event"],"x"]]],["v",120]],
                ["op",">",["f","abs",[["get",["l","event"],"vx"]]],["v",800]]],
                [["set","s","dismissed",[],["v",true]]],[]]""",
        )
        interp.runStatements(stmts, Locals("event", Value.rec("x" to Value.Num(150.0), "y" to Value.Num(0.0), "vx" to Value.Num(0.0), "vy" to Value.Num(0.0)), null))
        assertEquals(Value.TRUE, scope.state("dismissed"))
    }

    @Test
    fun `dragEnd below threshold does not dismiss`() = runTest {
        val doc = TestHarness.document(state = """[["dismissed",["v",false]]]""")
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        val stmts = decodeStmts(
            """["if",["op","||",
                ["op",">",["f","abs",[["get",["l","event"],"x"]]],["v",120]],
                ["op",">",["f","abs",[["get",["l","event"],"vx"]]],["v",800]]],
                [["set","s","dismissed",[],["v",true]]],[]]""",
        )
        interp.runStatements(stmts, Locals("event", Value.rec("x" to Value.Num(40.0), "y" to Value.Num(0.0), "vx" to Value.Num(100.0), "vy" to Value.Num(0.0)), null))
        assertEquals(Value.FALSE, scope.state("dismissed"))
    }

    @Test
    fun `tap inside repeat reads the bound repeat local`() = runTest {
        val doc = TestHarness.document(state = """[["picked",["v",""]]]""")
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        // repeat.over = ["basic","pro"], repeatIndex = 1 => local "p" = {"id":"pro"}
        val stmts = decodeStmts("""["set","s","picked",[],["get",["l","p"],"id"]]""")
        interp.runStatements(stmts, Locals("p", Value.rec("id" to Value.Str("pro")), null))
        assertEquals(Value.Str("pro"), scope.state("picked"))
    }

    private fun decodeStmts(vararg wire: String): List<Stmt> =
        wire.map { Stmt.decode(kotlinx.serialization.json.Json.parseToJsonElement(it)) }
}
