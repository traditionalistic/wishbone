package app.wishbone.core

import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Hand-written wire AST per area, transcribed from contract/conformance/handlers.json. */
class InterpreterHandlerTest {

    @Test
    fun `assigns state from an argument`() = runTest {
        val doc = TestHarness.document(
            state = """[["n",["v",0]]]""",
            handlers = """{"inc":{"params":["x"],"body":[["set","s","n",[],["op","+",["s","n"],["l","x"]]]]}}""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        interp.run("inc", listOf(Value.Num(2.0)))
        assertEquals(Value.Num(2.0), scope.state("n"))
    }

    @Test
    fun `if-else branch`() = runTest {
        val doc = TestHarness.document(
            state = """[["step",["v",0]],["done",["v",false]]]""",
            handlers = """{"next":{"params":[],"body":[
                ["if",["op","<",["s","step"],["v",2]],
                    [["set","s","step",[],["op","+",["s","step"],["v",1]]]],
                    [["set","s","done",[],["v",true]]]]
            ]}}""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc, stateOverrides = mapOf("step" to Value.Num(2.0)))
        interp.run("next", emptyList())
        assertEquals(Value.Num(2.0), scope.state("step"))
        assertEquals(Value.TRUE, scope.state("done"))
    }

    @Test
    fun `path assignment into a list of records`() = runTest {
        val doc = TestHarness.document(
            state = """[["items",["list",[["rec",{"title":["v","a"],"done":["v",false]}],["rec",{"title":["v","b"],"done":["v",false]}]]]]]""",
            handlers = """{"toggle":{"params":["i"],"body":[
                ["set","s","items",[["l","i"],["v","done"]],["un","!",["op","??",["at",["at",["s","items"],["l","i"]],["v","done"]],["v",false]]]]
            ]}}""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        interp.run("toggle", listOf(Value.Num(1.0)))
        val items = (scope.state("items") as Value.ListV).items
        assertEquals(Value.FALSE, (items[0] as Value.Rec).fields["done"])
        assertEquals(Value.TRUE, (items[1] as Value.Rec).fields["done"])
    }

    @Test
    fun `out-of-range path assignment is a no-op`() = runTest {
        val doc = TestHarness.document(
            state = """[["items",["list",[["rec",{"done":["v",false]}]]]]]""",
            handlers = """{"go":{"params":[],"body":[["set","s","items",[["v",5],["v","done"]],["v",true]]]}}""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        interp.run("go", emptyList())
        assertEquals(Value.ListV(listOf(Value.Rec(linkedMapOf("done" to Value.FALSE)))), scope.state("items"))
    }

    @Test
    fun `do-effects are recorded with their evaluated positional args, animate is not recorded`() = runTest {
        val doc = TestHarness.document(
            handlers = """{"go":{"params":[],"body":[
                ["let","id",["op","+",["v","x"],["f","str",[["v",1]]]]],
                ["do","navigate",[["l","id"],["v",true]],false],
                ["anim",["t","motion","snappy"],[["let","unused",["v",1]]]]
            ]}}""",
        )
        val sink = RecordingEffectSink()
        val (_, interp) = TestHarness.scopeAndInterpreter(doc, sink = sink)
        interp.run("go", emptyList())
        assertEquals(1, sink.effects.size)
        assertEquals(Value.Str("navigate"), (sink.effects[0] as Value.Rec).fields["effect"])
        assertEquals(Value.ListV(listOf(Value.Str("x1"), Value.TRUE)), (sink.effects[0] as Value.Rec).fields["args"])
    }

    @Test
    fun `await call binds the result record, ok and error branches`() = runTest {
        val doc = TestHarness.document(
            state = """[["discount",["v",0]],["msg",["v",""]]]""",
            handlers = """{"check":{"params":[],"body":[
                ["call","demo.checkPromo",["rec",{"code":["v","WB10"]}],"r",true],
                ["if",["get",["l","r"],"ok"],
                    [["set","s","discount",[],["op","??",["get",["get",["l","r"],"value"],"discount"],["v",0]]]],
                    [["set","s","msg",[],["op","??",["get",["get",["l","r"],"error"],"message"],["v","error"]]]]]
            ]}}""",
        )
        val sink = RecordingEffectSink(
            callResults = mapOf(
                "demo.checkPromo" to Value.rec(
                    "ok" to Value.TRUE,
                    "value" to Value.rec("discount" to Value.Num(10.0), "message" to Value.Str("ok")),
                ),
            ),
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc, sink = sink)
        interp.run("check", emptyList())
        assertEquals(Value.Num(10.0), scope.state("discount"))
        assertEquals(1, sink.effects.size)
    }

    @Test
    fun `fire-and-forget call is still recorded`() = runTest {
        val doc = TestHarness.document(
            handlers = """{"go":{"params":[],"body":[["call","demo.claimOffer",["rec",{"id":["v","o1"]}],null,false]]}}""",
        )
        val sink = RecordingEffectSink(callResults = mapOf("demo.claimOffer" to Value.rec("ok" to Value.TRUE)))
        val (_, interp) = TestHarness.scopeAndInterpreter(doc, sink = sink)
        interp.run("go", emptyList())
        assertEquals(1, sink.effects.size)
        assertEquals(Value.Str("demo.claimOffer"), (sink.effects[0] as Value.Rec).fields["op"])
    }

    @Test
    fun `run invokes another handler with its own params, state changes are visible to the caller`() = runTest {
        val doc = TestHarness.document(
            state = """[["log",["v",""]]]""",
            handlers = """{
                "a":{"params":[],"body":[
                    ["set","s","log",[],["op","+",["s","log"],["v","a"]]],
                    ["run","b",[]],
                    ["set","s","log",[],["op","+",["s","log"],["v","c"]]]
                ]},
                "b":{"params":[],"body":[["set","s","log",[],["op","+",["s","log"],["v","b"]]]]}
            }""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        interp.run("a", emptyList())
        assertEquals(Value.Str("abc"), scope.state("log"))
    }

    @Test
    fun `computed re-evaluates against fresh state on every read`() = runTest {
        val doc = TestHarness.document(
            state = """[["a",["v",1]],["seen",["v",0]]]""",
            computed = """{"b":["op","*",["s","a"],["v",2]]}""",
            handlers = """{"go":{"params":[],"body":[
                ["set","s","a",[],["v",5]],
                ["set","s","seen",[],["c","b"]]
            ]}}""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        interp.run("go", emptyList())
        assertEquals(Value.Num(5.0), scope.state("a"))
        assertEquals(Value.Num(10.0), scope.state("seen"))
    }

    @Test
    fun `locals never alias state (copy-on-write path assignment)`() = runTest {
        val doc = TestHarness.document(
            state = """[["items",["list",[["rec",{"done":["v",false]}]]]],["snap",["v",true]]]""",
            handlers = """{"go":{"params":[],"body":[
                ["let","before",["s","items"]],
                ["set","s","items",[["v",0],["v","done"]],["v",true]],
                ["set","s","snap",[],["op","??",["at",["at",["l","before"],["v",0]],["v","done"]],["v",true]]]
            ]}}""",
        )
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc)
        interp.run("go", emptyList())
        assertEquals(Value.FALSE, scope.state("snap"))
        assertEquals(Value.TRUE, ((scope.state("items") as Value.ListV).items[0] as Value.Rec).fields["done"])
    }

    @Test
    fun `unknown statement aborts the run and reports an error, prior state stays`() = runTest {
        val doc = TestHarness.document(
            state = """[["n",["v",0]]]""",
            handlers = """{"go":{"params":[],"body":[
                ["set","s","n",[],["v",1]],
                ["bogus"]
            ]}}""",
        )
        val sink = RecordingEffectSink()
        val (scope, interp) = TestHarness.scopeAndInterpreter(doc, sink = sink)
        interp.run("go", emptyList())
        assertEquals(Value.Num(1.0), scope.state("n"))
        assertTrue(sink.errors.isNotEmpty())
    }
}
