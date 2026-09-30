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
}
