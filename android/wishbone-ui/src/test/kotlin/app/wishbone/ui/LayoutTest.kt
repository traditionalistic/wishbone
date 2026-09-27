package app.wishbone.ui

import androidx.compose.ui.unit.Constraints
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
}
