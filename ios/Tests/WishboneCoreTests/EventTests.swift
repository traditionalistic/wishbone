import Foundation
import Testing
@testable import WishboneCore

/// Hand-written wire AST covering the same semantics as contract/conformance/events.json.
@MainActor
@Suite struct EventTests {
    private func node(_ json: String) throws -> Node { try Node(json: Data(json.utf8)) }

    @Test func changeEventWritesStateThroughEventValue() async throws {
        // textField "field" with on.change: email = event.value
        let root = try node(#"""
        {"t": "stack", "id": "root", "c": [
          {"t": "textField", "id": "field",
           "on": {"change": [["set", "s", "email", [], ["get", ["l", "event"], "value"]]]}}
        ]}
        """#)
        let interpreter = Interpreter()
        let scope = MapScope(state: ["email": .string("")])
        let sink = RecordingEffectSink()
        await interpreter.runEvent("change", root: root, nodeId: "field", payload: .record(["value": .string("a@b.c")]), repeatIndex: nil, scope: scope, effects: sink)
        #expect(scope.state("email") == .string("a@b.c"))
        #expect(sink.errors.isEmpty)
    }

    @Test func changeEventWithExplicitHandlerBodyTrimsAndCounts() async throws {
        let root = try node(#"""
        {"t": "stack", "id": "root", "c": [
          {"t": "textField", "id": "field",
           "on": {"change": [
             ["set", "s", "name", [], ["f", "trim", [["get", ["l", "event"], "value"]]]],
             ["set", "s", "edits", [], ["op", "+", ["s", "edits"], ["v", 1]]]
           ]}}
        ]}
        """#)
        let interpreter = Interpreter()
        let scope = MapScope(state: ["name": .string(""), "edits": .number(0)])
        await interpreter.runEvent("change", root: root, nodeId: "field", payload: .record(["value": .string("  Ann ")]), repeatIndex: nil, scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("name") == .string("Ann"))
        #expect(scope.state("edits") == .number(1))
    }

    @Test func tapEventRunsHandlerWithLiteralArg() async throws {
        let root = try node(#"""
        {"t": "stack", "id": "root", "c": [
          {"t": "text", "id": "btn", "on": {"tap": [["run", "inc", [["v", 3]]]]}}
        ]}
        """#)
        let interpreter = Interpreter(handlers: ["inc": Handler(params: ["by"], body: [.set(scope: "s", name: "n", path: [], value: .binary(op: "+", .state("n"), .local("by")))])])
        let scope = MapScope(state: ["n": .number(0)])
        await interpreter.runEvent("tap", root: root, nodeId: "btn", payload: .record([:]), repeatIndex: nil, scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("n") == .number(3))
    }

    @Test func dragEndReadsEventVelocityAndPosition() async throws {
        let root = try node(#"""
        {"t": "stack", "id": "root", "c": [
          {"t": "shape", "id": "card",
           "on": {"dragEnd": [["if",
             ["op", "||",
               ["op", ">", ["f", "abs", [["get", ["l", "event"], "x"]]], ["v", 120]],
               ["op", ">", ["f", "abs", [["get", ["l", "event"], "vx"]]], ["v", 800]]],
             [["set", "s", "dismissed", [], ["v", true]]], []]]}}
        ]}
        """#)
        let interpreter = Interpreter()

        let bigSwipe = MapScope(state: ["dismissed": .bool(false)])
        await interpreter.runEvent("dragEnd", root: root, nodeId: "card", payload: .record(["x": .number(150), "y": .number(0), "vx": .number(0), "vy": .number(0)]), repeatIndex: nil, scope: bigSwipe, effects: RecordingEffectSink())
        #expect(bigSwipe.state("dismissed") == .bool(true))

        let smallSwipe = MapScope(state: ["dismissed": .bool(false)])
        await interpreter.runEvent("dragEnd", root: root, nodeId: "card", payload: .record(["x": .number(40), "y": .number(0), "vx": .number(100), "vy": .number(0)]), repeatIndex: nil, scope: smallSwipe, effects: RecordingEffectSink())
        #expect(smallSwipe.state("dismissed") == .bool(false))
    }

    @Test func repeatBindsItemAndIndexForTheGivenRepeatIndex() async throws {
        // state plans = [{id:"basic"}, {id:"pro"}]; chip repeats over plans as p; tap: picked = p.id
        let root = try node(#"""
        {"t": "stack", "id": "root", "c": [
          {"t": "text", "id": "chip", "rep": {"over": ["s", "plans"], "as": "p"},
           "on": {"tap": [["set", "s", "picked", [], ["get", ["l", "p"], "id"]]]}}
        ]}
        """#)
        let interpreter = Interpreter()
        let scope = MapScope(state: [
            "picked": .string(""),
            "plans": .list([.record(["id": .string("basic")]), .record(["id": .string("pro")])]),
        ])
        await interpreter.runEvent("tap", root: root, nodeId: "chip", payload: .record([:]), repeatIndex: 1, scope: scope, effects: RecordingEffectSink())
        #expect(scope.state("picked") == .string("pro"))
    }

    @Test func unknownNodeOrEventReportsErrorInsteadOfCrashing() async throws {
        let root = try node(#"{"t": "stack", "id": "root", "c": []}"#)
        let interpreter = Interpreter()
        let sink = RecordingEffectSink()
        await interpreter.runEvent("tap", root: root, nodeId: "missing", payload: .null, repeatIndex: nil, scope: MapScope(), effects: sink)
        #expect(sink.errors.count == 1)
    }
}
