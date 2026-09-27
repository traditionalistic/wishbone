package app.wishbone.ui

import androidx.compose.runtime.MutableDoubleState
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableDoubleStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Expr
import app.wishbone.core.Node
import app.wishbone.core.Value
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlin.math.floor

/** Shared 1 s clock behind `env.now`; ticked by every visible Wishbone root. */
internal object Clock {
    val now = mutableLongStateOf(System.currentTimeMillis())
}

/** Renderer-only node fields the core keeps verbatim in `Node.raw` (spec §3.8). */
internal data class NodeExtras(val inst: Map<String, Expr>?, val a11yLabel: Expr?, val a11yRole: String?, val a11yHidden: Expr?) {
    companion object {
        fun index(root: Node): Map<String, NodeExtras> {
            val out = HashMap<String, NodeExtras>()
            fun walk(n: Node) {
                runCatching {
                    val inst = n.raw["inst"]?.jsonObject?.get("state")?.jsonObject?.mapValues { Expr.decode(it.value) }
                    val a = n.raw["a11y"]?.jsonObject
                    val hidden = a?.get("hidden")?.let { h -> (h as? JsonPrimitive)?.booleanOrNull?.let { Expr.Lit(Value.of(it)) } ?: Expr.decode(h) }
                    if (inst != null || a != null) out[n.id] = NodeExtras(inst, a?.get("label")?.let { Expr.decode(it) }, (a?.get("role") as? JsonPrimitive)?.contentOrNull, hidden)
                }
                n.children.forEach(::walk)
            }
            walk(root)
            return out
        }
    }
}

/** Template instance state: one cell per qualified name, resolved up the chain of enclosing instances. */
internal class InstanceScope(val parent: InstanceScope?, val cells: Map<String, MutableState<Value>>) {
    fun find(name: String): MutableState<Value>? = cells[name] ?: parent?.find(name)
}

/**
 * One per rendered screen: document, params, data, one observable cell per state key, instance
 * scopes, driver cells, timeline registry. Expressions are evaluated on read inside composables, so
 * Compose tracks exactly the cells each node reads. Non-state effects go to [effects].
 */
class Store(
    val doc: Document,
    val params: Map<String, Value>,
    data: Map<String, Value>,
    theme: Theme,
    env: Map<String, Value>,
    private val effects: EffectSink,
    internal val scope: CoroutineScope,
    val revision: Int = doc.revision,
    previous: Store? = null,
) : EffectSink {
    var theme by mutableStateOf(theme)
    var env by mutableStateOf(env)
    val data = mutableStateMapOf<String, Value>().apply { putAll(data) }

    /** Active timelines: name → start time (ms). Rendered by the motion layer (phase b). */
    val timelines = mutableStateMapOf<String, Long>()

    private val cells = LinkedHashMap<String, MutableState<Value>>()
    internal val instances = HashMap<String, InstanceScope>()
    private val carried = previous?.instances
    private val drivers = HashMap<String, MutableDoubleState>()
    internal val extras: Map<String, NodeExtras> = NodeExtras.index(doc.root)
    internal val root = RenderContext(this, null, null, "", Parent.None)

    val dark: Boolean get() = env["colorScheme"] == Value.Str("dark")

    init {
        for ((name, expr) in doc.state) {
            cells[name] = mutableStateOf(previous?.cells?.get(name)?.value ?: root.evalOrNull(expr) ?: Value.Null)
        }
        doc.timelines.filterValues { it.autoplay }.keys.forEach { timelines[it] = System.currentTimeMillis() }
    }

    fun state(name: String): Value = cells[name]?.value ?: Value.Null
    fun computed(name: String): Value = doc.computed[name]?.let { root.eval(it) } ?: Value.Null
    fun param(name: String): Value = params[name] ?: doc.params[name]?.let { root.eval(it) } ?: Value.Null
    fun env(name: String): Value = env[name] ?: if (name == "now") Value.Num(Clock.now.longValue.toDouble()) else Value.Null

    fun token(group: String, name: String): Value = when (group) {
        "space" -> Value.Num(theme.space[name] ?: 0.0)
        "radius" -> Value.Num(theme.radius[name] ?: 0.0)
        "color", "type", "icon", "motion" -> Value.rec("\$$group" to Value.Str(name))
        else -> Value.Null
    }

    fun driver(kind: String, id: String, axis: String): Double = driverCell(kind, id, axis).doubleValue
    internal fun driverCell(kind: String, id: String, axis: String) = drivers.getOrPut("$kind.$id.$axis") { mutableDoubleStateOf(0.0) }

    internal fun setState(name: String, path: List<Value>, value: Value) {
        val cell = cells[name] ?: return
        updatePath(cell.value, path, value)?.let { cell.value = it }
    }

    // ponytail: instances of removed repeat items stay in the map for the store's lifetime (a returning key gets its old state back)
    internal fun instance(key: String, decls: Map<String, Expr>, ctx: RenderContext): InstanceScope = instances.getOrPut(key) {
        val old = carried?.get(key)
        val cells = LinkedHashMap<String, MutableState<Value>>()
        val scope = InstanceScope(ctx.instance, cells)
        val init = ctx.copy(instance = scope)
        for ((n, e) in decls) cells[n] = mutableStateOf(old?.cells?.get(n)?.value ?: init.evalOrNull(e) ?: Value.Null)
        scope
    }

    /** Fire-and-forget calls (`awaited == false`) run in the screen scope; the handler continues with null. */
    override suspend fun call(op: String, args: Value, awaited: Boolean): Value {
        if (awaited) return effects.call(op, args, true)
        scope.launch { effects.call(op, args, true) }
        return Value.Null
    }

    override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {
        val name = (args.firstOrNull() as? Value.Str)?.value
        when (effect) {
            // ponytail: `await play` resumes immediately until the motion layer (phase b) knows durations
            "play" -> if (name != null) timelines[name] = System.currentTimeMillis()
            "stop" -> if (name != null) timelines.remove(name)
            "wait" -> delay(((args.firstOrNull() as? Value.Num)?.value ?: 0.0).toLong())
            else -> effects.perform(effect, args, await)
        }
    }

    override fun error(message: String) = effects.error(message)
}

/** Copy-on-write deep update; a missing field / out-of-range index / null on the path → null (no-op). */
internal fun updatePath(current: Value, path: List<Value>, value: Value): Value? {
    if (path.isEmpty()) return value
    val rest = path.subList(1, path.size)
    return when (val head = path[0]) {
        is Value.Str -> {
            val rec = current as? Value.Rec ?: return null
            val field = rec.fields[head.value] ?: return null
            Value.Rec(rec.fields + (head.value to (updatePath(field, rest, value) ?: return null)))
        }
        is Value.Num -> {
            val list = current as? Value.ListV ?: return null
            val i = head.value
            if (i.isNaN() || i != floor(i) || i < 0 || i >= list.items.size) return null
            val updated = updatePath(list.items[i.toInt()], rest, value) ?: return null
            Value.ListV(list.items.toMutableList().also { it[i.toInt()] = updated })
        }
        else -> null
    }
}
