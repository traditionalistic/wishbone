package app.wishbone.ui

import androidx.compose.runtime.MutableDoubleState
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableDoubleStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.withFrameNanos
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import app.wishbone.core.Document
import app.wishbone.core.EffectSink
import app.wishbone.core.Expr
import app.wishbone.core.Node
import app.wishbone.core.Stmt
import app.wishbone.core.Value
import app.wishbone.core.toJson
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.math.floor

/** Shared 1 s clock behind `env.now`; ticked by every visible Wishbone root. */
internal object Clock {
    val now = mutableLongStateOf(System.currentTimeMillis())
}

/** `drag` on a node (spec §4.3): axis x|y|both, snap points and bounds along the axis. */
internal data class DragSpec(val axis: String, val snap: List<Double>, val bounds: Pair<Double, Double>?) {
    fun clamp(v: Double) = bounds?.let { v.coerceIn(it.first, it.second) } ?: v
}

/** Renderer-only node fields the core keeps verbatim in `Node.raw` (spec §3.8). */
internal data class NodeExtras(
    val inst: Map<String, Expr>? = null,
    val a11yLabel: Expr? = null,
    val a11yRole: String? = null,
    val a11yHidden: Expr? = null,
    val anim: Map<String, Expr>? = null,
    val trIn: List<TransitionPart>? = null,
    val trOut: List<TransitionPart>? = null,
    val match: Expr? = null,
    val drag: DragSpec? = null,
) {
    /** Rendered through AnimatedVisibility (enter/exit transitions, shared bounds). */
    val animatedPresence get() = trIn != null || trOut != null || match != null

    companion object {
        /** A node whose renderer fields fail to decode renders without them; the failure goes to [onError]. */
        fun index(root: Node, onError: (String) -> Unit = {}): Map<String, NodeExtras> {
            val out = HashMap<String, NodeExtras>()
            fun walk(n: Node) {
                runCatching {
                    val r = n.raw
                    val a = r["a11y"]?.jsonObject
                    val tr = r["tr"]?.jsonObject
                    val drag = r["drag"]?.jsonObject?.let { d ->
                        fun nums(k: String) = (d[k] as? JsonArray)?.mapNotNull { it.jsonPrimitive.doubleOrNull }
                        DragSpec(d["axis"]?.jsonPrimitive?.contentOrNull ?: "both", nums("snap") ?: emptyList(), nums("bounds")?.takeIf { it.size == 2 }?.let { it[0] to it[1] })
                    }
                    val ex = NodeExtras(
                        inst = r["inst"]?.jsonObject?.get("state")?.jsonObject?.mapValues { Expr.decode(it.value) },
                        a11yLabel = a?.get("label")?.let { Expr.decode(it) },
                        a11yRole = (a?.get("role") as? JsonPrimitive)?.contentOrNull,
                        a11yHidden = a?.get("hidden")?.let { h -> (h as? JsonPrimitive)?.booleanOrNull?.let { Expr.Lit(Value.of(it)) } ?: Expr.decode(h) },
                        anim = r["anim"]?.jsonObject?.mapValues { Expr.decode(it.value) },
                        trIn = tr?.let { TransitionPart.list(it["in"] as? JsonArray) },
                        trOut = tr?.let { TransitionPart.list(it["out"] as? JsonArray) },
                        match = r["match"]?.let { Expr.decode(it) },
                        drag = drag,
                    )
                    if (ex != NodeExtras()) out[n.id] = ex
                }.onFailure { onError("node ${n.id}: bad renderer fields: ${it.message ?: it}") }
                n.children.forEach(::walk)
            }
            walk(root)
            return out
        }
    }
}

private fun List<Stmt>.hasAnim(): Boolean = any {
    it is Stmt.Anim || (it is Stmt.If && (it.then.hasAnim() || it.otherwise.hasAnim()))
}

private fun Node.hasAnim(): Boolean = on.values.any { it.hasAnim() } || children.any { it.hasAnim() }

/** Template instance state: one cell per qualified name, resolved up the chain of enclosing instances. */
internal class InstanceScope(val parent: InstanceScope?, val cells: Map<String, MutableState<Value>>) {
    fun find(name: String): MutableState<Value>? = cells[name] ?: parent?.find(name)
}

/** A screen's DSL state: state values by key, template-instance state by instance key (node id + repeat key path). */
internal data class StoreSnapshot(val state: Map<String, Value>, val instances: Map<String, Map<String, Value>>) {
    /** JSON text for saveable state; lambdas have no JSON form and come back as null. */
    fun encode(): String = Value.rec("state" to Value.Rec(state), "instances" to Value.Rec(instances.mapValues { Value.Rec(it.value) })).toJson().toString()

    companion object {
        /** Null when [text] isn't a snapshot: the screen starts from its initial state. */
        fun decode(text: String): StoreSnapshot? = runCatching {
            val f = (Value.fromJsonString(text) as Value.Rec).fields
            StoreSnapshot((f.getValue("state") as Value.Rec).fields, (f.getValue("instances") as Value.Rec).fields.mapValues { (it.value as Value.Rec).fields })
        }.getOrNull()
    }
}

/**
 * One per rendered screen: document, params, data, one observable cell per state key, instance
 * scopes, driver cells, timeline registry. Expressions are evaluated on read inside composables, so
 * Compose tracks exactly the cells each node reads. Non-state effects go to [effects].
 */
class Store internal constructor(
    val doc: Document,
    val params: Map<String, Value>,
    data: Map<String, Value>,
    theme: Theme,
    env: Map<String, Value>,
    private val effects: EffectSink,
    internal val scope: CoroutineScope,
    val revision: Int = doc.revision,
    previous: Store? = null,
    /** Rendered by [WishboneSlot]: the root keeps its own background (a screen paints it behind the system bars). */
    internal val slot: Boolean = false,
    /** State saved when the screen left the composition (a host back stack), used when there is no [previous] store. */
    restored: StoreSnapshot? = null,
) : EffectSink {
    var theme by mutableStateOf(theme)

    /** One cell per env key, written only on change, so a node re-renders only for the keys it reads. */
    private val envCells = HashMap<String, MutableState<Value>>()
    var env: Map<String, Value>
        get() = envCells.mapValues { it.value.value }
        set(values) {
            for ((k, v) in values) {
                val cell = envCells[k]
                if (cell == null) envCells[k] = mutableStateOf(v) else if (cell.value != v) cell.value = v
            }
        }
    val data = mutableStateMapOf<String, Value>().apply { putAll(data) }

    internal val timelines: TimelineRegistry = TimelineRegistry(doc.timelines, System.nanoTime(), previous?.timelines)

    /** Frame-clock time (ns) the transform layer samples timelines at; advanced only while one is running. */
    internal val frameNanos = mutableLongStateOf(System.nanoTime())

    /** Motion of the `animate(m) { … }` block whose writes are being rendered; cleared two frames after the last write. */
    internal var transaction: MotionSpec? = null
        private set
    private var transactionId = 0

    /** Without `animate(…)` statements, nodes without `anim` skip the animated-prop layer. */
    internal val hasTransactions = doc.handlers.values.any { it.body.hasAnim() } || doc.root.hasAnim()

    private val cells = LinkedHashMap<String, MutableState<Value>>()
    internal val instances = HashMap<String, InstanceScope>()
    /** Same-name state keys and instance cells keep these values (a new document, or a restored screen). */
    private val kept: StoreSnapshot? = previous?.snapshot() ?: restored
    /** Scroll/drag cells survive a rebuild: effects keyed on remembered scroll states keep writing the same cells. */
    private val drivers: HashMap<String, MutableDoubleState> = previous?.drivers ?: HashMap()

    /** Bumped when a drag cell is created, so a reader that missed it (composed earlier) re-reads. */
    private val dragCellsVersion = mutableIntStateOf(0)
    internal val extras: Map<String, NodeExtras> = NodeExtras.index(doc.root) { effects.error(it) }
    internal val root = RenderContext(this, null, null, "", Parent.None)

    val dark: Boolean get() = env("colorScheme") == Value.Str("dark")

    init {
        this.env = env
        for ((name, expr) in doc.state) {
            cells[name] = mutableStateOf(kept?.state?.get(name) ?: root.evalOrNull(expr) ?: Value.Null)
        }
    }

    internal val reduceMotion: Boolean get() = env("reduceMotion") == Value.TRUE

    fun play(name: String) = timelines.play(name, System.nanoTime())

    internal fun recordTransaction(motion: Value) {
        transaction = theme.motionOf(motion) ?: return
        val id = ++transactionId
        scope.launch {
            try {
                withFrameNanos { }
                withFrameNanos { }
            } catch (_: IllegalStateException) {
                // no frame clock (unit tests)
            }
            if (transactionId == id) transaction = null
        }
    }

    fun state(name: String): Value = cells[name]?.value ?: Value.Null

    /** Current state and instance values; kept instances not rendered again yet (e.g. on another tab) stay in it. */
    internal fun snapshot(): StoreSnapshot = StoreSnapshot(
        cells.mapValues { it.value.value },
        kept?.instances.orEmpty() + instances.mapValues { (_, scope) -> scope.cells.mapValues { it.value.value } },
    )

    fun computed(name: String): Value = doc.computed[name]?.let { root.eval(it) } ?: Value.Null
    fun param(name: String): Value = params[name] ?: doc.params[name]?.let { root.eval(it) } ?: Value.Null
    fun env(name: String): Value = envCells[name]?.value ?: if (name == "now") Value.Num(Clock.now.longValue.toDouble()) else Value.Null

    fun token(group: String, name: String): Value = when (group) {
        "space" -> Value.Num(theme.space[name] ?: 0.0)
        "radius" -> Value.Num(theme.radius[name] ?: 0.0)
        "color", "type", "icon", "motion" -> Value.rec("\$$group" to Value.Str(name))
        else -> Value.Null
    }

    fun driver(kind: String, id: String, axis: String): Double = driverCell(kind, id, axis).doubleValue
    internal fun driverCell(kind: String, id: String, axis: String) = drivers.getOrPut("$kind.$id.$axis") {
        if (kind == "drag") dragCellsVersion.intValue++
        mutableDoubleStateOf(0.0)
    }

    /** An existing cell, or null after subscribing to drag-cell creation (so the caller re-reads once it exists). */
    internal fun existingDriver(kind: String, id: String, axis: String): MutableDoubleState? =
        drivers["$kind.$id.$axis"] ?: run {
            dragCellsVersion.intValue
            null
        }

    internal fun setState(name: String, path: List<Value>, value: Value) {
        val cell = cells[name] ?: return
        updatePath(cell.value, path, value)?.let { cell.value = it }
    }

    // ponytail: instances of removed repeat items stay in the map for the store's lifetime (a returning key gets its old state back)
    internal fun instance(key: String, decls: Map<String, Expr>, ctx: RenderContext): InstanceScope = instances.getOrPut(key) {
        val old = kept?.instances?.get(key)
        val cells = LinkedHashMap<String, MutableState<Value>>()
        val scope = InstanceScope(ctx.instance, cells)
        val init = ctx.copy(instance = scope)
        for ((n, e) in decls) cells[n] = mutableStateOf(old?.get(n) ?: init.evalOrNull(e) ?: Value.Null)
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
            "play" -> if (name != null) {
                play(name)
                if (await && !reduceMotion) {
                    val ms = timelines.duration(name)
                    if (ms.isInfinite()) awaitCancellation() else delay(ms.toLong())
                }
            }
            "stop" -> if (name != null) timelines.stop(name)
            "wait" -> delay(((args.firstOrNull() as? Value.Num)?.value ?: 0.0).toLong())
            else -> effects.perform(effect, args, await)
        }
    }

    override fun error(message: String) = effects.error(message)

    private val warned = HashSet<String>()

    /** Render-time problems are logged once per key, not on every recomposition. */
    internal fun warnOnce(key: String, message: String) {
        if (warned.add(key)) error(message)
    }
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
