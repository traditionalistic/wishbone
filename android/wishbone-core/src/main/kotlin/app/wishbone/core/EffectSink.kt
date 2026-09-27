package app.wishbone.core

/**
 * Where the interpreter reports side effects. `call` is the typed backend-operation channel;
 * `perform` is every `do`-statement effect (navigate, back, present, dismiss, open, haptic, track,
 * host, wait, refresh, play, stop) — `wait`/`await play` suspend until the sink resumes the
 * coroutine. `error` reports an aborted handler run (unknown statement / eval error / budget
 * exceeded); state written before the error stays (the interpreter does not roll back).
 *
 * The interpreter always calls [call] synchronously, in statement order (so effect recording order
 * is deterministic regardless of `awaited`). When `awaited == false` the interpreter does not wait
 * for a meaningful result — **the sink itself** must not block: launch its own coroutine for the
 * real work and return [Value.Null] immediately. When `awaited == true` it suspends normally and
 * returns the real call-result record.
 */
interface EffectSink {
    suspend fun call(op: String, args: Value, awaited: Boolean): Value
    suspend fun perform(effect: String, args: List<Value>, await: Boolean)
    fun error(message: String)
}
