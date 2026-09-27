import Foundation

public enum InterpreterError: Error, Sendable, CustomStringConvertible {
    case budgetExceeded
    case typeMismatch(String)
    case unknown(String)
    case unknownExprTag(String)
    case unknownStmtTag(String)

    public var description: String {
        switch self {
        case .budgetExceeded: return "step budget exceeded"
        case let .typeMismatch(m): return "type mismatch: \(m)"
        case let .unknown(m): return "unknown: \(m)"
        case let .unknownExprTag(t): return "unknown expression tag: \(t)"
        case let .unknownStmtTag(t): return "unknown statement tag: \(t)"
        }
    }
}

/// One evaluation/run gets its own budget (10 000 steps): shared across nested eval calls
/// within that run (lambda bodies, stdlib HOF callbacks, nested `run` handler calls), never
/// across separate top-level calls.
final class StepBudget {
    private var remaining: Int
    init(_ limit: Int) { remaining = limit }
    func tick() throws {
        remaining -= 1
        if remaining < 0 { throw InterpreterError.budgetExceeded }
    }
}

func describeError(_ error: Error) -> String {
    // `String(describing:)` already resolves to `.description` for our CustomStringConvertible
    // error types; falls back to the default Error description otherwise.
    String(describing: error)
}

/// Mutable per-run state threaded through statement execution: the locals chain (grows with
/// each `let`) and the active `animate` transaction (spec §4.1).
private final class HandlerContext {
    var locals: Locals?
    var transaction: Value?
    init(locals: Locals?, transaction: Value? = nil) {
        self.locals = locals
        self.transaction = transaction
    }
}

/// Evaluates wire `Expr`/`Stmt` against a `Scope`. Stateless itself (only holds the
/// document's `fns`/`handlers` for lookups), so a plain `final class` — not actor-isolated,
/// which keeps it callable from `@MainActor` code (the future store) without extra hops,
/// and just as usable from a background context or a test.
public final class Interpreter: Sendable {
    let fns: [String: Fn]
    let handlers: [String: Handler]

    public init(fns: [String: Fn] = [:], handlers: [String: Handler] = [:]) {
        self.fns = fns
        self.handlers = handlers
    }

    public convenience init(document: Document) {
        self.init(fns: document.fns, handlers: document.handlers)
    }

    // MARK: - eval

    /// Pure expression evaluation. Fresh 10 000-step budget per call.
    public func eval(_ expr: Expr, scope: any Scope, locals: Locals? = nil) throws -> Value {
        try evalStep(expr, scope: scope, locals: locals, budget: StepBudget(10_000))
    }

    private func evalStep(_ expr: Expr, scope: any Scope, locals: Locals?, budget: StepBudget) throws -> Value {
        try budget.tick()
        switch expr {
        case let .literal(v):
            return v
        case let .state(n):
            return scope.state(n)
        case let .instanceState(n):
            return scope.instanceState(n)
        case let .computed(n):
            return scope.computed(n)
        case let .data(n):
            return scope.data(n)
        case let .local(n):
            return locals?.lookup(n) ?? .null
        case let .param(n):
            return scope.param(n)
        case let .env(n):
            return scope.env(n)
        case let .token(g, n):
            return scope.token(g, n)
        case let .driver(k, id, axis):
            return .number(scope.driver(k, id, axis))
        case .selfPressed:
            return .bool(scope.selfPressed())
        case let .get(objExpr, field):
            let obj = try evalStep(objExpr, scope: scope, locals: locals, budget: budget)
            if case let .record(fields) = obj { return fields[field] ?? .null }
            return .null
        case let .at(objExpr, keyExpr):
            let obj = try evalStep(objExpr, scope: scope, locals: locals, budget: budget)
            let key = try evalStep(keyExpr, scope: scope, locals: locals, budget: budget)
            return atIndex(obj, key)
        case let .binary(op, a, b):
            return try evalBinary(op, a, b, scope: scope, locals: locals, budget: budget)
        case let .unary(op, a):
            let v = try evalStep(a, scope: scope, locals: locals, budget: budget)
            switch op {
            case "!":
                guard case let .bool(b) = v else { throw InterpreterError.typeMismatch("un !: not a bool") }
                return .bool(!b)
            case "-":
                guard case let .number(n) = v else { throw InterpreterError.typeMismatch("un -: not a number") }
                return .number(-n)
            default:
                throw InterpreterError.unknown("un:\(op)")
            }
        case let .ternary(c, t, f):
            let cv = try evalStep(c, scope: scope, locals: locals, budget: budget)
            guard case let .bool(cb) = cv else { throw InterpreterError.typeMismatch("if: condition not a bool") }
            return cb
                ? try evalStep(t, scope: scope, locals: locals, budget: budget)
                : try evalStep(f, scope: scope, locals: locals, budget: budget)
        case let .stdlib(name, argExprs):
            return try callStdlib(name, argExprs, scope: scope, locals: locals, budget: budget)
        case let .userFn(name, argExprs):
            guard let fn = fns[name] else { throw InterpreterError.unknown("u:\(name)") }
            let argVals = try argExprs.map { try evalStep($0, scope: scope, locals: locals, budget: budget) }
            let fnLocals = bind(fn.params, argVals, parent: nil)
            return try evalStep(fn.body, scope: scope, locals: fnLocals, budget: budget)
        case let .lambda(params, body):
            return .lambda(Lambda(params: params, body: body, captured: locals))
        case let .list(items):
            return .list(try items.map { try evalStep($0, scope: scope, locals: locals, budget: budget) })
        case let .record(fields):
            var result: [String: Value] = [:]
            for (k, e) in fields { result[k] = try evalStep(e, scope: scope, locals: locals, budget: budget) }
            return .record(result)
        case let .interpolate(parts):
            var s = ""
            for p in parts { s += Format.str(try evalStep(p, scope: scope, locals: locals, budget: budget)) }
            return .string(s)
        case let .unknown(tag):
            throw InterpreterError.unknownExprTag(tag)
        }
    }

    private func bind(_ params: [String], _ args: [Value], parent: Locals?) -> Locals? {
        var locals = parent
        for (i, p) in params.enumerated() {
            locals = Locals(name: p, value: i < args.count ? args[i] : .null, parent: locals)
        }
        return locals
    }

    private func atIndex(_ obj: Value, _ key: Value) -> Value {
        switch (obj, key) {
        case let (.list(items), .number(n)):
            guard n.truncatingRemainder(dividingBy: 1) == 0, n >= 0, Int(n) < items.count else { return .null }
            return items[Int(n)]
        case let (.record(fields), .string(k)):
            return fields[k] ?? .null
        default:
            return .null
        }
    }

    private func evalBinary(_ op: String, _ aExpr: Expr, _ bExpr: Expr, scope: any Scope, locals: Locals?, budget: StepBudget) throws -> Value {
        switch op {
        case "&&":
            guard case let .bool(ab) = try evalStep(aExpr, scope: scope, locals: locals, budget: budget) else {
                throw InterpreterError.typeMismatch("&&: lhs not bool")
            }
            if !ab { return .bool(false) }
            guard case let .bool(bb) = try evalStep(bExpr, scope: scope, locals: locals, budget: budget) else {
                throw InterpreterError.typeMismatch("&&: rhs not bool")
            }
            return .bool(bb)
        case "||":
            guard case let .bool(ab) = try evalStep(aExpr, scope: scope, locals: locals, budget: budget) else {
                throw InterpreterError.typeMismatch("||: lhs not bool")
            }
            if ab { return .bool(true) }
            guard case let .bool(bb) = try evalStep(bExpr, scope: scope, locals: locals, budget: budget) else {
                throw InterpreterError.typeMismatch("||: rhs not bool")
            }
            return .bool(bb)
        case "??":
            let a = try evalStep(aExpr, scope: scope, locals: locals, budget: budget)
            if case .null = a { return try evalStep(bExpr, scope: scope, locals: locals, budget: budget) }
            return a
        case "==":
            let a = try evalStep(aExpr, scope: scope, locals: locals, budget: budget)
            let b = try evalStep(bExpr, scope: scope, locals: locals, budget: budget)
            return .bool(a == b)
        case "!=":
            let a = try evalStep(aExpr, scope: scope, locals: locals, budget: budget)
            let b = try evalStep(bExpr, scope: scope, locals: locals, budget: budget)
            return .bool(!(a == b))
        case "+":
            let a = try evalStep(aExpr, scope: scope, locals: locals, budget: budget)
            let b = try evalStep(bExpr, scope: scope, locals: locals, budget: budget)
            switch (a, b) {
            case let (.number(x), .number(y)): return .number(x + y)
            case let (.string(x), .string(y)): return .string(x + y)
            default: throw InterpreterError.typeMismatch("+: mismatched operand types")
            }
        case "-", "*", "/", "%":
            let a = try evalStep(aExpr, scope: scope, locals: locals, budget: budget)
            let b = try evalStep(bExpr, scope: scope, locals: locals, budget: budget)
            guard case let .number(x) = a, case let .number(y) = b else {
                throw InterpreterError.typeMismatch("\(op): not numbers")
            }
            switch op {
            case "-": return .number(x - y)
            case "*": return .number(x * y)
            case "/": return .number(x / y)
            default: return .number(x.truncatingRemainder(dividingBy: y))
            }
        case "<", "<=", ">", ">=":
            let a = try evalStep(aExpr, scope: scope, locals: locals, budget: budget)
            let b = try evalStep(bExpr, scope: scope, locals: locals, budget: budget)
            switch (a, b) {
            case let (.number(x), .number(y)):
                switch op {
                case "<": return .bool(x < y)
                case "<=": return .bool(x <= y)
                case ">": return .bool(x > y)
                default: return .bool(x >= y)
                }
            case let (.string(x), .string(y)):
                let c = CodePoints.compare(x, y)
                switch op {
                case "<": return .bool(c < 0)
                case "<=": return .bool(c <= 0)
                case ">": return .bool(c > 0)
                default: return .bool(c >= 0)
                }
            default:
                throw InterpreterError.typeMismatch("\(op): not comparable")
            }
        default:
            throw InterpreterError.unknown("op:\(op)")
        }
    }

    // MARK: - stdlib dispatch

    private func callLambda(_ lambda: Lambda, args: [Value], scope: any Scope, budget: StepBudget) throws -> Value {
        try budget.tick()
        let locals = bind(lambda.params, args, parent: lambda.captured)
        return try evalStep(lambda.body, scope: scope, locals: locals, budget: budget)
    }

    private func callStdlib(_ name: String, _ argExprs: [Expr], scope: any Scope, locals: Locals?, budget: StepBudget) throws -> Value {
        let args = try argExprs.map { try evalStep($0, scope: scope, locals: locals, budget: budget) }

        if Stdlib.nullPropagatingReceiver.contains(name), let first = args.first, first == .null {
            return .null
        }

        func num(_ i: Int) throws -> Double {
            guard i < args.count, case let .number(n) = args[i] else { throw InterpreterError.typeMismatch("\(name): arg \(i) not a number") }
            return n
        }
        func str(_ i: Int) throws -> String {
            guard i < args.count, case let .string(s) = args[i] else { throw InterpreterError.typeMismatch("\(name): arg \(i) not a string") }
            return s
        }
        func list(_ i: Int) throws -> [Value] {
            guard i < args.count, case let .list(l) = args[i] else { throw InterpreterError.typeMismatch("\(name): arg \(i) not a list") }
            return l
        }
        func optNum(_ i: Int) -> Double? {
            guard i < args.count, case let .number(n) = args[i] else { return nil }
            return n
        }
        func numList(_ i: Int) throws -> [Double] {
            try list(i).map {
                guard case let .number(n) = $0 else { throw InterpreterError.typeMismatch("\(name): expected number list") }
                return n
            }
        }
        func lambdaArg(_ i: Int) throws -> Lambda {
            guard i < args.count, case let .lambda(f) = args[i] else { throw InterpreterError.typeMismatch("\(name): arg \(i) not a function") }
            return f
        }

        switch name {
        case "min": return .number(Stdlib.min(try num(0), try num(1)))
        case "max": return .number(Stdlib.max(try num(0), try num(1)))
        case "abs": return .number(Stdlib.abs(try num(0)))
        case "round": return .number(Stdlib.round(try num(0)))
        case "floor": return .number(Stdlib.floor(try num(0)))
        case "ceil": return .number(Stdlib.ceil(try num(0)))
        case "sqrt": return .number(Stdlib.sqrt(try num(0)))
        case "pow": return .number(Stdlib.pow(try num(0), try num(1)))
        case "sin": return .number(Stdlib.sin(try num(0)))
        case "cos": return .number(Stdlib.cos(try num(0)))
        case "clamp": return .number(Stdlib.clamp(try num(0), try num(1), try num(2)))
        case "interpolate": return .number(Stdlib.interpolate(try num(0), try numList(1), try numList(2)))

        case "length":
            guard case let .string(s) = args[0] else { throw InterpreterError.typeMismatch("length: not a string") }
            return .number(Stdlib.length(s))
        case "isEmpty":
            switch args[0] {
            case let .string(s): return .bool(s.unicodeScalars.isEmpty)
            case let .list(l): return .bool(l.isEmpty)
            default: throw InterpreterError.typeMismatch("isEmpty: not a string or list")
            }
        case "upper": return .string(Stdlib.upper(try str(0)))
        case "lower": return .string(Stdlib.lower(try str(0)))
        case "trim": return .string(Stdlib.trim(try str(0)))
        case "contains":
            switch args[0] {
            case let .string(s):
                guard case let .string(t) = args[1] else { throw InterpreterError.typeMismatch("contains: not a string") }
                return .bool(Stdlib.containsString(s, t))
            case let .list(l):
                return .bool(l.contains(args[1]))
            default:
                throw InterpreterError.typeMismatch("contains: not a string or list")
            }
        case "startsWith": return .bool(Stdlib.startsWith(try str(0), try str(1)))
        case "endsWith": return .bool(Stdlib.endsWith(try str(0), try str(1)))
        case "split": return .list(Stdlib.split(try str(0), try str(1)))
        case "replace": return .string(Stdlib.replace(try str(0), try str(1), try str(2)))
        case "matches": return .bool(Stdlib.matches(try str(0), try str(1)))
        case "substring": return .string(Stdlib.substring(try str(0), try num(1), optNum(2)))
        case "num": return Stdlib.num(try str(0)).map(Value.number) ?? .null
        case "str": return .string(Stdlib.str(args[0]))

        case "count": return .number(Stdlib.count(try list(0)))
        case "first": return Stdlib.first(try list(0))
        case "last": return Stdlib.last(try list(0))
        case "sum": return .number(Stdlib.sum(try list(0)))
        case "join": return .string(Stdlib.join(try list(0), try str(1)))
        case "append": return .list(Stdlib.append(try list(0), args[1]))
        case "removeAt": return .list(Stdlib.removeAt(try list(0), try num(1)))
        case "set": return .list(Stdlib.set(try list(0), try num(1), args[2]))
        case "slice": return .list(Stdlib.slice(try list(0), try num(1), optNum(2)))
        case "indexOf": return .number(Stdlib.indexOf(try list(0), args[1]))
        case "reverse": return .list(Stdlib.reverse(try list(0)))
        case "range": return .list(Stdlib.range(try num(0)))
        case "with": return Stdlib.with(args[0], args[1])

        case "formatNumber": return .string(Stdlib.formatNumber(try num(0), try num(1)))
        case "formatDuration": return .string(Stdlib.formatDuration(try num(0), try str(1)))
        case "formatDate":
            let tz: String = { if case let .string(s) = scope.env("timeZone") { return s } else { return "UTC" } }()
            return .string(Stdlib.formatDate(try num(0), try str(1), timeZone: tz))
        case "plural":
            guard case let .record(forms) = args[1] else { throw InterpreterError.typeMismatch("plural: forms not a record") }
            let locale: String = { if case let .string(s) = scope.env("locale") { return s } else { return "ru" } }()
            return .string(Stdlib.plural(try num(0), forms, locale: locale))

        case "gradient": return Stdlib.gradient(try list(0), try num(1))
        case "alpha": return Stdlib.alpha(args[0], try num(1))
        case "spring": return Stdlib.spring(try num(0), try num(1))
        case "ease": return Stdlib.ease(try num(0))
        case "linear": return Stdlib.linear(try num(0))
        case "curve": return Stdlib.curve(try num(0), try num(1), try num(2), try num(3), try num(4))

        case "map":
            let xs = try list(0)
            let f = try lambdaArg(1)
            return .list(try xs.enumerated().map { i, item in
                try callLambda(f, args: [item, .number(Double(i))], scope: scope, budget: budget)
            })
        case "filter":
            let xs = try list(0)
            let f = try lambdaArg(1)
            var result: [Value] = []
            for (i, item) in xs.enumerated() {
                if case .bool(true) = try callLambda(f, args: [item, .number(Double(i))], scope: scope, budget: budget) {
                    result.append(item)
                }
            }
            return .list(result)
        case "find":
            let xs = try list(0)
            let f = try lambdaArg(1)
            for (i, item) in xs.enumerated() {
                if case .bool(true) = try callLambda(f, args: [item, .number(Double(i))], scope: scope, budget: budget) {
                    return item
                }
            }
            return .null
        case "some":
            let xs = try list(0)
            let f = try lambdaArg(1)
            for (i, item) in xs.enumerated() {
                if case .bool(true) = try callLambda(f, args: [item, .number(Double(i))], scope: scope, budget: budget) {
                    return .bool(true)
                }
            }
            return .bool(false)
        case "every":
            let xs = try list(0)
            let f = try lambdaArg(1)
            for (i, item) in xs.enumerated() {
                guard case .bool(true) = try callLambda(f, args: [item, .number(Double(i))], scope: scope, budget: budget) else {
                    return .bool(false)
                }
            }
            return .bool(true)
        case "reduce":
            let xs = try list(0)
            let f = try lambdaArg(2)
            var acc = args[1]
            for item in xs { acc = try callLambda(f, args: [acc, item], scope: scope, budget: budget) }
            return acc
        case "sortBy":
            let xs = try list(0)
            let f = try lambdaArg(1)
            let decorated = try xs.map { item in (item, try callLambda(f, args: [item], scope: scope, budget: budget)) }
            let sorted = decorated.sorted { a, b in
                switch (a.1, b.1) {
                case let (.number(x), .number(y)):
                    if x.isNaN { return false }
                    if y.isNaN { return true }
                    return x < y
                case let (.string(x), .string(y)):
                    return CodePoints.compare(x, y) < 0
                default:
                    return false
                }
            }
            return .list(sorted.map { $0.0 })

        default:
            throw InterpreterError.unknown("f:\(name)")
        }
    }

    // MARK: - statements (handler runs, events)

    /// Runs a top-level handler by name. Fresh locals from `args` bound to its params; fresh
    /// 10 000-step budget for the whole run.
    public func run(handler name: String, args: [Value], scope: any Scope, effects: any EffectSink) async {
        guard let handler = handlers[name] else {
            effects.error("unknown handler \(name)")
            return
        }
        await runStatements(handler.body, scope: scope, locals: bind(handler.params, args, parent: nil), effects: effects)
    }

    /// Entry point used both for handler bodies and for `on.*` event bodies (the latter via
    /// `runEvent`, which prepares `locals` with `event`/repeat bindings first).
    public func runStatements(_ stmts: [Stmt], scope: any Scope, locals: Locals?, effects: any EffectSink) async {
        let budget = StepBudget(10_000)
        let ctx = HandlerContext(locals: locals)
        do {
            try await execList(stmts, scope: scope, ctx: ctx, effects: effects, budget: budget)
        } catch {
            effects.error(describeError(error))
        }
    }

    private func execList(_ stmts: [Stmt], scope: any Scope, ctx: HandlerContext, effects: any EffectSink, budget: StepBudget) async throws {
        for stmt in stmts {
            try await exec(stmt, scope: scope, ctx: ctx, effects: effects, budget: budget)
        }
    }

    private func exec(_ stmt: Stmt, scope: any Scope, ctx: HandlerContext, effects: any EffectSink, budget: StepBudget) async throws {
        try budget.tick()
        switch stmt {
        case let .set(scopeKind, name, pathExprs, valueExpr):
            let path = try pathExprs.map { try evalStep($0, scope: scope, locals: ctx.locals, budget: budget) }
            let value = try evalStep(valueExpr, scope: scope, locals: ctx.locals, budget: budget)
            scope.set(scope: scopeKind, name: name, path: path, value: value, transaction: ctx.transaction)
        case let .letStmt(name, valueExpr):
            let value = try evalStep(valueExpr, scope: scope, locals: ctx.locals, budget: budget)
            ctx.locals = Locals(name: name, value: value, parent: ctx.locals)
        case let .ifStmt(condExpr, thenBody, elseBody):
            guard case let .bool(cb) = try evalStep(condExpr, scope: scope, locals: ctx.locals, budget: budget) else {
                throw InterpreterError.typeMismatch("if: condition not bool")
            }
            try await execList(cb ? thenBody : elseBody, scope: scope, ctx: ctx, effects: effects, budget: budget)
        case let .anim(motionExpr, body):
            let motion = try evalStep(motionExpr, scope: scope, locals: ctx.locals, budget: budget)
            let previous = ctx.transaction
            ctx.transaction = motion
            defer { ctx.transaction = previous }
            try await execList(body, scope: scope, ctx: ctx, effects: effects, budget: budget)
        case let .call(op, argsExpr, resultName, _):
            // `awaited` (wire `await`) only controls whether the *value* is bound — the call
            // is always issued in statement order so it's recorded synchronously (harness
            // rule 3, contract/conformance/compiled/handlers.json `h.call.fireforget`: even a
            // bare, unbound `call` records its effect immediately). The grammar never binds a
            // result without `await` anyway (`resultName` is null whenever `awaited` is
            // false), so this loses no real "fire and forget" behavior for well-formed
            // programs — a real networking EffectSink is free to return immediately from its
            // own `call()` and finish the network round trip in the background internally.
            let argsValue = try evalStep(argsExpr, scope: scope, locals: ctx.locals, budget: budget)
            let result = await effects.call(op: op, args: argsValue)
            if let resultName { ctx.locals = Locals(name: resultName, value: result, parent: ctx.locals) }
        case let .doEffect(effect, argExprs, awaited):
            let argVals = try argExprs.map { try evalStep($0, scope: scope, locals: ctx.locals, budget: budget) }
            await effects.perform(effect: effect, args: argVals, awaited: awaited)
        case let .run(handlerName, argExprs):
            guard let handler = handlers[handlerName] else { throw InterpreterError.unknown("run:\(handlerName)") }
            let argVals = try argExprs.map { try evalStep($0, scope: scope, locals: ctx.locals, budget: budget) }
            let nested = HandlerContext(locals: bind(handler.params, argVals, parent: nil), transaction: ctx.transaction)
            try await execList(handler.body, scope: scope, ctx: nested, effects: effects, budget: budget)
        case let .unknown(tag):
            throw InterpreterError.unknownStmtTag(tag)
        }
    }

    // MARK: - events

    /// Locates `nodeId` in `root`, runs its `on[eventName]` body with `event` (and, for a
    /// node under a `repeat`, the repeat's `as`/`index` bindings for `repeatIndex`) in locals.
    public func runEvent(
        _ eventName: String, root: Node, nodeId: String, payload: Value,
        repeatIndex: Int?, scope: any Scope, effects: any EffectSink
    ) async {
        do {
            guard let located = try locate(root, targetId: nodeId, scope: scope, outerLocals: nil, repeatIndex: repeatIndex) else {
                effects.error("event target not found: \(nodeId)")
                return
            }
            guard let stmts = located.node.on[eventName] else {
                effects.error("no handler for event \(eventName) on \(nodeId)")
                return
            }
            let locals = Locals(name: "event", value: payload, parent: located.locals)
            await runStatements(stmts, scope: scope, locals: locals, effects: effects)
        } catch {
            effects.error(describeError(error))
        }
    }

    /// Walks the static tree, threading repeat bindings for any `rep`-bearing ancestor (using
    /// the caller-supplied `repeatIndex` — V1 only exercises a single repeat level on the
    /// event's own target node; nested repeats above the target aren't disambiguated).
    private func locate(
        _ node: Node, targetId: String, scope: any Scope, outerLocals: Locals?, repeatIndex: Int?
    ) throws -> (node: Node, locals: Locals?)? {
        var locals = outerLocals
        if let rep = node.rep, let idx = repeatIndex {
            let budget = StepBudget(10_000)
            let overValue = try evalStep(rep.over, scope: scope, locals: outerLocals, budget: budget)
            if case let .list(items) = overValue, idx >= 0, idx < items.count {
                locals = Locals(name: rep.indexName, value: .number(Double(idx)), parent: locals)
                locals = Locals(name: rep.asName, value: items[idx], parent: locals)
            }
        }
        if node.id == targetId { return (node, locals) }
        for child in node.children {
            if let found = try locate(child, targetId: targetId, scope: scope, outerLocals: locals, repeatIndex: repeatIndex) {
                return found
            }
        }
        return nil
    }
}
