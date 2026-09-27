package app.wishbone.core

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.double
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File

/** Walks up from the working directory to find the repo root (has both `contract/` and `android/`). Gradle's `Test` task working dir defaults to the module dir (`android/wishbone-core`), but this doesn't assume that. */
object RepoPaths {
    val root: File by lazy {
        var dir = File(".").absoluteFile
        while (!(File(dir, "contract").isDirectory && File(dir, "android").isDirectory)) {
            dir = dir.parentFile ?: throw IllegalStateException(
                "could not locate repo root (contract/ + android/) from ${File(".").absolutePath}",
            )
        }
        dir
    }

    val conformanceCompiledDir: File get() = File(root, "contract/conformance/compiled")
    val conformanceDir: File get() = File(root, "contract/conformance")
    val korpuTokensFile: File get() = File(root, "contract/tokens/korpu.json")
}

/** Just the `space`/`radius` numeric tables from contract/tokens/korpu.json — the only groups the interpreter itself resolves to numbers (color/type/icon/motion are symbolic, see [MapScope.token]). */
object KorpuTheme {
    private val json by lazy { Json.parseToJsonElement(RepoPaths.korpuTokensFile.readText()).jsonObject }
    val space: Map<String, Double> by lazy { json["space"]!!.jsonObject.mapValues { it.value.jsonPrimitive.double } }
    val radius: Map<String, Double> by lazy { json["radius"]!!.jsonObject.mapValues { it.value.jsonPrimitive.double } }
}

/** contract/README.md "Env по умолчанию". */
val DEFAULT_ENV: Map<String, Value> = mapOf(
    "now" to Value.Num(0.0),
    "platform" to Value.Str("test"),
    "locale" to Value.Str("ru"),
    "brand" to Value.Str("korpu"),
    "colorScheme" to Value.Str("light"),
    "reduceMotion" to Value.FALSE,
    "screenWidth" to Value.Num(390.0),
    "screenHeight" to Value.Num(844.0),
    "safeTop" to Value.Num(0.0),
    "safeBottom" to Value.Num(0.0),
    "timeZone" to Value.Str("UTC"),
)

/**
 * Test-double [EffectSink]. Recording shape follows contract/README.md harness rule 3:
 * `{"effect": name, "args": [...]}`; `call` records `{"effect": "call", "op": id, "args": <record>}`.
 * A `call` with no matching [callResults] entry answers `{ok: false, error: {code: NETWORK, message: "no mock"}}`.
 * `animate` is intentionally never recorded (its body just runs) — there is no "anim" case here because
 * [Interpreter] runs the `anim` body directly and never calls the sink for the `anim` statement itself.
 */
class RecordingEffectSink(private val callResults: Map<String, Value> = emptyMap()) : EffectSink {
    val effects = mutableListOf<Value>()
    val errors = mutableListOf<String>()

    override suspend fun call(op: String, args: Value, awaited: Boolean): Value {
        effects.add(Value.rec("effect" to Value.Str("call"), "op" to Value.Str(op), "args" to args))
        val result = callResults[op] ?: Value.rec(
            "ok" to Value.FALSE,
            "error" to Value.rec("code" to Value.Str("NETWORK"), "message" to Value.Str("no mock")),
        )
        return if (awaited) result else Value.Null
    }

    override suspend fun perform(effect: String, args: List<Value>, await: Boolean) {
        effects.add(Value.rec("effect" to Value.Str(effect), "args" to Value.ListV(args)))
    }

    override fun error(message: String) {
        errors.add(message)
    }
}
