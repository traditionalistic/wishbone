package app.wishbone.ui

import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.RowScope
import androidx.compose.runtime.Stable
import androidx.compose.runtime.State
import app.wishbone.core.Expr
import app.wishbone.core.Format
import app.wishbone.core.Interpreter
import app.wishbone.core.Locals
import app.wishbone.core.Node
import app.wishbone.core.Scope
import app.wishbone.core.Stmt
import app.wishbone.core.Value
import app.wishbone.core.bind
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.launch

/** The layout a node is placed in — `fill` becomes `weight` along a Row/Column main axis. */
internal sealed interface Parent {
    data object None : Parent
    data class Column(val scope: ColumnScope) : Parent
    data class Row(val scope: RowScope) : Parent
    data class Box(val scope: BoxScope) : Parent

    /** Scroll content: unbounded along the axis, so `fill` there is intrinsic (like SwiftUI). */
    data class Scroll(val horizontal: Boolean) : Parent
}

/**
 * A materialized child: node, context (repeat locals / key path), stable identity. `visible` is false for a
 * `when`-false node that animates out (it stays in the list so AnimatedVisibility can run its exit).
 */
internal data class Item(val node: Node, val ctx: RenderContext, val key: String, val visible: Boolean = true)

/**
 * Where a node is evaluated: repeat locals, enclosing template instance, repeat key path, layout
 * parent. Implements the core [Scope] over the [store]. Value equality (store/instance/pressed by
 * identity, locals structurally) lets Compose skip a NodeView whose context was rebuilt unchanged.
 */
@Stable
internal class RenderContext(
    val store: Store,
    val locals: Locals?,
    val instance: InstanceScope?,
    val keyPath: String,
    val parent: Parent,
    /** Index of the innermost repeat that materialized this context (timeline stagger). */
    val index: Int = 0,
    /** Pressed state of the nearest enclosing tap target (`self.pressed`). */
    val pressed: State<Boolean>? = null,
) : Scope {
    private val interpreter by lazy(LazyThreadSafetyMode.NONE) { Interpreter(store.doc, this, store) }

    override fun equals(other: Any?): Boolean = other is RenderContext && other.store === store && other.instance === instance &&
        other.pressed === pressed && other.keyPath == keyPath && other.parent == parent && other.index == index && sameLocals(other.locals, locals)

    override fun hashCode(): Int = keyPath.hashCode() * 31 + index

    private fun sameLocals(a: Locals?, b: Locals?): Boolean {
        var x = a
        var y = b
        while (x != null && y != null) {
            if (x !== y && (x.name != y.name || x.value != y.value)) return false
            if (x === y) return true
            x = x.parent
            y = y.parent
        }
        return x == null && y == null
    }

    fun copy(
        locals: Locals? = this.locals,
        instance: InstanceScope? = this.instance,
        keyPath: String = this.keyPath,
        parent: Parent = this.parent,
        index: Int = this.index,
        pressed: State<Boolean>? = this.pressed,
    ) = RenderContext(store, locals, instance, keyPath, parent, index, pressed)

    fun withParent(p: Parent) = if (p == parent) this else copy(parent = p)
    fun withInstance(key: String, decls: Map<String, Expr>) = copy(instance = store.instance(key, decls, this))

    fun eval(e: Expr): Value = interpreter.eval(e, locals)

    /** Eval errors never crash: logged, and the caller renders nothing. */
    fun evalOrNull(e: Expr): Value? = try {
        eval(e)
    } catch (x: Exception) {
        store.error(x.message ?: x.toString())
        null
    }

    /** Runs an `on.*` body with `event` bound; starts synchronously so state writes land before the next frame. */
    fun fire(stmts: List<Stmt>, payload: Value = Value.rec()) {
        store.scope.launch(start = CoroutineStart.UNDISPATCHED) {
            store.runningHandlers++
            try {
                interpreter.runStatements(stmts, locals.bind("event", payload))
            } finally {
                store.runningHandlers--
            }
        }
    }

    fun identifier(node: Node) = "wb.${store.doc.screen}.${node.id}$keyPath"

    /** `when` absent or true (a slot decides on its root the same way). */
    fun shows(node: Node) = node.whenExpr?.let { evalOrNull(it) == Value.TRUE } != false

    /** Expands `repeat` (identity = key value, else index) and drops `when`-false nodes. */
    fun materialize(nodes: List<Node>): List<Item> {
        val out = ArrayList<Item>()
        val seen = HashSet<String>()
        fun add(node: Node, c: RenderContext) {
            val shown = c.shows(node)
            if (!shown && store.extras[node.id]?.animatedPresence != true) return
            var key = node.id + c.keyPath
            // ponytail: duplicate repeat keys get an index suffix instead of an error
            if (!seen.add(key)) {
                key += "#${out.size}"
                seen.add(key)
            }
            out += Item(node, c, key, shown)
        }
        for (node in nodes) {
            val rep = node.rep
            if (rep == null) {
                add(node, this)
                continue
            }
            val items = (evalOrNull(rep.over) as? Value.ListV)?.items ?: continue
            items.forEachIndexed { i, item ->
                val c = copy(locals = locals.bind(rep.asName, item).bind(rep.indexName, Value.Num(i.toDouble())), index = i)
                val k = rep.key?.let { c.evalOrNull(it) }?.let { Format.str(it) } ?: i.toString()
                add(node, c.copy(keyPath = "$keyPath.$k"))
            }
        }
        return out
    }

    override fun state(name: String) = store.state(name)
    override fun instanceState(name: String) = instance?.find(name)?.value ?: Value.Null
    override fun computed(name: String) = store.computed(name)
    override fun data(name: String) = store.data[name] ?: Value.Null
    override fun param(name: String) = store.param(name)
    override fun env(name: String) = store.env(name)
    override fun token(group: String, name: String) = store.token(group, name)
    /** Drag cells are per materialized instance: the nearest `id + keyPath` prefix that has one. */
    override fun driver(kind: String, id: String, axis: String): Double {
        if (kind != "drag") return store.driver(kind, id, axis)
        var path = keyPath
        while (true) {
            store.existingDriver(kind, id + path, axis)?.let { return it.doubleValue }
            if (path.isEmpty()) return 0.0
            path = path.substringBeforeLast('.', "")
        }
    }

    override fun selfPressed() = pressed?.value ?: false

    override fun set(scope: String, name: String, path: List<Value>, value: Value, transaction: Value?) {
        if (transaction != null) store.recordTransaction(transaction)
        if (scope == "i") {
            val cell = instance?.find(name) ?: return
            updatePath(cell.value, path, value)?.let { cell.value = it }
        } else {
            store.setState(name, path, value)
        }
    }
}
