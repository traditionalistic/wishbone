package app.wishbone.core

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject

/** Builds small hand-written wire documents/expressions for unit tests (see core-decisions.md: "unit-test with hand-written wire AST"). */
object TestHarness {

    /** A minimal document with a dummy empty `root`, from just the wire fragments a test needs. */
    fun document(
        state: String = "[]",
        computed: String = "{}",
        params: String = "{}",
        fns: String = "{}",
        handlers: String = "{}",
        data: String = "[]",
        timelines: String = "{}",
    ): Document {
        val json = """
            {"format":1,"runtimeMin":"1.0.0","screen":"test","revision":1,
             "params":$params,"state":$state,"computed":$computed,"fns":$fns,
             "handlers":$handlers,"data":$data,"timelines":$timelines,
             "root":{"t":"stack","id":"root"}}
        """.trimIndent()
        return Document.decode(Json.parseToJsonElement(json).jsonObject)
    }

    fun scopeAndInterpreter(
        document: Document = document(),
        data: Map<String, Value> = emptyMap(),
        params: Map<String, Value> = emptyMap(),
        env: Map<String, Value> = emptyMap(),
        stateOverrides: Map<String, Value> = emptyMap(),
        sink: EffectSink = RecordingEffectSink(),
    ): Pair<MapScope, Interpreter> = MapScope.create(
        document = document,
        data = data,
        params = params,
        env = DEFAULT_ENV + env,
        stateOverrides = stateOverrides,
        sink = sink,
    )

    /** Evaluates a bare wire expression (`["op", "+", ...]`) against an optional document/state/etc. */
    fun eval(
        exprWire: String,
        document: Document = document(),
        data: Map<String, Value> = emptyMap(),
        params: Map<String, Value> = emptyMap(),
        env: Map<String, Value> = emptyMap(),
        stateOverrides: Map<String, Value> = emptyMap(),
    ): Value {
        val (_, interpreter) = scopeAndInterpreter(document, data, params, env, stateOverrides)
        return interpreter.eval(Expr.decode(Json.parseToJsonElement(exprWire)), null)
    }
}
