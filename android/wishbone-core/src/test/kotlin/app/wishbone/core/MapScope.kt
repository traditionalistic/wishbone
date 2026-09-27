package app.wishbone.core

import kotlin.math.floor

/**
 * Test-double [Scope] over plain maps + the korpu theme (contract/tokens/korpu.json), per
 * core-decisions.md. No template/instance materialization: `instanceState` is just a second flat
 * map, not resolved by walking enclosing instances (that's a renderer concern, out of scope here).
 */
class MapScope(
    private val document: Document,
    private val dataValues: Map<String, Value>,
    private val paramOverrides: Map<String, Value>,
    private val envValues: Map<String, Value>,
    private val drivers: Map<String, Double> = emptyMap(),
    private val pressed: Boolean = false,
) : Scope {
    /** Set right after construction — [computed]/[param]-default evaluation needs an [Interpreter]. */
    lateinit var interpreter: Interpreter

    val state: MutableMap<String, Value> = LinkedHashMap()
    val instanceState: MutableMap<String, Value> = LinkedHashMap()

    /** Every `set()` call in order, with the `transaction` (`animate(...)` motion) active at that point — for asserting `anim` scoping. */
    val setCalls: MutableList<Pair<String, Value?>> = mutableListOf()

    override fun state(name: String): Value = state[name] ?: Value.Null
    override fun instanceState(name: String): Value = instanceState[name] ?: Value.Null

    override fun computed(name: String): Value {
        val expr = document.computed[name] ?: return Value.Null
        return interpreter.eval(expr, null)
    }

    override fun data(name: String): Value = dataValues[name] ?: Value.Null

    override fun param(name: String): Value {
        paramOverrides[name]?.let { return it }
        val default = document.params[name] ?: return Value.Null
        return interpreter.eval(default, null)
    }

    override fun env(name: String): Value = envValues[name] ?: Value.Null

    override fun token(group: String, name: String): Value = when (group) {
        "space" -> Value.Num(KorpuTheme.space[name] ?: 0.0)
        "radius" -> Value.Num(KorpuTheme.radius[name] ?: 0.0)
        "color" -> Value.rec("\$color" to Value.Str(name))
        "type" -> Value.rec("\$type" to Value.Str(name))
        "icon" -> Value.rec("\$icon" to Value.Str(name))
        "motion" -> Value.rec("\$motion" to Value.Str(name))
        else -> Value.Null
    }

    override fun driver(kind: String, id: String, axis: String): Double = drivers["$kind.$id.$axis"] ?: 0.0
    override fun selfPressed(): Boolean = pressed

    override fun set(scope: String, name: String, path: List<Value>, value: Value, transaction: Value?) {
        setCalls.add(name to transaction)
        val target = if (scope == "i") instanceState else state
        val current = target[name] ?: Value.Null
        val updated = updatePath(current, path, value) ?: return
        target[name] = updated
    }

    /** Copy-on-write deep update; missing field / out-of-range index / null on the path => no-op (returns null). */
    private fun updatePath(current: Value, path: List<Value>, value: Value): Value? {
        if (path.isEmpty()) return value
        val head = path[0]
        val rest = path.subList(1, path.size)
        return when (head) {
            is Value.Str -> {
                val rec = current as? Value.Rec ?: return null
                if (!rec.fields.containsKey(head.value)) return null
                val updatedField = updatePath(rec.fields.getValue(head.value), rest, value) ?: return null
                Value.Rec(rec.fields + (head.value to updatedField))
            }
            is Value.Num -> {
                val list = current as? Value.ListV ?: return null
                val idx = head.value
                if (idx.isNaN() || idx != floor(idx) || idx < 0 || idx >= list.items.size) return null
                val i = idx.toInt()
                val updatedItem = updatePath(list.items[i], rest, value) ?: return null
                Value.ListV(list.items.toMutableList().also { it[i] = updatedItem })
            }
            else -> null
        }
    }

    companion object {
        /**
         * Builds a scope wired to a fresh [Interpreter] and initializes `state` per
         * contract/README.md harness rule 1: `document.state` in order (each initializer sees
         * `params`/`data`/`env` and previously-initialized state), then `stateOverrides` on top.
         */
        fun create(
            document: Document,
            data: Map<String, Value> = emptyMap(),
            params: Map<String, Value> = emptyMap(),
            env: Map<String, Value> = DEFAULT_ENV,
            stateOverrides: Map<String, Value> = emptyMap(),
            drivers: Map<String, Double> = emptyMap(),
            pressed: Boolean = false,
            sink: EffectSink = RecordingEffectSink(),
        ): Pair<MapScope, Interpreter> {
            val scope = MapScope(document, data, params, env, drivers, pressed)
            val interpreter = Interpreter(document, scope, sink)
            scope.interpreter = interpreter
            for ((name, expr) in document.state) {
                scope.state[name] = interpreter.eval(expr, null)
            }
            for ((k, v) in stateOverrides) scope.state[k] = v
            return scope to interpreter
        }
    }
}
