package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

/** Slots (spec 2026-09-28 §2): content shows when there is a store and its root's `when` holds. */
class SlotTest {
    private val theme = Theme.parse(File("../../contract/tokens/korpu.json").readText())
    private val effects = object : EffectSink {
        override suspend fun call(op: String, args: Value, awaited: Boolean): Value = Value.Null
        override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {}
        override fun error(message: String) {}
    }

    private fun store(json: String) = Store(Document.parse(json), emptyMap(), emptyMap(), theme, emptyMap(), effects, CoroutineScope(Dispatchers.Unconfined))

    @Test
    fun aSlotShowsWhenItsRootsWhenHolds() {
        assertFalse(slotVisible(null)) // nothing loaded (yet), unpublished or failed
        val s = store("""{"format":1,"screen":"slot.x","revision":1,"state":[["on",["v",true]]],"root":{"t":"stack","id":"root","when":["s","on"]}}""")
        assertTrue(slotVisible(s))
        s.root.set("s", "on", emptyList(), Value.FALSE, null)
        assertFalse(slotVisible(s)) // e.g. the raffle is over
        assertTrue(slotVisible(store("""{"format":1,"screen":"slot.x","revision":1,"root":{"t":"stack","id":"root"}}""")))
    }
}
