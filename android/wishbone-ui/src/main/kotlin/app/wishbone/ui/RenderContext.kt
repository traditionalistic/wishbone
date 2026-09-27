package app.wishbone.ui

import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.RowScope
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

/** A materialized child: the node, its context (repeat locals / key path) and a stable identity. */
internal class Item(val node: Node, val ctx: RenderContext, val key: String)

/**
 * Where a node is evaluated: repeat locals, enclosing template instance, repeat key path, layout
 * parent. Implements the core [Scope] over the [store].
 */
internal class RenderContext(
    val store: Store,
    val locals: Locals?,
    val instance: InstanceScope?,
    val keyPath: String,
    val parent: Parent,
) : Scope {
    private val interpreter = Interpreter(store.doc, this, store)

    fun copy(locals: Locals? = this.locals, instance: InstanceScope? = this.instance, keyPath: String = this.keyPath, parent: Parent = this.parent) =
        RenderContext(store, locals, instance, keyPath, parent)

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
        store.scope.launch(start = CoroutineStart.UNDISPATCHED) { interpreter.runStatements(stmts, locals.bind("event", payload)) }
    }

    fun identifier(node: Node) = "wb.${store.doc.screen}.${node.id}$keyPath"

    /** Expands `repeat` (identity = key value, else index) and drops `when`-false nodes. */
    fun materialize(nodes: List<Node>): List<Item> {
        val out = ArrayList<Item>()
        val seen = HashSet<String>()
        fun add(node: Node, c: RenderContext) {
            if (node.whenExpr != null && c.evalOrNull(node.whenExpr!!) != Value.TRUE) return
            var key = node.id + c.keyPath
            // ponytail: duplicate repeat keys get an index suffix instead of an error
            if (!seen.add(key)) {
                key += "#${out.size}"
                seen.add(key)
            }
            out += Item(node, c, key)
        }
        for (node in nodes) {
            val rep = node.rep
            if (rep == null) {
                add(node, this)
                continue
            }
            val items = (evalOrNull(rep.over) as? Value.ListV)?.items ?: continue
            items.forEachIndexed { i, item ->
                val c = copy(locals = locals.bind(rep.asName, item).bind(rep.indexName, Value.Num(i.toDouble())))
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
    override fun driver(kind: String, id: String, axis: String) = store.driver(kind, id, axis)
    override fun selfPressed() = false // phase b

    override fun set(scope: String, name: String, path: List<Value>, value: Value, transaction: Value?) {
        if (scope == "i") {
            val cell = instance?.find(name) ?: return
            updatePath(cell.value, path, value)?.let { cell.value = it }
        } else {
            store.setState(name, path, value)
        }
    }
}
