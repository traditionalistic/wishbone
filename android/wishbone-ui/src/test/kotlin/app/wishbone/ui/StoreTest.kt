package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.InterpreterException
import app.wishbone.core.Value
import app.wishbone.core.bind
import androidx.compose.runtime.snapshots.Snapshot
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertSame
import org.junit.jupiter.api.Assertions.assertThrows
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

    private fun store(d: Document = doc, params: Map<String, Value> = emptyMap(), previous: Store? = null, restored: StoreSnapshot? = null) =
        Store(d, params, emptyMap(), theme, emptyMap(), effects, CoroutineScope(Dispatchers.Unconfined), previous = previous, restored = restored)

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
    fun computedIsEvaluatedOncePerStateChange() {
        val s = store(Document.parse("""{"screen":"t","state":[["a",["v",1]]],"computed":{"pair":["rec",{"a":["s","a"]}],"bad":["op","-",["v","x"],["v",1]]},"root":{"t":"stack","id":"root"}}"""))
        val first = s.computed("pair")
        assertSame(first, s.computed("pair")) // a record literal builds a new value per evaluation: same instance = not evaluated again
        s.root.set("s", "a", emptyList(), n(5.0), null)
        val second = s.computed("pair")
        assertEquals(Value.rec("a" to n(5.0)), second)
        assertSame(second, s.computed("pair"))
        s.root.set("s", "a", emptyList(), n(6.0), null) // a second write before any snapshot advance
        assertEquals(Value.rec("a" to n(6.0)), s.computed("pair"))
        assertEquals(Value.Null, s.computed("missing"))
        repeat(2) { assertThrows(InterpreterException::class.java) { s.computed("bad") } } // an error reaches every reader
    }

    @Test
    fun tokensReResolveAfterAThemeOrSchemeChange() {
        val s = Store(doc, emptyMap(), emptyMap(), theme, mapOf("colorScheme" to Value.Str("light")), effects, CoroutineScope(Dispatchers.Unconfined))
        val light = s.theme.color(s.token("color", "bg1"), s.dark)
        assertEquals(n(theme.space.getValue("md")), s.token("space", "md"))
        s.env = mapOf("colorScheme" to Value.Str("dark"))
        assertNotEquals(light, s.theme.color(s.token("color", "bg1"), s.dark))
        s.theme = Theme(theme.brand, theme.version, theme.colors, theme.type, theme.space + ("md" to 99.0), theme.radius, theme.motions)
        assertEquals(n(99.0), s.token("space", "md"))
    }

    @Test
    fun dataSourceWritesInvalidateOnlyTheirReaders() {
        val s = Store(doc, emptyMap(), mapOf("me" to n(1.0), "offers" to n(2.0)), theme, emptyMap(), effects, CoroutineScope(Dispatchers.Unconfined))
        val reads = HashSet<Any>()
        Snapshot.observe(readObserver = { reads.add(it) }) { s.root.data("me"); s.root.data("later") }
        val applied = HashSet<Any>()
        val handle = Snapshot.registerApplyObserver { changed, _ -> applied.addAll(changed) }
        try {
            s.putData(mapOf("me" to n(1.0), "offers" to n(3.0))) // `me` unchanged
            Snapshot.sendApplyNotifications()
            assertTrue(applied.intersect(reads).isEmpty())
            s.putData(mapOf("later" to n(4.0))) // a source first seen in a refresh reaches its early readers
            Snapshot.sendApplyNotifications()
            assertTrue(applied.intersect(reads).isNotEmpty())
            assertEquals(n(4.0), s.root.data("later"))
            assertEquals(n(3.0), s.root.data("offers"))
        } finally {
            handle.dispose()
        }
    }

    @Test
    fun transformsThatCanNotAnimateMoveToTheDrawPhase() {
        fun screen(handlers: String) = store(
            Document.parse(
                """{"screen":"t","state":[["x",["v",0]]],"handlers":{$handlers},"root":{"t":"stack","id":"root","c":[
                  {"t":"stack","id":"hero","p":{"opacity":["f","interpolate",[["dr","scroll","page","y"],["v",[0,160]],["v",[1,0]]]],"offsetY":["s","x"],"width":["dr","scroll","page","y"]}},
                  {"t":"stack","id":"anim","p":{"scale":["dr","scroll","page","y"]},"anim":{"scale":["t","motion","smooth"]}}]}}""",
            ),
        )
        // `animate(…)` writes state: the scroll-driven opacity can't take part, the state-driven offset can (stays composed)
        val animating = screen(""""go":{"params":[],"body":[["anim",["t","motion","snappy"],[["set","s","x",[],["v",1]]]]]}""")
        assertEquals(setOf("opacity"), animating.layerProps["hero"]?.keys) // width is layout: never
        assertEquals(null, animating.layerProps["anim"]) // an implicit animation needs the composed value
        assertEquals(setOf("opacity", "offsetY"), screen("").layerProps["hero"]?.keys) // no transactions: any transform
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
    fun snapshotRestoresScreenAndInstanceStateIntoANewStore() {
        val s = store()
        s.root.set("s", "a", emptyList(), n(5.0), null)
        s.root.set("s", "rec", listOf(Value.Str("l"), n(0.0)), Value.Str("x"), null)
        s.root.withInstance("tg", s.extras.getValue("tg").inst!!).set("i", "Toggle.on", emptyList(), Value.TRUE, null)
        // what rememberSaveable keeps while a host back stack has the screen off the composition
        val saved = s.snapshot().encode()
        val back = store(restored = StoreSnapshot.decode(saved))
        assertEquals(n(5.0), back.state("a"))
        assertEquals(s.state("rec"), back.state("rec"))
        // an instance not rendered again yet (e.g. on another tab) is still saved the next time
        assertEquals(Value.TRUE, back.snapshot().instances["tg"]?.get("Toggle.on"))
        assertEquals(Value.TRUE, back.root.withInstance("tg", back.extras.getValue("tg").inst!!).instanceState("Toggle.on"))
        assertEquals(null, StoreSnapshot.decode("not json"))
    }

    @Test
    fun aSnapshotOfAnotherScreenIsNotRestored() {
        val s = store()
        s.root.set("s", "a", emptyList(), n(5.0), null)
        val saved = StoreSnapshot.decode(s.snapshot().encode())
        assertEquals("t", saved?.screen)
        // a reused host key after process death: another screen with a same-name key starts from its own initial state
        val other = Document.parse("""{"screen":"other","state":[["a",["v",1]]],"root":{"t":"stack","id":"root"}}""")
        assertEquals(n(1.0), store(other, restored = saved).state("a"))
    }

    @Test
    fun nothingIsSavedWhileAHandlerRuns() {
        val gate = CompletableDeferred<Value>()
        val sink = object : EffectSink {
            override suspend fun call(op: String, args: Value, awaited: Boolean): Value = gate.await()
            override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {}
            override fun error(message: String) { errors += message }
        }
        val d = Document.parse(
            """{"screen":"t","state":[["loading",["v",false]]],
               "handlers":{"load":{"params":[],"body":[["set","s","loading",[],["v",true]],["call","x.load",["v",{}],"r",true],["set","s","loading",[],["v",false]]]}},
               "root":{"t":"stack","id":"root","on":{"appear":[["run","load",[]]]}}}""",
        )
        val s = Store(d, emptyMap(), emptyMap(), theme, emptyMap(), sink, CoroutineScope(Dispatchers.Unconfined))
        s.root.fire(d.root.on.getValue("appear"))
        assertEquals(Value.TRUE, s.state("loading"))
        // `loading = true` must not come back without the (cancelled) run that resets it
        assertEquals(null, s.savedSnapshot())
        gate.complete(Value.Null)
        assertEquals(Value.FALSE, s.savedSnapshot()?.state?.get("loading"))
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
