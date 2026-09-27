package app.wishbone.core

/**
 * Where the interpreter reports side effects. `call` is the typed backend-operation channel;
 * `perform` is every `do`-statement effect (navigate, back, present, dismiss, open, haptic, track,
 * host, wait, refresh, play, stop) — `wait`/`await play` suspend until the sink resumes the
 * coroutine. `error` reports an aborted handler run (unknown statement / eval error / budget
 * exceeded); state written before the error stays (the interpreter does not roll back).
 */
interface EffectSink {
    suspend fun call(op: String, args: Value): Value
    suspend fun perform(effect: String, args: List<Value>, await: Boolean)
    fun error(message: String)
}
