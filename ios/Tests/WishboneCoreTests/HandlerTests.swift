import Foundation
import Testing
@testable import WishboneCore

/// Hand-written wire AST covering the same semantics as contract/conformance/handlers.json.
@Suite struct HandlerTests {
    private func stmts(_ json: String) throws -> [Stmt] { try Stmt.array(json: Data(json.utf8)) }

    @Test func assignAddsArgumentToState() async throws {
        // handler inc(x) { n = n + x }
        let interpreter = Interpreter(handlers: [
            "inc": Handler(params: ["x"], body: try stmts(#"[["set", "s", "n", [], ["op", "+", ["s", "n"], ["l", "x"]]]]"#)),
        ])
        let scope = MapScope(state: ["n": .number(0)])
        let sink = RecordingEffectSink()
        await interpreter.run(handler: "inc", args: [.number(2)], scope: scope, effects: sink)
        #expect(scope.state("n") == .number(2))
        #expect(sink.effects.isEmpty)
        #expect(sink.errors.isEmpty)
    }

    @Test func ifElseBranches() async throws {
        // if step < 2 { step = step + 1 } else { done = true }
        let body = try stmts(#"""
        [["if", ["op", "<", ["s", "step"], ["v", 2]],
          [["set", "s", "step", [], ["op", "+", ["s", "step"], ["v", 1]]]],
          [["set", "s", "done", [], ["v", true]]]]]
        """#)
        let interpreter = Interpreter(handlers: ["next": Handler(params: [], body: body)])

        let thenScope = MapScope(state: ["step": .number(0), "done": .bool(false)])
        await interpreter.run(handler: "next", args: [], scope: thenScope, effects: RecordingEffectSink())
        #expect(thenScope.state("step") == .number(1))
        #expect(thenScope.state("done") == .bool(false))

        let elseScope = MapScope(state: ["step": .number(2), "done": .bool(false)])
        await interpreter.run(handler: "next", args: [], scope: elseScope, effects: RecordingEffectSink())
        #expect(elseScope.state("step") == .number(2))
        #expect(elseScope.state("done") == .bool(true))
    }

    @Test func pathAssignmentIntoListAndRecord() async throws {
        // items[i].done = !(items[i].done ?? false)
        let toggle = try stmts(#"""
        [["set", "s", "items", [["l", "i"], ["v", "done"]],
          ["un", "!", ["op", "??", ["get", ["at", ["s", "items"], ["l", "i"]], "done"], ["v", false]]]]]
        """#)
        let interpreter = Interpreter(handlers: ["toggle": Handler(params: ["i"], body: toggle)])
        let scope = MapScope(state: ["items": .list([
            .record(["title": .string("a"), "done": .bool(false)]),
            .record(["title": .string("b"), "done": .bool(false)]),
        ])])
        await interpreter.run(handler: "toggle", args: [.number(1)], scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("items") == .list([
            .record(["title": .string("a"), "done": .bool(false)]),
            .record(["title": .string("b"), "done": .bool(true)]),
        ]))
    }

    @Test func outOfRangePathAssignmentIsNoOp() async throws {
        let go = try stmts(#"[["set", "s", "items", [["v", 5], ["v", "done"]], ["v", true]]]"#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let scope = MapScope(state: ["items": .list([.record(["done": .bool(false)])])])
        await interpreter.run(handler: "go", args: [], scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("items") == .list([.record(["done": .bool(false)])]))
    }

    @Test func letEffectsRecordArgumentsAsWritten() async throws {
        let go = try stmts(#"""
        [["let", "id", ["op", "+", ["v", "x"], ["f", "str", [["v", 1]]]]],
         ["do", "navigate", [["l", "id"]], false],
         ["do", "haptic", [["v", "light"]], false]]
        """#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let sink = RecordingEffectSink()
        await interpreter.run(handler: "go", args: [], scope: MapScope(), effects: sink)
        #expect(sink.effects == [
            .record(["effect": .string("navigate"), "args": .list([.string("x1")])]),
            .record(["effect": .string("haptic"), "args": .list([.string("light")])]),
        ])
    }

    @Test func awaitedCallBindsResultAndUnawaitedIsFireAndForget() async throws {
        let go = try stmts(#"""
        [["call", "demo.checkPromo", ["v", {"code": "WB10"}], "r", true],
         ["set", "s", "discount", [], ["op", "??", ["get", ["get", ["l", "r"], "value"], "discount"], ["v", 0]]]]
        """#)
        let interpreter = Interpreter(handlers: ["check": Handler(params: [], body: go)])
        let scope = MapScope(state: ["discount": .number(0)])
        let sink = RecordingEffectSink()
        sink.calls["demo.checkPromo"] = .record(["ok": .bool(true), "value": .record(["discount": .number(10)])])
        await interpreter.run(handler: "check", args: [], scope: scope, effects: sink)
        #expect(scope.state("discount") == .number(10))
        #expect(sink.effects == [.record(["effect": .string("call"), "op": .string("demo.checkPromo"), "args": .record(["code": .string("WB10")])])])
    }

    @Test func callFailureUsesUnmockedNetworkError() async throws {
        let go = try stmts(#"""
        [["call", "demo.checkPromo", ["v", {}], "r", true],
         ["set", "s", "msg", [], ["get", ["get", ["l", "r"], "error"], "message"]]]
        """#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let scope = MapScope(state: ["msg": .string("")])
        let sink = RecordingEffectSink() // no mock registered
        await interpreter.run(handler: "go", args: [], scope: scope, effects: sink)
        #expect(scope.state("msg") == .string("no mock"))
    }

    @Test func runCallsAnotherHandlerAndWaitsForIt() async throws {
        // handler a() { log = log + "a"; b(); log = log + "c" }  handler b() { log = log + "b" }
        let a = try stmts(#"""
        [["set", "s", "log", [], ["op", "+", ["s", "log"], ["v", "a"]]],
         ["run", "b", []],
         ["set", "s", "log", [], ["op", "+", ["s", "log"], ["v", "c"]]]]
        """#)
        let b = try stmts(#"[["set", "s", "log", [], ["op", "+", ["s", "log"], ["v", "b"]]]]"#)
        let interpreter = Interpreter(handlers: ["a": Handler(params: [], body: a), "b": Handler(params: [], body: b)])
        let scope = MapScope(state: ["log": .string("")])
        await interpreter.run(handler: "a", args: [], scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("log") == .string("abc"))
    }

    @Test func animateSetsTransactionForItsBodyOnly() async throws {
        let go = try stmts(#"""
        [["anim", ["t", "motion", "snappy"], [["set", "s", "x", [], ["v", 1]]]],
         ["set", "s", "y", [], ["v", 2]]]
        """#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let scope = MapScope(state: ["x": .number(0), "y": .number(0)])
        await interpreter.run(handler: "go", args: [], scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("x") == .number(1))
        #expect(scope.state("y") == .number(2))
        #expect(scope.setCalls[0].transaction == .record(["$motion": .string("snappy")]))
        #expect(scope.setCalls[1].transaction == nil)
    }

    @Test func computedIsReEvaluatedFreshOnEachRead() async throws {
        // state a=1; let b=a*2; state seen=0; handler go() { a=5; seen=b }
        let go = try stmts(#"""
        [["set", "s", "a", [], ["v", 5]],
         ["set", "s", "seen", [], ["c", "b"]]]
        """#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let scope = MapScope(
            state: ["a": .number(1), "seen": .number(0)],
            computed: ["b": try! Expr(json: #"["op", "*", ["s", "a"], ["v", 2]]"#)]
        )
        await interpreter.run(handler: "go", args: [], scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("a") == .number(5))
        #expect(scope.state("seen") == .number(10)) // fresh read sees a=5, not stale a=1
    }

    @Test func letBindsAnImmutableSnapshotNotAnAliasIntoState() async throws {
        // let before = items; items[0].done = true; snap = before[0].done ?? true
        let go = try stmts(#"""
        [["let", "before", ["s", "items"]],
         ["set", "s", "items", [["v", 0], ["v", "done"]], ["v", true]],
         ["set", "s", "snap", [], ["op", "??", ["get", ["at", ["l", "before"], ["v", 0]], "done"], ["v", true]]]]
        """#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let scope = MapScope(state: ["items": .list([.record(["done": .bool(false)])]), "snap": .bool(true)])
        await interpreter.run(handler: "go", args: [], scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("items") == .list([.record(["done": .bool(true)])]))
        #expect(scope.state("snap") == .bool(false)) // `before` still sees the old, unmutated value
    }

    @Test func unknownStatementAbortsRunButKeepsPriorWrites() async throws {
        let go: [Stmt] = [
            .set(scope: "s", name: "n", path: [], value: .literal(.number(1))),
            .unknown("madeUpTag"),
            .set(scope: "s", name: "n", path: [], value: .literal(.number(999))),
        ]
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let scope = MapScope(state: ["n": .number(0)])
        let sink = RecordingEffectSink()
        await interpreter.run(handler: "go", args: [], scope: scope, effects: sink)
        #expect(scope.state("n") == .number(1)) // write before the error stays; the one after never runs
        #expect(sink.errors.count == 1)
    }

    @Test func miscEffectsRecordBareEffectNames() async throws {
        let go = try stmts(#"""
        [["do", "refresh", [["v", "me"]], false],
         ["do", "host", [["v", "openProfile"], ["v", {"id": 5}]], false],
         ["do", "present", [["v", "sheet.x"], ["v", {"a": 1}]], false],
         ["do", "dismiss", [], false],
         ["do", "back", [], false]]
        """#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let sink = RecordingEffectSink()
        await interpreter.run(handler: "go", args: [], scope: MapScope(), effects: sink)
        #expect(sink.effects == [
            .record(["effect": .string("refresh"), "args": .list([.string("me")])]),
            .record(["effect": .string("host"), "args": .list([.string("openProfile"), .record(["id": .number(5)])])]),
            .record(["effect": .string("present"), "args": .list([.string("sheet.x"), .record(["a": .number(1)])])]),
            .record(["effect": .string("dismiss"), "args": .list([])]),
            .record(["effect": .string("back"), "args": .list([])]),
        ])
    }

    @Test func timelineEffectsRecordBareNames() async throws {
        let go = try stmts(#"""
        [["do", "play", [["v", "intro"]], false],
         ["do", "wait", [["v", 300]], true],
         ["do", "play", [["v", "intro"]], true],
         ["do", "stop", [["v", "intro"]], false]]
        """#)
        let interpreter = Interpreter(handlers: ["go": Handler(params: [], body: go)])
        let sink = RecordingEffectSink()
        await interpreter.run(handler: "go", args: [], scope: MapScope(), effects: sink)
        #expect(sink.effects == [
            .record(["effect": .string("play"), "args": .list([.string("intro")])]),
            .record(["effect": .string("wait"), "args": .list([.number(300)])]),
            .record(["effect": .string("play"), "args": .list([.string("intro")])]),
            .record(["effect": .string("stop"), "args": .list([.string("intro")])]),
        ])
    }
}
