package app.wishbone.core

import kotlinx.coroutines.CancellationException
import kotlin.math.floor

sealed class InterpreterException(message: String) : Exception(message) {
    class BudgetExceeded : InterpreterException("step budget exceeded (10000)")
    class EvalError(message: String) : InterpreterException(message)
}

private const val STEP_BUDGET = 10_000

/** One step per expression/statement node visited, scoped to a single top-level eval/run call. */
private class Budget {
    private var remaining = STEP_BUDGET
    fun tick() {
        remaining--
        if (remaining < 0) throw InterpreterException.BudgetExceeded()
    }
}

/**
 * Evaluates expressions (pure) and runs statement lists (handler/event bodies, suspend — effects
 * are dispatched through [effects]). `doc` supplies user `fn`/`handler` declarations; everything
 * else state/data/param/env/token/driver goes through [scope].
 */
class Interpreter(
    private val doc: Document,
    private val scope: Scope,
    private val effects: EffectSink,
) {
    /** Pure; throws [InterpreterException] on error (unknown tag, budget exceeded, type error). */
    fun eval(expr: Expr, locals: Locals?): Value = evalInternal(expr, locals, Budget())

    /** Invokes a top-level `handler` by name (unknown name reports via [EffectSink.error] rather than throwing). */
    suspend fun run(handlerName: String, args: List<Value>) {
        val handler = doc.handlers[handlerName]
        if (handler == null) {
            effects.error("unknown handler: $handlerName")
            return
        }
        var locals: Locals? = null
        for ((p, v) in handler.params.zip(args)) locals = locals.bind(p, v)
        runStatements(handler.body, locals)
    }

    /**
     * Entry point for `on.*` event bodies: [locals] carries repeat vars and `event` (bound by the
     * caller — :wishbone-core does not materialize the node tree, see core-decisions.md). Errors
     * abort the run and are reported via [EffectSink.error]; state written before the error stays.
     */
    suspend fun runStatements(stmts: List<Stmt>, locals: Locals?) {
        val budget = Budget()
        try {
            execStmts(stmts, locals, budget, transaction = null)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            effects.error(e.message ?: e.toString())
        }
    }

    private suspend fun execStmts(stmts: List<Stmt>, locals: Locals?, budget: Budget, transaction: Value?): Locals? {
        var loc = locals
        for (stmt in stmts) loc = execStmt(stmt, loc, budget, transaction)
        return loc
    }

    private suspend fun execStmt(stmt: Stmt, locals: Locals?, budget: Budget, transaction: Value?): Locals? {
        budget.tick()
        return when (stmt) {
            is Stmt.Set -> {
                val path = stmt.path.map { evalInternal(it, locals, budget) }
                val value = evalInternal(stmt.value, locals, budget)
                scope.set(stmt.scope, stmt.name, path, value, transaction)
                locals
            }
            is Stmt.Let -> locals.bind(stmt.name, evalInternal(stmt.value, locals, budget))
            is Stmt.If -> {
                val branch = if (asBool(evalInternal(stmt.cond, locals, budget))) stmt.then else stmt.otherwise
                execStmts(branch, locals, budget, transaction) // let-bindings inside the branch don't leak out
                locals
            }
            is Stmt.Anim -> {
                val motion = evalInternal(stmt.motion, locals, budget)
                execStmts(stmt.body, locals, budget, motion)
                locals
            }
            is Stmt.CallOp -> {
                // Always called synchronously, in statement order (deterministic effect recording).
                // `awaited` tells the SINK whether it may return a real result or must not block —
                // see EffectSink.call's contract; the interpreter itself never launches anything.
                val argsValue = evalInternal(stmt.args, locals, budget)
                val result = effects.call(stmt.op, argsValue, stmt.await)
                if (stmt.await && stmt.resultName != null) locals.bind(stmt.resultName, result) else locals
            }
            is Stmt.Do -> {
                val args = stmt.args.map { evalInternal(it, locals, budget) }
                effects.perform(stmt.effect, args, stmt.await)
                locals
            }
            is Stmt.Run -> {
                val handler = doc.handlers[stmt.handler]
                    ?: throw InterpreterException.EvalError("unknown handler: ${stmt.handler}")
                val args = stmt.args.map { evalInternal(it, locals, budget) }
                var handlerLocals: Locals? = null
                for ((p, v) in handler.params.zip(args)) handlerLocals = handlerLocals.bind(p, v)
                execStmts(handler.body, handlerLocals, budget, transaction)
                locals
            }
            is Stmt.Unknown -> throw InterpreterException.EvalError("unknown statement tag: ${stmt.tag}")
        }
    }

    private fun evalInternal(e: Expr, locals: Locals?, budget: Budget): Value {
        budget.tick()
        return when (e) {
            is Expr.Lit -> e.value
            is Expr.StateRef -> scope.state(e.name)
            is Expr.InstRef -> scope.instanceState(e.name)
            is Expr.ComputedRef -> scope.computed(e.name)
            is Expr.DataRef -> scope.data(e.name)
            is Expr.LocalRef -> locals.lookup(e.name) ?: Value.Null
            is Expr.ParamRef -> scope.param(e.name)
            is Expr.EnvRef -> scope.env(e.name)
            is Expr.TokenRef -> scope.token(e.group, e.name)
            is Expr.DriverRef -> Value.Num(scope.driver(e.kind, e.id, e.axis))
            Expr.SelfPressed -> Value.of(scope.selfPressed())
            is Expr.Get -> {
                val obj = evalInternal(e.obj, locals, budget)
                if (obj is Value.Rec) obj.fields[e.field] ?: Value.Null else Value.Null
            }
            is Expr.At -> evalAt(e, locals, budget)
            is Expr.BinOp -> evalBinOp(e, locals, budget)
            is Expr.UnOp -> evalUnOp(e, locals, budget)
            is Expr.If -> if (asBool(evalInternal(e.cond, locals, budget))) {
                evalInternal(e.then, locals, budget)
            } else {
                evalInternal(e.otherwise, locals, budget)
            }
            is Expr.Call -> evalCall(e, locals, budget)
            is Expr.UserCall -> evalUserCall(e, locals, budget)
            is Expr.Lam -> Value.Lambda(e.params, e.body, locals)
            is Expr.ListLit -> Value.ListV(e.items.map { evalInternal(it, locals, budget) })
            is Expr.RecLit -> Value.Rec(e.fields.mapValues { (_, v) -> evalInternal(v, locals, budget) })
            is Expr.StrInterp -> Value.Str(e.parts.joinToString("") { Format.str(evalInternal(it, locals, budget)) })
            is Expr.Unknown -> throw InterpreterException.EvalError("unknown expression tag: ${e.tag}")
        }
    }

    private fun evalAt(e: Expr.At, locals: Locals?, budget: Budget): Value {
        val obj = evalInternal(e.obj, locals, budget)
        val key = evalInternal(e.key, locals, budget)
        return when (obj) {
            is Value.ListV -> {
                val idx = (key as? Value.Num)?.value
                if (idx != null && idx == floor(idx) && idx >= 0 && idx < obj.items.size) obj.items[idx.toInt()] else Value.Null
            }
            is Value.Rec -> (key as? Value.Str)?.let { obj.fields[it.value] } ?: Value.Null
            else -> Value.Null
        }
    }

    private fun evalBinOp(e: Expr.BinOp, locals: Locals?, budget: Budget): Value {
        when (e.op) {
            "&&" -> return if (!asBool(evalInternal(e.a, locals, budget))) Value.FALSE else Value.of(asBool(evalInternal(e.b, locals, budget)))
            "||" -> return if (asBool(evalInternal(e.a, locals, budget))) Value.TRUE else Value.of(asBool(evalInternal(e.b, locals, budget)))
            "??" -> {
                val a = evalInternal(e.a, locals, budget)
                return if (a != Value.Null) a else evalInternal(e.b, locals, budget)
            }
        }
        val a = evalInternal(e.a, locals, budget)
        val b = evalInternal(e.b, locals, budget)
        return when (e.op) {
            "+" -> when {
                a is Value.Num && b is Value.Num -> Value.Num(a.value + b.value)
                a is Value.Str && b is Value.Str -> Value.Str(a.value + b.value)
                else -> throw InterpreterException.EvalError("'+' requires two numbers or two strings, got $a, $b")
            }
            "-" -> Value.Num(asNum(a) - asNum(b))
            "*" -> Value.Num(asNum(a) * asNum(b))
            "/" -> Value.Num(asNum(a) / asNum(b))
            "%" -> Value.Num(asNum(a) % asNum(b))
            "==" -> Value.of(deepEquals(a, b))
            "!=" -> Value.of(!deepEquals(a, b))
            "<" -> Value.of(isLess(a, b))
            "<=" -> Value.of(isLessOrEqual(a, b))
            ">" -> Value.of(isLess(b, a))
            ">=" -> Value.of(isLessOrEqual(b, a))
            else -> throw InterpreterException.EvalError("unknown operator: ${e.op}")
        }
    }

    /**
     * Direct IEEE comparison for numbers — NOT routed through a 3-way `compareTo`/`Int`, which
     * cannot represent "neither less nor greater nor equal" (any NaN comparison must be `false`,
     * for every one of `< <= > >=`, e.g. `NaN <= 1` and `NaN >= NaN` are both `false`). Code-point
     * ordering for strings.
     */
    private fun isLess(a: Value, b: Value): Boolean = when {
        a is Value.Num && b is Value.Num -> a.value < b.value
        a is Value.Str && b is Value.Str -> compareCodePoints(a.value, b.value) < 0
        else -> throw InterpreterException.EvalError("cannot compare $a and $b")
    }

    private fun isLessOrEqual(a: Value, b: Value): Boolean = when {
        a is Value.Num && b is Value.Num -> a.value <= b.value
        a is Value.Str && b is Value.Str -> compareCodePoints(a.value, b.value) <= 0
        else -> throw InterpreterException.EvalError("cannot compare $a and $b")
    }

    private fun evalUnOp(e: Expr.UnOp, locals: Locals?, budget: Budget): Value {
        val a = evalInternal(e.a, locals, budget)
        return when (e.op) {
            "!" -> Value.of(!asBool(a))
            "-" -> Value.Num(-asNum(a))
            else -> throw InterpreterException.EvalError("unknown unary operator: ${e.op}")
        }
    }

    private fun evalCall(e: Expr.Call, locals: Locals?, budget: Budget): Value {
        val argValues = e.args.map { evalInternal(it, locals, budget) }
        val ctx = object : StdlibContext {
            override val scope: Scope get() = this@Interpreter.scope
            override fun apply(lambda: Value.Lambda, args: List<Value>): Value {
                var lambdaLocals = lambda.closure
                for ((p, v) in lambda.params.zip(args)) lambdaLocals = lambdaLocals.bind(p, v)
                return evalInternal(lambda.body, lambdaLocals, budget)
            }
        }
        return Stdlib(ctx).call(e.name, argValues)
    }

    private fun evalUserCall(e: Expr.UserCall, locals: Locals?, budget: Budget): Value {
        val fn = doc.fns[e.name] ?: throw InterpreterException.EvalError("unknown fn: ${e.name}")
        val argValues = e.args.map { evalInternal(it, locals, budget) }
        var fnLocals: Locals? = null
        for ((p, v) in fn.params.zip(argValues)) fnLocals = fnLocals.bind(p, v)
        return evalInternal(fn.body, fnLocals, budget)
    }

    private fun asBool(v: Value): Boolean = (v as? Value.Bool)?.value ?: throw InterpreterException.EvalError("expected bool, got $v")
    private fun asNum(v: Value): Double = (v as? Value.Num)?.value ?: throw InterpreterException.EvalError("expected number, got $v")
}
