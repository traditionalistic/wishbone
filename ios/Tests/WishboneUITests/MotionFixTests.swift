import Foundation
import Testing
@testable import WishboneUI

/// 9b review fixes that are plain logic.
@MainActor
@Suite struct MotionFixTests {
    private func document(_ timelines: String, handler: String = "[]") throws -> Document {
        try Document(json: #"{"format": 1, "screen": "s", "root": {"t": "stack", "id": "root", "on": {"tap": "# + handler + #"}}, "timelines": "# + timelines + "}")
    }

    private let staggered = #"{"in": {"duration": 200, "repeat": 1, "autoplay": true, "tracks": [{"target": "item", "prop": "opacity", "stagger": 200, "keys": [{"t": 0, "v": 0}, {"t": 200, "v": 1}]}]}}"#

    @Test func staggeredAutoplayRunsUntilTheLastInstanceFinished() async throws {
        let start = Date()
        let tl = Timelines(try document(staggered).timelines, now: start)
        try await Task.sleep(for: .milliseconds(20))      // the first render comes after autoplay started…
        for i in 0..<3 { tl.register("item", index: i) } // …and only then are the 3 instances known
        #expect(abs(tl.remaining("in", now: start + 0.35)! - 250) < 1) // ends at D + 2·stagger = 600 ms
        try await Task.sleep(for: .milliseconds(330))     // ~350 ms: past D (200), well before 600
        #expect(tl.isRunning("item") || Date().timeIntervalSince(start) > 0.58)
        try await Task.sleep(for: .milliseconds(600))
        #expect(!tl.isRunning("item"))
    }

    @Test func aNewRevisionKeepsTheStartOfTimelinesThatStillExist() throws {
        let t0 = Date(timeIntervalSince1970: 1000)
        let old = Timelines(try document(staggered).timelines, now: t0)
        let new = Timelines(try document(staggered).timelines, now: t0 + 5, previous: old)
        #expect(new.started["in"] == t0) // no autoplay replay
        let other = Timelines(try document(#"{"x": {"duration": 10, "repeat": 1, "autoplay": true, "tracks": [{"target": "a", "prop": "scale", "keys": [{"t": 0, "v": 0}, {"t": 10, "v": 1}]}]}}"#).timelines,
                              now: t0 + 5, previous: old)
        #expect(other.started["x"] == t0 + 5 && other.started["in"] == nil)
    }

    @Test func aSecondDragContinuesFromTheRestingOffset() throws {
        let spec = try #require(DragSpec(try Value(json: #"{"axis": "x", "snap": [-420, 0, 420], "bounds": [-420, 420]}"#)))
        let rested = CGSize(width: spec.rest(300), height: 0)                      // first drag snapped to 420
        #expect(spec.follow(CGSize(width: -50, height: 12), from: rested) == CGSize(width: 370, height: 0))
        #expect(spec.follow(CGSize(width: 90, height: 0), from: rested).width == 420) // bounds still apply
        #expect(spec.rest(spec.follow(CGSize(width: -300, height: 0), from: rested).width) == 0)
    }

    @Test func discardingTheScreenCancelsAnEndlessAwaitPlay() async throws {
        let doc = try document(#"{"spin": {"duration": 100, "repeat": -1, "tracks": [{"target": "root", "prop": "rotation", "keys": [{"t": 0, "v": 0}, {"t": 100, "v": 360}]}]}}"#,
                               handler: #"[["do", "play", [["v", "spin"]], true]]"#)
        let bag = TaskBag()
        let store = Store(document: doc, params: [:], data: [:], wishbone: testWishbone(), tasks: bag)
        let run = store.fire(doc.root.on["tap"]!, ctx: RenderContext(store: store), event: .record([:]))
        try await Task.sleep(for: .milliseconds(50))
        #expect(bag.count == 1) // still awaiting the infinite timeline
        bag.cancelAll()
        await run.value         // returns once cancelled
        #expect(bag.count == 0)
    }

    @Test func awaitPlayReturnsAtOnceUnderReduceMotion() async throws {
        let doc = try document(#"{"slow": {"duration": 60000, "repeat": 1, "tracks": [{"target": "root", "prop": "opacity", "keys": [{"t": 0, "v": 0}, {"t": 60000, "v": 1}]}]}}"#,
                               handler: #"[["do", "play", [["v", "slow"]], true]]"#)
        let wb = testWishbone()
        wb.env.reduceMotion = true
        let store = Store(document: doc, params: [:], data: [:], wishbone: wb)
        let started = Date()
        await store.fire(doc.root.on["tap"]!, ctx: RenderContext(store: store), event: .record([:])).value
        #expect(Date().timeIntervalSince(started) < 1)
    }
}

@MainActor
@Suite struct HandlerLifetimeTests {
    private func answer(_ revision: Int, handler: String) throws -> ScreenResponse {
        try API.parseScreen(Data(#"""
        {"etag": "e", "revision": \#(revision), "data": {}, "document": {"format": 1, "screen": "s", "revision": \#(revision),
         "root": {"t": "stack", "id": "root", "on": {"tap": \#(handler)}}}}
        """#.utf8))
    }

    @Test func aHandlerSurvivesADocumentReplaceAndIsCancelledOnTeardown() async throws {
        let nav = Navigator()
        var model: ScreenModel? = ScreenModel(wishbone: testWishbone(), route: Route(screen: "s", params: [:]), navigator: nav)
        let awaitCall = #"[["call", "demo.x", ["v", {}], "r", true], ["do", "navigate", [["v", "next"]], false]]"#
        model?.apply(try answer(1, handler: awaitCall))
        let v1 = try #require(model?.store)
        let run = v1.fire(v1.document.root.on["tap"]!, ctx: RenderContext(store: v1), event: .record([:]))
        model?.apply(try answer(2, handler: "[]"))          // live-preview push / SWR refresh mid-call
        #expect(model?.store !== v1)
        await run.value
        #expect(nav.path.map(\.screen) == ["next"])        // the trailing effect still ran

        let longWait = #"[["do", "wait", [["v", 60000]], true], ["do", "navigate", [["v", "late"]], false]]"#
        model?.apply(try answer(3, handler: longWait))
        let v3 = try #require(model?.store)
        let pending = v3.fire(v3.document.root.on["tap"]!, ctx: RenderContext(store: v3), event: .record([:]))
        try await Task.sleep(for: .milliseconds(30))
        model = nil                                         // screen torn down
        await pending.value
        #expect(nav.path.map(\.screen) == ["next"])        // cancelled: no late navigate
    }
}
