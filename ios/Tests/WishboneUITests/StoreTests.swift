import Foundation
import Observation
import Synchronization
import Testing
@testable import WishboneUI

/// Counts `withObservationTracking` change notifications.
final class Changes: Sendable {
    private let count = Mutex(0)
    var value: Int { count.withLock { $0 } }
    @Sendable func bump() { count.withLock { $0 += 1 } }
}

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

    /// An unchanged repeat item keeps its `Locals`, so its row's inputs compare equal and SwiftUI skips it (a page append
    /// doesn't re-render every row); a changed value, a moved item and a removed key don't.
    @Test func repeatItemsKeepTheLocalsOfUnchangedItems() throws {
        let doc = try document(#"""
        {"format": 1, "screen": "s", "state": [["rows", ["v", [{"id": "a", "n": 1}, {"id": "b", "n": 2}, {"id": "c", "n": 3}]]]],
         "root": {"t": "stack", "id": "root", "c": [
            {"t": "text", "id": "row", "rep": {"over": ["s", "rows"], "as": "item", "index": "i", "key": ["get", ["l", "item"], "id"]}}]}}
        """#)
        let store = Store(document: doc, params: [:], data: [:], wishbone: testWishbone())
        let ctx = RenderContext(store: store), node = doc.root.children[0]
        func locals() -> [String: Locals] {
            Dictionary(uniqueKeysWithValues: RepeatItem.items(node.rep!, id: node.id, ctx: ctx).map { ($0.id, $0.ctx.locals!) })
        }
        let first = locals()
        let again = locals()
        #expect(["a", "b", "c"].allSatisfy { first[$0] === again[$0] })

        ctx.set(scope: "s", name: "rows", path: [.number(1), .string("n")], value: .number(20), transaction: nil) // leaderboard refresh
        let refreshed = locals()
        #expect(refreshed["a"] === first["a"] && refreshed["c"] === first["c"])
        #expect(refreshed["b"] !== first["b"] && refreshed["b"]?.value == .record(["id": .string("b"), "n": .number(20)]))

        let rows: Value = .list([.record(["id": .string("c"), "n": .number(3)]), .record(["id": .string("a"), "n": .number(1)])])
        ctx.set(scope: "s", name: "rows", path: [], value: rows, transaction: nil) // reorder + shrink
        let moved = locals()
        #expect(moved.keys.sorted() == ["a", "c"])
        #expect(moved["a"] !== refreshed["a"] && moved["a"]?.parent?.value == .number(1)) // new index
        #expect(moved["c"] !== refreshed["c"] && moved["c"]?.parent?.value == .number(0))
        #expect(store.repeatLocals["row"]?.keys.sorted() == ["a", "c"]) // "b" dropped
        #expect(locals()["c"] === moved["c"])
    }

    /// Computed values are memoised per change of what they read — state, env, scroll drivers, other computed — and a
    /// reader served from the memo still re-renders when one of those changes.
    @Test func computedValuesFollowEverythingTheyRead() throws {
        let doc = try document(#"""
        {"format": 1, "screen": "s", "state": [["early", ["c", "late2"]], ["count", ["v", 1]], ["late", ["v", 3]]],
         "computed": {"next": ["op", "+", ["s", "count"], ["v", 1]], "twice": ["op", "*", ["c", "next"], ["v", 2]],
                      "wide": ["op", ">", ["e", "screenWidth"], ["v", 400]], "y": ["dr", "scroll", "page", "y"],
                      "late2": ["op", "*", ["s", "late"], ["v", 2]]},
         "root": {"t": "stack", "id": "root"}}
        """#)
        let store = Store(document: doc, params: [:], data: [:], wishbone: testWishbone())
        let ctx = RenderContext(store: store)
        #expect(ctx.computed("late2") == .number(6)) // read by a state initializer before `late` existed
        #expect(ctx.computed("next") == .number(2) && ctx.computed("twice") == .number(4))

        let changes = Changes()
        withObservationTracking { _ = ctx.computed("twice") } onChange: { changes.bump() } // a memo hit
        ctx.set(scope: "s", name: "count", path: [], value: .number(5), transaction: nil)
        #expect(changes.value == 1)
        #expect(ctx.computed("next") == .number(6) && ctx.computed("twice") == .number(12))

        #expect(ctx.computed("wide") == .bool(false))
        store.geometry.screenSize = CGSize(width: 402, height: 874)
        #expect(ctx.computed("wide") == .bool(true))

        #expect(ctx.computed("y") == .number(0))
        store.driver("scroll:page:y").value = .number(120)
        #expect(ctx.computed("y") == .number(120))
    }

    /// Token values are memoised per theme: a brand switch re-resolves them (and re-renders their readers); colours
    /// stay named, so the scheme still resolves them at render, light ↔ dark.
    @Test func tokenValuesFollowTheTheme() throws {
        let cache = DiskCache(dir: tempDir())
        var kz = try JSONSerialization.jsonObject(with: contractFile("tokens/kz.json")) as! [String: Any]
        kz["space"] = ["lg": 20]
        let base = URL(string: "http://localhost:1")!
        cache.themes.write(DiskCache.themeKey(scope: Wishbone.cacheScope(base, DefaultHost(), "kz")), try JSONSerialization.data(withJSONObject: kz))
        let wb = Wishbone(baseURL: base, brand: "korpu", host: DefaultHost(), cache: cache)
        let ctx = RenderContext(store: Store(document: try document(docV2), params: [:], data: [:], wishbone: wb))
        #expect(ctx.token("space", "lg") == .number(16))

        let changes = Changes()
        withObservationTracking { _ = ctx.token("space", "lg") } onChange: { changes.bump() } // a memo hit
        wb.brand = "kz"
        #expect(changes.value == 1)
        #expect(ctx.token("space", "lg") == .number(20))

        let el16 = ctx.token("color", "el16")
        #expect(el16 == color("el16"))
        #expect(wb.theme.color(el16, dark: false) == RGBA(hex: "#080808FF") && wb.theme.color(el16, dark: true) == RGBA(hex: "#F5F5F5FF"))
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
