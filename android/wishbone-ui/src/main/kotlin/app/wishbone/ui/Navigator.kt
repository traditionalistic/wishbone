package app.wishbone.ui

import androidx.compose.runtime.mutableStateListOf
import app.wishbone.core.Value

enum class Mode { Root, Push, Fade, Present }

data class Entry(val screen: String, val params: Map<String, Value>, val mode: Mode, val key: Long)

/**
 * The SDK-owned stack. Entries from the first [Mode.Present] on form the modal sheet (its own
 * inner stack); everything before it is the base stack.
 */
class Navigator(rootScreen: String, rootParams: Map<String, Value>) {
    private var nextKey = 0L
    val entries = mutableStateListOf(Entry(rootScreen, rootParams, Mode.Root, nextKey++))

    /** Leaves the SDK: `back`/`dismiss` at the root (the host's `onExit`, like iOS). */
    var exit: () -> Unit = {}

    private val sheetStart: Int get() = entries.indexOfFirst { it.mode == Mode.Present }.let { if (it < 0) entries.size else it }
    val base: List<Entry> get() = entries.subList(0, sheetStart).toList()
    val sheet: List<Entry> get() = entries.subList(sheetStart, entries.size).toList()

    fun navigate(screen: String, params: Map<String, Value>, transition: String?) {
        entries += Entry(screen, params, if (transition == "fade") Mode.Fade else Mode.Push, nextKey++)
    }

    // ponytail: present from inside a sheet pushes inside that sheet (one sheet level), nested sheets if ever needed
    fun present(screen: String, params: Map<String, Value>) {
        entries += Entry(screen, params, if (sheetStart < entries.size) Mode.Push else Mode.Present, nextKey++)
    }

    /** Pops the top entry (closing the sheet when it held one entry); at the root calls [exit] and returns false. */
    fun back(): Boolean {
        if (entries.size <= 1) {
            exit()
            return false
        }
        entries.removeAt(entries.lastIndex)
        return true
    }

    fun dismiss() {
        val start = sheetStart
        if (start < entries.size) entries.removeRange(start, entries.size) else back()
    }
}
