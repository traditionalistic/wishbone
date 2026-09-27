import Testing
@testable import WishboneCore

/// Hand-written wire AST covering the same semantics as contract/conformance/expr.json,
/// since the compiled vectors don't exist yet (ConformanceTests picks those up once they do).
@MainActor
@Suite struct InterpreterEvalTests {
    let interpreter = Interpreter()

    private func eval(_ json: String, scope: MapScope = MapScope()) throws -> Value {
        try interpreter.eval(try Expr(json: json), scope: scope)
    }

    @Test func arithmeticPrecedenceAndAssociativity() throws {
        // 1 + 2 * 3 = 7
        #expect(try eval(#"["op", "+", ["v", 1], ["op", "*", ["v", 2], ["v", 3]]]"#) == .number(7))
        // -7 % 3 = -1 (fmod, sign of dividend)
        #expect(try eval(#"["op", "%", ["un", "-", ["v", 7]], ["v", 3]]"#) == .number(-1))
    }

    @Test func divisionByZeroProducesIEEE754Specials() throws {
        #expect(try eval(#"["op", "/", ["v", 1], ["v", 0]]"#) == .number(.infinity))
        #expect(try eval(#"["op", "/", ["v", -1], ["v", 0]]"#) == .number(-.infinity))
        let nanResult = try eval(#"["op", "/", ["v", 0], ["v", 0]]"#)
        if case let .number(n) = nanResult { #expect(n.isNaN) } else { Issue.record("expected number") }
    }

    @Test func shortCircuitAvoidsEvaluatingRHS() throws {
        // false && (1/0 > 0) must not throw evaluating the RHS incorrectly, and must be false.
        #expect(try eval(#"["op", "&&", ["v", false], ["op", ">", ["op", "/", ["v", 1], ["v", 0]], ["v", 0]]]"#) == .bool(false))
        #expect(try eval(#"["op", "??", ["v", null], ["v", 5]]"#) == .number(5))
    }

    @Test func stringComparisonIsCodePointOrder() throws {
        #expect(try eval(#"["op", "<", ["v", "B"], ["v", "a"]]"#) == .bool(true))
    }

    @Test func nanComparisonsAreAlwaysFalse() throws {
        // cmp.nan.le / cmp.nan.ge: NaN is neither <=, >=, <, nor > anything, including itself.
        let nan = #"["op", "/", ["v", 0], ["v", 0]]"#
        #expect(try eval("[\"op\", \"<=\", \(nan), [\"v\", 1]]") == .bool(false))
        #expect(try eval("[\"op\", \">=\", \(nan), \(nan)]") == .bool(false))
    }

    @Test func indexingDoesNotTrapOnExtremeMagnitudes() throws {
        // ["at", list, 1e19] must not crash converting a huge-but-"integral" Double to Int.
        #expect(try eval(#"["at", ["v", [1,2,3]], ["v", 1e19]]"#) == .null)
        #expect(try eval(#"["at", ["v", [1,2,3]], ["v", -1e19]]"#) == .null)
    }

    @Test func stringInterpolationConcatenatesStr() throws {
        let scope = MapScope(state: ["name": .string("Мир")])
        #expect(try eval(#"["str", [["v", "n="], ["v", 3], ["v", " f="], ["v", 2.5], ["v", " b="], ["v", true]]]"#) == .string("n=3 f=2.5 b=true"))
        #expect(try eval(#"["str", [["v", "Привет, "], ["s", "name"], ["v", "!"]]]"#, scope: scope) == .string("Привет, Мир!"))
    }

    @Test func getAndAtAreNullSafe() throws {
        #expect(try eval(#"["get", ["v", null], "name"]"#) == .null)
        #expect(try eval(#"["at", ["v", [1, 2, 3]], ["v", 5]]"#) == .null)
        #expect(try eval(#"["get", ["v", {"a": {"b": 1}}], "a"]"#) == .record(["b": .number(1)]))
        #expect(try eval(#"["at", ["v", {"k": 1}], ["v", "k"]]"#) == .number(1)) // map<T> string-keyed via "at"
    }

    @Test func listIndexRequiresAnIntegerInRange() throws {
        // [1, 2, 3][1.5] ?? -1 == -1 (contract/conformance/expr.json list.index.frac)
        #expect(try eval(#"["op", "??", ["at", ["v", [1,2,3]], ["v", 1.5]], ["v", -1]]"#) == .number(-1))
    }

    @Test func mapIndexingByStringKeyViaAt() throws {
        // state m: map<number> = {a: 1}; m["a"] ?? 0 == 1; m["z"] ?? 0 == 0
        let scope = MapScope(state: ["m": .record(["a": .number(1)])])
        #expect(try eval(#"["op", "??", ["at", ["s", "m"], ["v", "a"]], ["v", 0]]"#, scope: scope) == .number(1))
        #expect(try eval(#"["op", "??", ["at", ["s", "m"], ["v", "z"]], ["v", 0]]"#, scope: scope) == .number(0))
    }

    @Test func recordEqualityAndFieldAccess() throws {
        #expect(try eval(#"["op", "==", ["v", {"a": 1, "b": 2}], ["v", {"b": 2, "a": 1}]]"#) == .bool(true))
    }

    @Test func listHigherOrderFunctions() throws {
        #expect(try eval(#"["f", "map", [["v", [1,2,3]], ["lam", ["x"], ["op", "*", ["l", "x"], ["v", 2]]]]]"#) == .list([.number(2), .number(4), .number(6)]))
        #expect(try eval(#"["f", "filter", [["v", [1,2,3,4]], ["lam", ["x"], ["op", "==", ["op", "%", ["l", "x"], ["v", 2]], ["v", 0]]]]]"#) == .list([.number(2), .number(4)]))
        #expect(try eval(#"["f", "map", [["v", ["a","b"]], ["lam", ["x","i"], ["op", "+", ["l", "x"], ["f", "str", [["l","i"]]]]]]]"#) == .list([.string("a0"), .string("b1")]))
        #expect(try eval(#"["f", "reduce", [["v", [1,2,3]], ["v", 0], ["lam", ["acc","x"], ["op", "+", ["l","acc"], ["l","x"]]]]]"#) == .number(6))
    }

    @Test func nullPropagationOnOptionalReceiver() throws {
        // xs.first where xs is null (as if from a null-propagated optional chain)
        #expect(try eval(#"["f", "first", [["v", null]]]"#) == .null)
        #expect(try eval(#"["f", "length", [["v", null]]]"#) == .null)
    }

    @Test func tokensSpaceAndRadiusResolveFromTheme() throws {
        // korpu.json: space.lg=16, space.xs=4; radius.pill=9999
        #expect(try eval(#"["op", "+", ["t", "space", "lg"], ["t", "space", "xs"]]"#) == .number(20))
        #expect(try eval(#"["t", "radius", "pill"]"#) == .number(9999))
    }

    @Test func colorAndMotionTokensStaySymbolic() throws {
        #expect(try eval(#"["t", "color", "el16"]"#) == .record(["$color": .string("el16")]))
        #expect(try eval(#"["t", "motion", "snappy"]"#) == .record(["$motion": .string("snappy")]))
    }

    @Test func envReadsFromScopeWithOverrides() throws {
        let scope = MapScope(env: defaultEnv(overrides: ["colorScheme": .string("dark"), "reduceMotion": .bool(true)]))
        #expect(try eval(#"["op", "&&", ["op", "==", ["e", "colorScheme"], ["v", "dark"]], ["e", "reduceMotion"]]"#, scope: scope) == .bool(true))
    }

    @Test func userFunctionSeesOnlyItsOwnParams() throws {
        let interpreter = Interpreter(fns: ["tax": Fn(params: ["x"], body: try! Expr(json: #"["op", "*", ["l", "x"], ["v", 0.2]]"#))])
        let result = try interpreter.eval(try Expr(json: #"["u", "tax", [["v", 100]]]"#), scope: MapScope())
        #expect(result == .number(20))
    }

    @Test func dataAndParamAndComputedResolveThroughScope() throws {
        let scope = MapScope(
            state: ["a": .number(2)],
            computed: ["b": try! Expr(json: #"["op", "+", ["s", "a"], ["v", 1]]"#)],
            data: ["me": .record(["name": .string("Ann")])],
            params: ["id": .string("42")]
        )
        #expect(try eval(#"["get", ["d", "me"], "name"]"#, scope: scope) == .string("Ann"))
        #expect(try eval(#"["op", "+", ["v", "id="], ["p", "id"]]"#, scope: scope) == .string("id=42"))
        #expect(try eval(#"["c", "b"]"#, scope: scope) == .number(3))
    }

    @Test func stepBudgetExceededThrows() throws {
        // A *wide* list (not deeply nested — nesting depth would blow the call stack well
        // before hitting the step budget) with more elements than the 10 000-step budget.
        let expr = Expr.list(Array(repeating: .literal(.number(1)), count: 10_005))
        #expect(throws: InterpreterError.self) { try interpreter.eval(expr, scope: MapScope()) }
    }

    @Test func stepBudgetIsNotExceededJustUnderTheLimit() throws {
        let expr = Expr.list(Array(repeating: .literal(.number(1)), count: 9_990))
        #expect(try interpreter.eval(expr, scope: MapScope()) == .list(Array(repeating: .number(1), count: 9_990)))
    }
}
