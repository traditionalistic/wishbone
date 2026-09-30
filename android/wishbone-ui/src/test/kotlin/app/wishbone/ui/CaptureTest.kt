package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancel
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.io.File

class CaptureTest {
    private val theme = Theme.parse(File("../../contract/tokens/korpu.json").readText())

    private val doc = Document.parse(
        """
        {"screen":"t","revision":1,"state":[["step",["v",0]]],
         "handlers":{
           "go":{"params":[],"body":[
             ["set","s","step",[],["v",1]],
             ["do","navigate",[["v","other"],["v",{}],["v","push"]],false],
             ["do","back",[],false],
             ["do","present",[["v","sheet"],["v",{}]],false],
             ["do","dismiss",[],false],
             ["do","open",[["v","https://example.com"]],false],
             ["do","haptic",[["v","light"]],false],
             ["do","track",[["v","tapped"],["v",{}]],false],
             ["do","host",[["v","share"],["v",{}]],true],
             ["set","s","step",[],["v",2]],
             ["call","x.load",["v",{}],"r",true],
             ["set","s","step",[],["v",3]]]},
           "reload":{"params":[],"body":[["do","refresh",[["v","me"]],true],["set","s","step",[],["v",9]]]}},
         "root":{"t":"stack","id":"root","on":{"appear":[["run","go",[]]]}}}
        """,
    )

    @Test
    fun captureModeKeepsStateAndSwallowsEverythingOutward() {
        val errors = mutableListOf<String>()
        val scope = CoroutineScope(Dispatchers.Unconfined)
        val s = Store(doc, emptyMap(), emptyMap(), theme, emptyMap(), CaptureEffects { errors += it }, scope)
        s.root.fire(doc.root.on.getValue("appear"))
        // ran past every outward effect as a no-op, then parked on the call
        assertEquals(Value.Num(2.0), s.state("step"))
        assertEquals(1, s.runningHandlers)
        assertEquals(emptyList<String>(), errors)
        s.root.fire(doc.handlers.getValue("reload").body)
        assertEquals(2, s.runningHandlers)
        // the layer leaves the composition: the parked runs end without running on
        scope.cancel()
        assertEquals(0, s.runningHandlers)
        assertEquals(Value.Num(2.0), s.state("step"))
    }

    @Test
    fun settleWaitsTheFloorThenTwoQuietPollsAndGivesUpAtTheDeadline() {
        assertEquals(Settle.Wait(0), settle(0, 0, false))
        assertEquals(Settle.Wait(0), settle(699, 0, false, quietPolls = 5)) // quiet inside the floor does not count
        assertEquals(Settle.Wait(1), settle(700, 0, false))
        assertEquals(Settle.Capture(settled = true), settle(750, 0, false, quietPolls = 1))
        assertEquals(Settle.Wait(0), settle(750, 1, false, quietPolls = 1)) // an image in flight resets the streak…
        assertEquals(Settle.Wait(0), settle(750, 0, true, quietPolls = 1)) // …so does a finite timeline
        assertEquals(Settle.Wait(0), settle(5999, 1, true))
        assertEquals(Settle.Capture(settled = false), settle(6000, 1, true))
        assertEquals(Settle.Capture(settled = false), settle(6000, 0, false, quietPolls = 1))
    }

    /** The poll (50 ms apart, the first at the floor) that captured, and whether it was settled. */
    private fun captureAt(busy: List<Boolean>): Pair<Int, Boolean>? {
        var quiet = 0
        for ((i, b) in busy.withIndex()) {
            when (val s = settle(SETTLE_FLOOR_MS + i * SETTLE_POLL_MS, if (b) 1 else 0, false, quiet)) {
                is Settle.Wait -> quiet = s.quietPolls
                is Settle.Capture -> return i to s.settled
            }
        }
        return null
    }

    @Test
    fun aBusyPollRestartsTheQuietStreak() {
        assertEquals(1 to true, captureAt(listOf(false, false, false)))
        assertEquals(3 to true, captureAt(listOf(false, true, false, false, false)))
        assertEquals(null, captureAt(listOf(true, false, true)))
        assertEquals(106 to false, captureAt(List(200) { true })) // 700 + 106 × 50 = 6000
    }
}
