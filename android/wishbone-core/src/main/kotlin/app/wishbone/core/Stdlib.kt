package app.wishbone.core

import java.util.Locale
import java.util.regex.Pattern
import java.util.regex.PatternSyntaxException
import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.cos
import kotlin.math.floor
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

/** What [Stdlib] needs back from the interpreter: apply a lambda value, and read env for format functions. */
interface StdlibContext {
    val scope: Scope
    /** Calls [lambda] with one argument; a lambda declaring no params ignores it. */
    fun apply(lambda: Value.Lambda, a: Value): Value
    /** Calls [lambda] with two arguments; params beyond the lambda's arity are dropped. */
    fun apply(lambda: Value.Lambda, a: Value, b: Value): Value
}

private fun isWhitespace(cp: Int): Boolean = when {
    cp in 0x09..0x0D || cp == 0x20 -> true
    cp < 0x85 -> false
    else -> cp == 0x85 || cp == 0xA0 || cp == 0x1680 || cp in 0x2000..0x200A ||
        cp == 0x2028 || cp == 0x2029 || cp == 0x202F || cp == 0x205F || cp == 0x3000
}

private val NUM_PATTERN: Pattern = Pattern.compile("^[+-]?(\\d+\\.?\\d*|\\.\\d+)([eE][+-]?\\d+)?$")

/** `matches(s, re)` compiles a lot of the same small regexes repeatedly (e.g. inside a `map`); cache them. */
private val patternCache = java.util.concurrent.ConcurrentHashMap<String, Pattern>()

/**
 * Compiles [re] with [flags], falling back to compiling with no flags at all if the runtime
 * rejects [flags] outright. Android's (ICU-based) `Pattern.compile` throws `IllegalArgumentException`
 * for `Pattern.UNICODE_CHARACTER_CLASS` — desktop Java supports it, so every JVM unit test passed
 * while this threw on-device for every `matches()` call. ICU's `\d \w \s` are already
 * Unicode-aware by default, so the flag isn't needed there anyway.
 *
 * `PatternSyntaxException` (`re` itself is bad) is NOT treated as a flag problem — it IS-A
 * `IllegalArgumentException`, so it must be caught first and rethrown, or a genuinely bad regex
 * would silently retry (and still fail, just with a less clear stack) instead of surfacing as-is.
 *
 * `flags` is a parameter (rather than hardcoding `Pattern.UNICODE_CHARACTER_CLASS` inside) so a
 * unit test can force the fallback branch on any JVM by passing a flag bit no platform accepts.
 */
internal fun compileWithFallback(re: String, flags: Int): Pattern =
    try {
        Pattern.compile(re, flags)
    } catch (e: PatternSyntaxException) {
        throw e
    } catch (e: IllegalArgumentException) {
        Pattern.compile(re)
    }

private fun compiledPattern(re: String): Pattern = patternCache.getOrPut(re) {
    try {
        compileWithFallback(re, Pattern.UNICODE_CHARACTER_CLASS)
    } catch (e: PatternSyntaxException) {
        throw InterpreterException.EvalError("invalid regex '$re': ${e.message}")
    }
}

/**
 * Every `"method": true` name in contract/stdlib.json. The compiler lowers `x.m(...)` for an
 * optional receiver `x: T?` straight to `["f", "m", [x, ...]]` — the same wire shape a guaranteed
 * non-null direct call `m(x, ...)` produces — so the interpreter can't tell them apart from the
 * call site. Per the "Optional receivers" note the *result* must still be null when the receiver
 * is null, so every method-flagged function short-circuits on a null first argument here, in one
 * place, rather than re-implementing the check in each function below.
 */
private val NULL_PROPAGATING_METHODS = setOf(
    "length", "isEmpty", "upper", "lower", "trim", "contains", "startsWith", "endsWith",
    "split", "replace", "matches", "substring", "count", "first", "last", "map", "filter",
    "find", "some", "every", "reduce", "sum", "join", "append", "removeAt", "set", "slice",
    "indexOf", "sortBy", "reverse", "with",
)

/** Every function in contract/stdlib.json `functions`, dispatched by name on already-evaluated args. */
object Stdlib {

    fun call(ctx: StdlibContext, name: String, args: List<Value>): Value {
        if (name in NULL_PROPAGATING_METHODS && args.isNotEmpty() && args[0] == Value.Null) return Value.Null
        return dispatch(ctx, name, args)
    }

    private fun dispatch(ctx: StdlibContext, name: String, args: List<Value>): Value = when (name) {
        "min" -> Value.Num(min(num(args[0]), num(args[1])))
        "max" -> Value.Num(kotlin.math.max(num(args[0]), num(args[1])))
        "abs" -> Value.Num(abs(num(args[0])))
        "round" -> Value.Num(Format.roundHalfAwayFromZero(num(args[0])))
        "floor" -> Value.Num(floor(num(args[0])))
        "ceil" -> Value.Num(ceil(num(args[0])))
        "sqrt" -> Value.Num(sqrt(num(args[0])))
        "pow" -> Value.Num(num(args[0]).pow(num(args[1])))
        "sin" -> Value.Num(sin(num(args[0])))
        "cos" -> Value.Num(cos(num(args[0])))
        "clamp" -> Value.Num(min(kotlin.math.max(num(args[0]), num(args[1])), num(args[2]))) // literally min(max(x,lo),hi) — no bound swapping
        "interpolate" -> Value.Num(interpolate(num(args[0]), list(args[1]), list(args[2])))

        "length" -> Value.Num(str(args[0]).codePointCount(0, str(args[0]).length).toDouble())
        "isEmpty" -> when (val a = args[0]) {
            is Value.Str -> Value.of(a.value.isEmpty())
            is Value.ListV -> Value.of(a.items.isEmpty())
            else -> throw typeError("isEmpty", a)
        }
        "upper" -> Value.Str(mapCase(str(args[0]), toUpper = true))
        "lower" -> Value.Str(mapCase(str(args[0]), toUpper = false))
        "trim" -> Value.Str(trimWhitespace(str(args[0])))
        "contains" -> when (val a = args[0]) {
            is Value.Str -> Value.of(str(args[1]).let { a.value.contains(it) })
            is Value.ListV -> Value.of(a.items.any { deepEquals(it, args[1]) })
            else -> throw typeError("contains", a)
        }
        "startsWith" -> Value.of(str(args[0]).startsWith(str(args[1])))
        "endsWith" -> Value.of(str(args[0]).endsWith(str(args[1])))
        "split" -> {
            val s = str(args[0]); val sep = str(args[1])
            val parts = if (sep.isEmpty()) {
                codePoints(s).map { String(Character.toChars(it)) }
            } else {
                s.split(sep)
            }
            Value.ListV(parts.map { Value.Str(it) })
        }
        "replace" -> {
            val a = str(args[1])
            if (a.isEmpty()) args[0] else Value.Str(str(args[0]).replace(a, str(args[2])))
        }
        "matches" -> Value.of(compiledPattern(str(args[1])).matcher(str(args[0])).matches())
        "substring" -> {
            val s = str(args[0])
            val n = s.codePointCount(0, s.length)
            val start = clampIndex(num(args[1]), n)
            val end = if (args.size > 2 && args[2] != Value.Null) clampIndex(num(args[2]), n) else n
            if (start >= end) {
                Value.Str("")
            } else {
                val from = s.offsetByCodePoints(0, start)
                Value.Str(s.substring(from, s.offsetByCodePoints(from, end - start)))
            }
        }
        "num" -> {
            val s = trimWhitespace(str(args[0]))
            if (NUM_PATTERN.matcher(s).matches()) {
                val d = s.toDoubleOrNull()
                if (d != null && d.isFinite()) Value.Num(d) else Value.Null
            } else {
                Value.Null
            }
        }
        "str" -> Value.Str(Format.str(args[0]))

        "count" -> Value.Num(list(args[0]).size.toDouble())
        "first" -> list(args[0]).firstOrNull() ?: Value.Null
        "last" -> list(args[0]).lastOrNull() ?: Value.Null
        "map" -> Value.ListV(list(args[0]).mapIndexed { i, v -> applyLambda(ctx, args[1], v, i) })
        "filter" -> Value.ListV(list(args[0]).filterIndexed { i, v -> truthy(applyLambda(ctx, args[1], v, i)) })
        "find" -> list(args[0]).let { xs -> xs.indices.firstOrNull { truthy(applyLambda(ctx, args[1], xs[it], it)) }?.let { xs[it] } } ?: Value.Null
        "some" -> Value.of(list(args[0]).let { xs -> xs.indices.any { truthy(applyLambda(ctx, args[1], xs[it], it)) } })
        "every" -> Value.of(list(args[0]).let { xs -> xs.indices.all { truthy(applyLambda(ctx, args[1], xs[it], it)) } })
        "reduce" -> list(args[0]).fold(args[1]) { acc, v -> ctx.apply(args[2] as Value.Lambda, acc, v) }
        "sum" -> Value.Num(list(args[0]).sumOf { num(it) })
        "join" -> Value.Str(list(args[0]).joinToString(str(args[1])) { Format.str(it) })
        "append" -> Value.ListV(list(args[0]) + args[1])
        "removeAt" -> {
            val xs = list(args[0]); val i = indexOrNull(args[1], xs.size)
            if (i == null) args[0] else Value.ListV(xs.toMutableList().also { it.removeAt(i) })
        }
        "set" -> {
            val xs = list(args[0]); val i = indexOrNull(args[1], xs.size)
            if (i == null) args[0] else Value.ListV(xs.toMutableList().also { it[i] = args[2] })
        }
        "slice" -> {
            val xs = list(args[0]); val n = xs.size
            val start = clampIndex(num(args[1]), n)
            val end = if (args.size > 2 && args[2] != Value.Null) clampIndex(num(args[2]), n) else n
            if (start >= end) Value.ListV(emptyList()) else Value.ListV(xs.subList(start, end))
        }
        "indexOf" -> Value.Num(list(args[0]).indexOfFirst { deepEquals(it, args[1]) }.toDouble())
        "sortBy" -> sortBy(ctx, list(args[0]), args[1] as Value.Lambda)
        "reverse" -> Value.ListV(list(args[0]).reversed())
        "range" -> {
            val n = num(args[0])
            if (!n.isFinite() || n <= 0.0) Value.ListV(emptyList())
            else Value.ListV((0 until min(floor(n).toInt(), 10_000)).map { Value.Num(it.toDouble()) })
        }
        "with" -> {
            val r = args[0] as Value.Rec
            val patch = args[1] as Value.Rec
            Value.Rec(r.fields + patch.fields)
        }

        "formatNumber" -> Value.Str(Format.formatNumber(num(args[0]), num(args[1])))
        "formatDuration" -> Value.Str(Format.formatDuration(num(args[0]), str(args[1])))
        "formatDate" -> Value.Str(Format.formatDate(num(args[0]), str(args[1]), envStr(ctx, "timeZone")))
        "plural" -> Value.Str(Plural.format(num(args[0]), (args[1] as Value.Rec).fields, envStr(ctx, "locale")))

        "gradient" -> Value.rec("\$gradient" to args[0], "angle" to args[1])
        "alpha" -> alpha(args[0] as Value.Rec, num(args[1]))
        "spring" -> Value.rec("\$motion" to Value.rec("kind" to Value.Str("spring"), "response" to args[0], "damping" to args[1]))
        "ease" -> curveMotion(args[0], listOf(0.42, 0.0, 0.58, 1.0))
        "linear" -> curveMotion(args[0], listOf(0.0, 0.0, 1.0, 1.0))
        "curve" -> curveMotion(args[0], listOf(num(args[1]), num(args[2]), num(args[3]), num(args[4])))

        else -> throw InterpreterException.EvalError("unknown stdlib function: $name")
    }

    private fun applyLambda(ctx: StdlibContext, fn: Value, item: Value, index: Int): Value {
        val lambda = fn as Value.Lambda
        return if (lambda.params.size >= 2) ctx.apply(lambda, item, Value.Num(index.toDouble())) else ctx.apply(lambda, item)
    }

    private fun sortBy(ctx: StdlibContext, items: List<Value>, keyFn: Value.Lambda): Value {
        val keys = items.map { ctx.apply(keyFn, it) }
        val byString = keys.firstOrNull { it is Value.Str } != null
        val indices = items.indices.sortedWith(
            if (byString) {
                compareBy(codePointComparator) { i: Int -> (keys[i] as? Value.Str)?.value ?: "" }
            } else {
                Comparator { i, j -> compareNumKey((keys[i] as? Value.Num)?.value ?: Double.NaN, (keys[j] as? Value.Num)?.value ?: Double.NaN) }
            },
        )
        return Value.ListV(indices.map { items[it] })
    }

    /** NaN sorts after every number; otherwise plain IEEE `<`/`>` — so `-0.0`/`0.0` compare equal and the sort stays stable. */
    private fun compareNumKey(a: Double, b: Double): Int = when {
        a.isNaN() && b.isNaN() -> 0
        a.isNaN() -> 1
        b.isNaN() -> -1
        a < b -> -1
        a > b -> 1
        else -> 0
    }

    private fun alpha(c: Value.Rec, a: Double): Value {
        val clamped = a.coerceIn(0.0, 1.0)
        val existing = (c.fields["alpha"] as? Value.Num)?.value ?: 1.0
        val product = existing * clamped
        val fields = LinkedHashMap(c.fields)
        if (product == 1.0) fields.remove("alpha") else fields["alpha"] = Value.Num(product)
        return Value.Rec(fields)
    }

    private fun curveMotion(duration: Value, bezier: List<Double>): Value = Value.rec(
        "\$motion" to Value.rec(
            "kind" to Value.Str("curve"),
            "duration" to duration,
            "bezier" to Value.ListV(bezier.map { Value.Num(it) }),
        ),
    )

    private fun interpolate(x: Double, input: List<Value>, output: List<Value>): Double {
        // Every element must be a number even on the early-return paths (same errors as before).
        for (v in input) num(v)
        for (v in output) num(v)
        if (input.size != output.size || input.size < 2) return if (output.isEmpty()) 0.0 else num(output[0])
        for (i in 0 until input.size - 1) if (num(input[i + 1]) < num(input[i])) return num(output[0])
        if (x <= num(input[0])) return num(output[0])
        if (x >= num(input[input.size - 1])) return num(output[output.size - 1])
        // last stop <= x — resolves a duplicated stop to the later segment.
        var i = 0
        for (j in input.indices) if (num(input[j]) <= x) i = j
        val x0 = num(input[i]); val x1 = num(input[i + 1])
        val t = if (x1 == x0) 1.0 else (x - x0) / (x1 - x0)
        return num(output[i]) + (num(output[i + 1]) - num(output[i])) * t
    }

    private fun envStr(ctx: StdlibContext, name: String): String = (ctx.scope.env(name) as? Value.Str)?.value ?: ""

    /** Full case mapping (can expand one code point to several, e.g. "ß" -> "SS"), applied per code point so no cross-character context (locale, final-sigma) ever applies. */
    private fun mapCase(s: String, toUpper: Boolean): String {
        if (s.all { it.code < 0x80 }) return if (toUpper) s.uppercase(Locale.ROOT) else s.lowercase(Locale.ROOT)
        val sb = StringBuilder(s.length)
        var i = 0
        while (i < s.length) {
            val cp = s.codePointAt(i)
            val one = String(Character.toChars(cp))
            sb.append(if (toUpper) one.uppercase(Locale.ROOT) else one.lowercase(Locale.ROOT))
            i += Character.charCount(cp)
        }
        return sb.toString()
    }

    private fun trimWhitespace(s: String): String {
        var start = 0
        var end = s.length
        while (start < end) {
            val cp = s.codePointAt(start)
            if (!isWhitespace(cp)) break
            start += Character.charCount(cp)
        }
        while (end > start) {
            val cp = s.codePointBefore(end)
            if (!isWhitespace(cp)) break
            end -= Character.charCount(cp)
        }
        return s.substring(start, end)
    }

    private fun clampIndex(n: Double, length: Int): Int {
        val i = if (n.isNaN()) 0 else floor(n).toInt()
        return i.coerceIn(0, length)
    }

    private fun indexOrNull(v: Value, count: Int): Int? {
        val n = (v as? Value.Num)?.value ?: return null
        if (n.isNaN() || n != floor(n)) return null
        val i = n.toInt()
        return if (i in 0 until count) i else null
    }
}

private fun codePoints(s: String): List<Int> {
    val list = ArrayList<Int>(s.length)
    var i = 0
    while (i < s.length) {
        val cp = s.codePointAt(i)
        list.add(cp)
        i += Character.charCount(cp)
    }
    return list
}

private fun num(v: Value): Double = (v as? Value.Num)?.value ?: throw InterpreterException.EvalError("expected number, got ${describe(v)}")
private fun str(v: Value): String = (v as? Value.Str)?.value ?: throw InterpreterException.EvalError("expected string, got ${describe(v)}")
private fun list(v: Value): List<Value> = (v as? Value.ListV)?.items ?: throw InterpreterException.EvalError("expected list, got ${describe(v)}")
private fun truthy(v: Value): Boolean = (v as? Value.Bool)?.value ?: throw InterpreterException.EvalError("expected bool, got ${describe(v)}")
private fun typeError(fn: String, v: Value): Throwable = InterpreterException.EvalError("$fn: unsupported argument type ${describe(v)}")
