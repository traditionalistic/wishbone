package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.io.File

class StoreTest {
    private val theme = Theme.parse(File("../../contract/tokens/korpu.json").readText())
    private val errors = mutableListOf<String>()
    private val effects = object : EffectSink {
        override suspend fun call(op: String, args: Value, awaited: Boolean): Value = Value.Null
        override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {}
        override fun error(message: String) { errors += message }
    }

    private val doc = Document.parse(
        """
        {"format":1,"screen":"t","revision":1,
         "params":{"who":["v","anon"]},
         "state":[["a",["v",1]],["b",["op","+",["s","a"],["v",1]]],["rec",["v",{"x":{"y":1},"l":[1,2]}]]],
         "computed":{"twice":["op","*",["s","a"],["v",2]]},
         "handlers":{"inc":{"params":[],"body":[["set","s","a",[],["op","+",["s","a"],["v",1]]]]}},
         "root":{"t":"stack","id":"root","c":[
           {"t":"text","id":"btn","p":{"text":["v","+"]},"on":{"tap":[["run","inc",[]]]}},
           {"t":"stack","id":"tg","inst":{"state":{"Toggle.on":["v",false]}},"c":[
             {"t":"shape","id":"tg/dot","inst":{"state":{"Dot.hover":["i","Toggle.on"]}}}]}]}}
        """,
    )

    private fun store(d: Document = doc, params: Map<String, Value> = emptyMap(), previous: Store? = null) =
        Store(d, params, emptyMap(), theme, emptyMap(), effects, CoroutineScope(Dispatchers.Unconfined), previous = previous)

    private fun n(v: Double) = Value.Num(v)

    @Test
    fun stateInitializersRunInOrderAndComputedOnRead() {
        val s = store()
        assertEquals(n(1.0), s.state("a"))
        assertEquals(n(2.0), s.state("b"))
        assertEquals(n(2.0), s.computed("twice"))
        s.root.set("s", "a", emptyList(), n(5.0), null)
        assertEquals(n(10.0), s.computed("twice"))
    }

    @Test
    fun paramsDefaultAndOverride() {
        assertEquals(Value.Str("anon"), store().param("who"))
        assertEquals(Value.Str("me"), store(params = mapOf("who" to Value.Str("me"))).param("who"))
        assertEquals(Value.Null, store().param("missing"))
    }

    @Test
    fun pathAssignmentIsCopyOnWriteAndNoOpWhenMissing() {
        val s = store()
        val before = s.state("rec")
        s.root.set("s", "rec", listOf(Value.Str("x"), Value.Str("y")), n(5.0), null)
        assertEquals(n(5.0), ((s.state("rec") as Value.Rec).fields["x"] as Value.Rec).fields["y"])
        assertEquals(n(1.0), ((before as Value.Rec).fields["x"] as Value.Rec).fields["y"])
        s.root.set("s", "rec", listOf(Value.Str("l"), n(1.0)), n(9.0), null)
        assertEquals(Value.ListV(listOf(n(1.0), n(9.0))), (s.state("rec") as Value.Rec).fields["l"])
        val snapshot = s.state("rec")
        s.root.set("s", "rec", listOf(Value.Str("zzz")), n(1.0), null)
        s.root.set("s", "rec", listOf(Value.Str("l"), n(7.0)), n(1.0), null)
        assertEquals(snapshot, s.state("rec"))
    }

    @Test
    fun reloadKeepsValuesOfStateKeysThatStillExist() {
        val old = store()
        old.root.set("s", "a", emptyList(), n(5.0), null)
        val newDoc = Document.parse(
            """{"screen":"t","state":[["a",["v",100]],["c",["op","+",["s","a"],["v",1]]]],"root":{"t":"stack","id":"root"}}""",
        )
        val fresh = store(newDoc, previous = old)
        assertEquals(n(5.0), fresh.state("a"))
        assertEquals(n(6.0), fresh.state("c"))
        assertEquals(Value.Null, fresh.state("b"))
    }

    @Test
    fun instanceStateWalksUpTheChain() {
        val s = store()
        val outer = s.root.withInstance("tg", s.extras.getValue("tg").inst!!)
        val inner = outer.withInstance("tg/dot", s.extras.getValue("tg/dot").inst!!)
        assertEquals(Value.FALSE, inner.instanceState("Toggle.on"))
        assertEquals(Value.FALSE, inner.instanceState("Dot.hover"))
        inner.set("i", "Toggle.on", emptyList(), Value.TRUE, null)
        assertEquals(Value.TRUE, outer.instanceState("Toggle.on"))
        assertEquals(Value.Null, outer.instanceState("Dot.hover"))
        // same identity -> same cells
        assertEquals(Value.TRUE, s.root.withInstance("tg", s.extras.getValue("tg").inst!!).instanceState("Toggle.on"))
        // survives a document reload for the same identity
        val reloaded = store(previous = s)
        assertEquals(Value.TRUE, reloaded.root.withInstance("tg", reloaded.extras.getValue("tg").inst!!).instanceState("Toggle.on"))
    }

    @Test
    fun tapRunsHandlerAndErrorsAreReported() {
        val s = store()
        s.root.fire(s.doc.root.children[0].on.getValue("tap"), Value.rec())
        assertEquals(n(2.0), s.state("a"))
        s.root.fire(listOf(app.wishbone.core.Stmt.Unknown("zap")), Value.rec())
        assertEquals(1, errors.size)
    }
}
