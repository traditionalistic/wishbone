package app.wishbone.ui

import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.io.File

/**
 * Every compiled showcase screen (the playground fixtures `ios/Playground/Fixtures/compile.sh` builds from
 * contract/screens) decodes, and every materialized node's props evaluate against a real [Store] without errors.
 */
class ShowcaseEvalTest {
    private val theme = Theme.parse(File("../../contract/tokens/korpu.json").readText())
    private val env = mapOf(
        "now" to Value.Num(0.0), "platform" to Value.Str("android"), "locale" to Value.Str("ru"), "brand" to Value.Str("korpu"),
        "colorScheme" to Value.Str("light"), "reduceMotion" to Value.FALSE, "screenWidth" to Value.Num(390.0),
        "screenHeight" to Value.Num(844.0), "safeTop" to Value.Num(0.0), "safeBottom" to Value.Num(0.0), "timeZone" to Value.Str("UTC"),
    )

    @Test
    fun everyShowcaseNodeEvaluates() {
        val files = File("../../ios/Playground/Resources/Fixtures").listFiles { f -> f.name.startsWith("showcase.") }.orEmpty().sorted()
        assertEquals(4, files.size, "compiled showcases: $files")
        for (f in files) {
            val errors = mutableListOf<String>()
            val effects = object : EffectSink {
                override suspend fun call(op: String, args: Value, awaited: Boolean): Value = Value.Null
                override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {}
                override fun error(message: String) { errors += message }
            }
            val doc = Document.parse(Json.parseToJsonElement(f.readText()).jsonObject["document"].toString())
            val store = Store(doc, emptyMap(), emptyMap(), theme, env, effects, CoroutineScope(Dispatchers.Unconfined))
            var nodes = 0
            fun walk(items: List<Item>) {
                for (item in items.filter { it.visible }) {
                    val inst = store.extras[item.node.id]?.inst
                    val ctx = if (inst != null) item.ctx.withInstance(item.node.id + item.ctx.keyPath, inst) else item.ctx
                    item.node.props.values.forEach { ctx.evalOrNull(it) }
                    nodes++
                    walk(ctx.materialize(item.node.children))
                }
            }
            walk(store.root.materialize(listOf(doc.root)))
            assertTrue(nodes > 10, "${f.name}: only $nodes nodes")
            assertEquals(emptyList<String>(), errors, f.name)
        }
    }
}
