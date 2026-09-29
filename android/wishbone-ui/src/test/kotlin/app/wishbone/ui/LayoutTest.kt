package app.wishbone.ui

import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Constraints
import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.io.File

class LayoutTest {
    @Test
    fun nestedScrollAxisIsCappedOnlyWhenUnbounded() {
        val nested = Constraints(maxWidth = 1080, maxHeight = Constraints.Infinity) // inside a vertical scroll
        assertEquals(Constraints(maxWidth = 1080, maxHeight = 2400), capAxis(nested, horizontal = false, limit = 2400))
        assertEquals(nested, capAxis(nested, horizontal = true, limit = 1080)) // other axis untouched
        val bounded = Constraints(minHeight = 100, maxWidth = 1080, maxHeight = 900)
        assertEquals(bounded, capAxis(bounded, horizontal = false, limit = 2400))
        val row = Constraints(minWidth = 3000, maxWidth = Constraints.Infinity, maxHeight = 500)
        assertEquals(Constraints(minWidth = 1080, maxWidth = 1080, maxHeight = 500), capAxis(row, horizontal = true, limit = 1080))
    }

    @Test
    fun horizontalScrollTakesItsContentHeight() {
        // raffle.main's daily rail: an axis-h scroll in a v-stack card is as tall as its tiles (SwiftUI ScrollView),
        // not the card's leftover height — only its width is greedy, so no height modifier constrains the content
        val rail = BoxProps.from(emptyMap()).flexible("scroll", axis = "h")
        assertEquals(Dim.Fill, rail.width)
        assertEquals(Dim.Wrap, rail.height)
        assertEquals(Modifier.fillMaxWidth(), Modifier.dims(rail, Parent.None))
        // an axis-v scroll stays greedy both ways (iOS frames its content to the full width)
        assertEquals(Modifier.fillMaxWidth().fillMaxHeight(), Modifier.dims(BoxProps.from(emptyMap()).flexible("scroll", axis = null), Parent.None))
        // an explicit size still wins
        assertEquals(Dim.Fixed(80.0), BoxProps.from(mapOf("height" to Value.Num(80.0))).flexible("scroll", axis = "h").height)
    }

    @Test
    fun childKeysAreRelativeToTheirContainer() {
        val doc = Document.parse(
            """{"screen":"t","root":{"t":"stack","id":"root","c":[
              {"t":"stack","id":"row","rep":{"over":["v",[{"id":"a"},{"id":"b"}]]},"c":[
                {"t":"text","id":"title"},
                {"t":"text","id":"chip","rep":{"over":["v",[1,2]],"key":["l","item"]}}]}]}}""",
        )
        val effects = object : EffectSink {
            override suspend fun call(op: String, args: Value, awaited: Boolean): Value = Value.Null
            override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {}
            override fun error(message: String) {}
        }
        val theme = Theme.parse(File("../../contract/tokens/korpu.json").readText())
        val store = Store(doc, emptyMap(), emptyMap(), theme, emptyMap(), effects, CoroutineScope(Dispatchers.Unconfined))
        val rows = store.root.materialize(doc.root.children)
        assertEquals(listOf("row.0", "row.1"), rows.map { it.key })
        val first = rows[0].ctx.materialize(rows[0].node.children)
        val second = rows[1].ctx.materialize(rows[1].node.children)
        // a lazy row composed for index 1 can reuse the one composed for index 0: same inner keys
        assertEquals(listOf("title", "chip.1", "chip.2"), first.map { it.key })
        assertEquals(first.map { it.key }, second.map { it.key })
        // instance identity (a11y ids, instance state, drag cells) still uses the full path
        assertEquals(".1.2", second[2].ctx.keyPath)
        assertEquals(1, second[2].ctx.index)
    }
}
