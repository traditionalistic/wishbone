package app.wishbone.ui

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class NavigatorTest {
    private fun nav() = Navigator("root", emptyMap())
    private fun Navigator.ids() = entries.map { it.screen }

    @Test
    fun pushFadeAndBack() {
        val n = nav()
        n.navigate("a", emptyMap(), null)
        n.navigate("b", emptyMap(), "fade")
        assertEquals(listOf("root", "a", "b"), n.ids())
        assertEquals(Mode.Fade, n.entries.last().mode)
        assertEquals(Mode.Push, n.entries[1].mode)
        assertTrue(n.back())
        assertTrue(n.back())
        assertFalse(n.back())
        assertEquals(listOf("root"), n.ids())
    }

    @Test
    fun keysAreUniqueAndIncreasing() {
        val n = nav()
        n.navigate("a", emptyMap(), null)
        n.back()
        n.navigate("a", emptyMap(), null)
        assertTrue(n.entries[1].key > n.entries[0].key)
        assertEquals(2, n.entries.map { it.key }.toSet().size)
    }

    @Test
    fun presentOpensSheetAndNavigateStaysInside() {
        val n = nav()
        n.navigate("a", emptyMap(), null)
        n.present("s", emptyMap())
        n.navigate("s2", emptyMap(), null)
        assertEquals(listOf("root", "a"), n.base.map { it.screen })
        assertEquals(listOf("s", "s2"), n.sheet.map { it.screen })
        n.back()
        assertEquals(listOf("s"), n.sheet.map { it.screen })
        n.back() // last sheet entry: closes the sheet
        assertTrue(n.sheet.isEmpty())
        assertEquals(listOf("root", "a"), n.ids())
    }

    @Test
    fun dismissClosesWholeSheetElsePops() {
        val n = nav()
        n.present("s", emptyMap())
        n.navigate("s2", emptyMap(), null)
        n.dismiss()
        assertEquals(listOf("root"), n.ids())
        n.navigate("a", emptyMap(), null)
        n.dismiss()
        assertEquals(listOf("root"), n.ids())
        n.dismiss() // root stays
        assertEquals(listOf("root"), n.ids())
    }

    @Test
    fun backAndDismissAtRootExit() {
        val n = nav()
        var exits = 0
        n.exit = { exits++ }
        n.navigate("a", emptyMap(), null)
        n.back()
        assertEquals(0, exits)
        assertFalse(n.back())
        n.dismiss()
        assertEquals(2, exits)
        assertEquals(listOf("root"), n.ids())
    }
}
