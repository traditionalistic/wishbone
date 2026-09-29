package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Stmt
import app.wishbone.core.Value
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.io.File

/**
 * contract/screens/raffle.main's leaderboard handlers (compiled: contract/conformance/compiled-screens, kept current by
 * the backend's `TestCompiledScreensGolden`), run against a [Store] whose `call`s the test answers by hand.
 */
class RaffleMainTest {
    private val theme = Theme.parse(File("../../contract/tokens/korpu.json").readText())
    private val doc = Document.parse(File("../../contract/conformance/compiled-screens/raffle.main.json").readText())

    /** Calls in the order made; each waits until the test completes it. */
    private val calls = mutableListOf<Pair<String, CompletableDeferred<Value>>>()
    private val performed = mutableListOf<Pair<String, List<Value>>>()
    private val errors = mutableListOf<String>()
    private val effects = object : EffectSink {
        override suspend fun call(op: String, args: Value, awaited: Boolean): Value =
            CompletableDeferred<Value>().also { calls += op to it }.await()

        override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {
            performed += effect to args
        }

        override fun error(message: String) { errors += message }
    }

    private fun row(position: Int) = Value.rec(
        "key" to Value.Str("u$position"), "position" to Value.Num(position.toDouble()), "name" to Value.Str("Имя $position"),
        "tickets" to Value.Num(1000.0 - position), "isMe" to Value.FALSE,
    )

    private fun page(from: Int) = Value.ListV((from until from + 20).map { row(it + 1) })
    private fun ok(value: Value) = Value.rec("ok" to Value.TRUE, "value" to value)

    private fun store() = Store(
        doc, emptyMap(),
        mapOf("o" to Value.rec("hidden" to Value.FALSE), "board" to Value.rec("items" to page(0), "total" to Value.Num(60.0))),
        theme, emptyMap(), effects, CoroutineScope(Dispatchers.Unconfined),
    )

    private fun Store.run(handler: String) = root.fire(listOf(Stmt.Run(handler, emptyList())))
    private fun Store.rows() = (state("rows") as Value.ListV).items.size

    /** Answers the oldest pending call of [op]. */
    private fun answer(op: String, result: Value) {
        val i = calls.indexOfFirst { it.first == op }
        check(i >= 0) { "no pending $op" }
        calls.removeAt(i).second.complete(result)
    }

    @Test
    fun hidingTheNameStartsTheLeaderboardOver() {
        val s = store()
        s.run("loadMore")
        answer("raffle.leaderboard", ok(Value.rec("items" to page(20))))
        assertEquals(20, s.rows())

        s.run("toggleHidden")
        answer("raffle.toggleHidden", ok(Value.rec("hidden" to Value.TRUE)))
        // page 2 has the old names: dropped, reloaded as the table scrolls; page 1 (`board`) is refreshed
        assertEquals(0, s.rows())
        assertEquals(Value.FALSE, s.state("boardDone"))
        assertEquals(Value.FALSE, s.state("moreFailed"))
        assertEquals(Value.FALSE, s.state("loadingMore"))
        assertEquals(listOf("refresh" to listOf<Value>(Value.Str("board"))), performed)

        s.run("loadMore")
        answer("raffle.leaderboard", ok(Value.rec("items" to page(20))))
        assertEquals(20, s.rows())
        assertEquals(emptyList<String>(), errors)
    }

    @Test
    fun aPageInFlightDuringTheToggleIsDropped() {
        val s = store()
        s.run("loadMore") // page 2, asked for under the old masking
        s.run("toggleHidden")
        answer("raffle.toggleHidden", ok(Value.rec("hidden" to Value.TRUE)))
        assertEquals(Value.FALSE, s.state("loadingMore"))
        s.run("loadMore") // the table asks again at once, not held up by the stale request
        answer("raffle.leaderboard", ok(Value.rec("items" to page(20)))) // the stale page 2
        assertEquals(0, s.rows())
        answer("raffle.leaderboard", ok(Value.rec("items" to page(20)))) // page 2 under the new masking
        assertEquals(20, s.rows())
        assertEquals(Value.FALSE, s.state("loadingMore"))
        assertEquals(emptyList<String>(), errors)
    }
}
