import Foundation
import Testing
@testable import WishboneUI

@MainActor
@Suite struct StoreTests {
    private func document(_ json: String) throws -> Document { try Document(json: json) }

    private let docV1 = #"""
    {"format": 1, "screen": "s", "revision": 1,
     "params": {"id": ["v", "x"], "n": ["v", 1]},
     "state": [["count", ["v", 1]], ["double", ["op", "*", ["s", "count"], ["v", 2]]], ["gone", ["v", true]], ["who", ["p", "id"]]],
     "computed": {"next": ["op", "+", ["s", "count"], ["v", 1]]},
     "data": [{"name": "me", "op": "demo.profile", "args": {}, "required": false}],
     "root": {"t": "stack", "id": "root"}}
    """#

    private let docV2 = #"""
    {"format": 1, "screen": "s", "revision": 2,
     "state": [["count", ["v", 100]], ["fresh", ["op", "+", ["s", "count"], ["v", 1]]]],
     "root": {"t": "stack", "id": "root"}}
    """#

    @Test func stateInitializersRunInOrderAndSeeParams() throws {
        let store = Store(document: try document(docV1), params: ["id": .string("host")], data: [:], wishbone: testWishbone())
        let ctx = RenderContext(store: store)
        #expect(ctx.state("count") == .number(1))
        #expect(ctx.state("double") == .number(2))
        #expect(ctx.state("who") == .string("host"))
        #expect(ctx.param("n") == .number(1)) // default
        #expect(ctx.computed("next") == .number(2))
    }

    @Test func reloadKeepsValuesOfStateKeysThatStillExist() throws {
        let wb = testWishbone()
        let v1 = Store(document: try document(docV1), params: [:], data: [:], wishbone: wb)
        RenderContext(store: v1).set(scope: "s", name: "count", path: [], value: .number(7), transaction: nil)
        let v2 = Store(document: try document(docV2), params: [:], data: [:], wishbone: wb, previous: v1)
        let ctx = RenderContext(store: v2)
        #expect(ctx.state("count") == .number(7))     // kept
        #expect(ctx.state("fresh") == .number(8))     // new key, initializer sees the kept value
        #expect(ctx.state("gone") == .null)           // dropped with the old document
    }

    @Test func pathAssignmentAndComputedOnRead() throws {
        let doc = try document(#"""
        {"format": 1, "screen": "s", "state": [["items", ["v", [{"done": false}, {"done": false}]]]],
         "computed": {"left": ["f", "count", [["f", "filter", [["s", "items"], ["lam", ["i"], ["un", "!", ["get", ["l", "i"], "done"]]]]]]]},
         "root": {"t": "stack", "id": "root"}}
        """#)
        let ctx = RenderContext(store: Store(document: doc, params: [:], data: [:], wishbone: testWishbone()))
        #expect(ctx.computed("left") == .number(2))
        ctx.set(scope: "s", name: "items", path: [.number(1), .string("done")], value: .bool(true), transaction: nil)
        #expect(ctx.computed("left") == .number(1))
        ctx.set(scope: "s", name: "items", path: [.number(5), .string("done")], value: .bool(true), transaction: nil) // no-op
        #expect(ctx.computed("left") == .number(1))
    }

    @Test func dataResultsAndUpdates() throws {
        let store = Store(document: try document(docV1), params: [:], data: ["me": .record(["ok": .bool(true), "value": .string("Ann")])], wishbone: testWishbone())
        #expect(store.dataValue("me") == .string("Ann"))
        store.update(data: ["me": .record(["ok": .bool(false), "error": .record([:])])])
        #expect(store.dataValue("me") == .null)
        #expect(store.dataValue("absent") == .null)
    }

    @Test func instanceStateWalksUpTheChain() throws {
        let store = Store(document: try document(docV2), params: [:], data: [:], wishbone: testWishbone())
        let outer = InstanceScope(parent: nil, values: ["Card.open": .bool(false)])
        let inner = InstanceScope(parent: outer, values: ["Toggle.on": .bool(true)])
        var ctx = RenderContext(store: store)
        ctx.instance = inner
        #expect(ctx.instanceState("Toggle.on") == .bool(true))
        #expect(ctx.instanceState("Card.open") == .bool(false))
        ctx.set(scope: "i", name: "Card.open", path: [], value: .bool(true), transaction: nil)
        #expect(outer.cells["Card.open"]?.value == .bool(true))
        #expect(ctx.instanceState("Nope.x") == .null)
    }

    @Test func instanceStateIsKeyedByNodeAndRepeatPathAndSurvivesReload() throws {
        let wb = testWishbone()
        let node = try Node(json: Data(#"{"t": "stack", "id": "tg", "inst": {"state": {"Toggle.on": ["v", false]}}}"#.utf8))
        let v1 = Store(document: try document(docV2), params: [:], data: [:], wishbone: wb)
        var ctx = RenderContext(store: v1)
        ctx.idSuffix = ".a"
        let a = v1.instance(node, ctx: ctx)
        #expect(v1.instance(node, ctx: ctx) === a)            // same materialized instance on re-render
        ctx.idSuffix = ".b"
        #expect(v1.instance(node, ctx: ctx) !== a)            // another repeat item
        a.cells["Toggle.on"]?.value = .bool(true)
        let v2 = Store(document: try document(docV2), params: [:], data: [:], wishbone: wb, previous: v1)
        ctx = RenderContext(store: v2)
        ctx.idSuffix = ".a"
        #expect(v2.instance(node, ctx: ctx).cells["Toggle.on"]?.value == .bool(true))
    }

    @Test func tokensAndEnv() throws {
        let store = Store(document: try document(docV2), params: [:], data: [:], wishbone: testWishbone())
        let ctx = RenderContext(store: store)
        #expect(ctx.token("space", "lg") == .number(16))
        #expect(ctx.token("radius", "pill") == .number(9999))
        #expect(ctx.token("color", "el16") == color("el16"))
        #expect(ctx.env("platform") == .string("ios"))
        #expect(ctx.env("brand") == .string("korpu"))
        #expect(ctx.env("colorScheme") == .string("light"))
        store.wishbone.env.dark = true
        #expect(ctx.env("colorScheme") == .string("dark"))
    }

    @Test func handlerRunsWriteStateAndPerformEffects() async throws {
        let doc = try document(#"""
        {"format": 1, "screen": "s", "state": [["n", ["v", 0]]],
         "root": {"t": "stack", "id": "root", "on": {"tap": [
            ["set", "s", "n", [], ["op", "+", ["s", "n"], ["v", 1]]],
            ["do", "navigate", [["v", "next"], ["v", {"id": "1"}], ["v", "fade"]], false],
            ["do", "present", [["v", "sheet"]], false]]}}}
        """#)
        let nav = Navigator()
        let store = Store(document: doc, params: [:], data: [:], wishbone: testWishbone(), navigator: nav)
        let ctx = RenderContext(store: store)
        await store.fire(doc.root.on["tap"]!, ctx: ctx, event: .record([:])).value
        #expect(ctx.state("n") == .number(1))
        #expect(nav.path.map(\.screen) == ["next"] && nav.path[0].fade && nav.path[0].params == ["id": .string("1")])
        #expect(nav.sheet?.screen == "sheet")
    }
}
