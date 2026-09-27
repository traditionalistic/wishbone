package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Value
import app.wishbone.core.bind
import androidx.compose.runtime.snapshots.Snapshot
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertTrue
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

    @Test
    fun renderContextHasValueEquality() {
        val s = store()
        val a = s.root.copy(locals = null.bind("item", Value.rec("id" to Value.Str("x"))), keyPath = ".x", index = 1)
        val b = s.root.copy(locals = null.bind("item", Value.rec("id" to Value.Str("x"))), keyPath = ".x", index = 1)
        assertEquals(a, b) // rebuilt, structurally equal → Compose can skip
        assertNotEquals(a, s.root.copy(locals = null.bind("item", Value.rec("id" to Value.Str("y"))), keyPath = ".x", index = 1))
        assertNotEquals(a, b.copy(keyPath = ".y"))
        assertEquals(s.root, s.root.withParent(Parent.None))
    }

    @Test
    fun envChangeInvalidatesOnlyItsReaders() {
        val s = Store(doc, emptyMap(), emptyMap(), theme, mapOf("colorScheme" to Value.Str("light"), "safeBottom" to Value.Num(0.0)), effects, CoroutineScope(Dispatchers.Unconfined))
        val reads = HashSet<Any>()
        Snapshot.observe(readObserver = { reads.add(it) }) { s.dark }
        val applied = HashSet<Any>()
        val handle = Snapshot.registerApplyObserver { changed, _ -> applied.addAll(changed) }
        try {
            s.env = mapOf("colorScheme" to Value.Str("light"), "safeBottom" to Value.Num(300.0))
            Snapshot.sendApplyNotifications()
            assertTrue(applied.intersect(reads).isEmpty()) // keyboard/inset changes don't touch colorScheme readers
            s.env = mapOf("colorScheme" to Value.Str("dark"), "safeBottom" to Value.Num(300.0))
            Snapshot.sendApplyNotifications()
            assertTrue(applied.intersect(reads).isNotEmpty())
            assertEquals(true, s.dark)
        } finally {
            handle.dispose()
        }
    }

    @Test
    fun dragReadersSubscribeUntilTheCellExists() {
        val s = store()
        val child = s.root.copy(keyPath = ".a.b") // a node inside drag instance "knob.a"
        val reads = HashSet<Any>()
        assertEquals(0.0, Snapshot.observe(readObserver = { reads.add(it) }) { child.driver("drag", "knob", "x") })
        val applied = HashSet<Any>()
        val handle = Snapshot.registerApplyObserver { changed, _ -> applied.addAll(changed) }
        try {
            val cell = s.driverCell("drag", "knob.a", "x") // created later, e.g. when the drag node composes
            Snapshot.sendApplyNotifications()
            assertTrue(applied.intersect(reads).isNotEmpty()) // the early reader is invalidated
            cell.doubleValue = 42.0
            assertEquals(42.0, child.driver("drag", "knob", "x")) // nearest key-path prefix
            assertEquals(0.0, s.root.copy(keyPath = ".z").driver("drag", "knob", "x"))
        } finally {
            handle.dispose()
        }
    }

    @Test
    fun driverCellsSurviveAStoreRebuild() {
        val s = store()
        val scroll = s.driverCell("scroll", "page", "y").apply { doubleValue = 120.0 }
        s.driverCell("drag", "card.k", "x").doubleValue = -60.0
        val rebuilt = store(previous = s)
        assertTrue(rebuilt.driverCell("scroll", "page", "y") === scroll) // effects keep writing the live cell
        assertEquals(120.0, rebuilt.driver("scroll", "page", "y"))
        assertEquals(-60.0, rebuilt.root.copy(keyPath = ".k").driver("drag", "card", "x"))
    }
}
