package app.wishbone.core

/**
 * The read/write surface the interpreter needs from its host (store). Locals (let, handler/lambda
 * params, repeat vars, `event`) are NOT part of Scope — they're threaded through `eval`/`run` as
 * an immutable [Locals] chain.
 *
 * `instanceState`/`set("i", ...)` names are template-qualified (e.g. `"Toggle.on"`); a real
 * implementation resolves them by walking up enclosing template instances until one declares the
 * name. `:wishbone-core` does not implement that walk (no template/instance materialization here,
 * see core-decisions.md) — [MapScope] (test-only) just uses a single flat map.
 */
interface Scope {
    fun state(name: String): Value
    fun instanceState(name: String): Value
    fun computed(name: String): Value
    fun data(name: String): Value
    fun param(name: String): Value
    fun env(name: String): Value
    fun token(group: String, name: String): Value
    fun driver(kind: String, id: String, axis: String): Double
    fun selfPressed(): Boolean

    /**
     * Path assignment: copy-on-write deep update. `path` elements are `Value.Str` (field name) or
     * `Value.Num` (list index). A missing field, an out-of-range index, or `null` anywhere on the
     * path makes the whole assignment a silent no-op. `transaction` is the motion [Value] of the
     * enclosing `animate(...)` block, if any (null outside one).
     */
    fun set(scope: String, name: String, path: List<Value>, value: Value, transaction: Value?)
}
