package app.wishbone.ui

import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.Constraints
import app.wishbone.core.Value
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

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
}
