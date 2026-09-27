import SwiftUI
import Testing
@testable import WishboneUI

@Suite struct MotionTests {
    private func motion(_ name: String) -> Value { .record(["$motion": .string(name)]) }

    @Test func motionTokensMapToSwiftUIAnimations() {
        let t = Theme.bundledKorpu
        #expect(t.animation(motion("snappy")) == .spring(response: 0.18, dampingFraction: 0.86))
        #expect(t.animation(motion("smooth")) == .timingCurve(0.38, 0.7, 0.125, 1.0, duration: 0.3))
        let spec: Value = .record(["$motion": .record(["kind": .string("curve"), "duration": .number(200), "bezier": .list([0, 0, 1, 1].map(Value.number))])])
        #expect(t.animation(spec) == .timingCurve(0, 0, 1, 1, duration: 0.2))
        #expect(t.animation(motion("nope")) == nil)
        #expect(t.animation(.null) == nil)
    }

    @Test func implicitSpecsFallBackToStar() {
        let m = Motion.Implicit(specs: ["opacity": .linear, "*": .easeIn])
        #expect(m["opacity"] == .linear)
        #expect(m["scale"] == .easeIn)
        #expect(m.any("width", "height") == .easeIn)
        #expect(Motion.Implicit(specs: ["height": .linear]).any("width", "height") == .linear)
        #expect(Motion.Implicit()["opacity"] == nil)
    }

    @Test func transitionPartsFromWire() throws {
        let wire = try Value(json: #"[{"k": "fade"}, {"k": "slide", "edge": "end"}, {"k": "scale", "from": 0.6}, {"k": "offset", "x": 0, "y": 8}, {"k": "wobble"}]"#)
        #expect(TransitionPart.parts(wire) == [.fade, .slide(.trailing), .scale(0.6), .offset(0, 8)])
        #expect(TransitionPart.parts(try Value(json: #"[{"k": "slide", "edge": "start"}, {"k": "slide", "edge": "top"}, {"k": "slide"}]"#))
                == [.slide(.leading), .slide(.top), .slide(.bottom)])
        #expect(TransitionPart.parts(nil).isEmpty)
        // reduce motion: anything becomes a single fade; nothing stays nothing
        #expect(TransitionPart.reduced([.slide(.trailing), .fade]) == [.fade])
        #expect(TransitionPart.reduced([]) == [])
    }

    @Test func dragFollowsAxisWithinBoundsAndRestsOnNearestSnap() throws {
        let spec = try #require(DragSpec(try Value(json: #"{"axis": "x", "snap": [-420, 0, 420], "bounds": [-300, 500]}"#)))
        #expect(spec.follow(CGSize(width: -400, height: 30)) == CGSize(width: -300, height: 0))
        #expect(spec.rest(100) == 0)
        #expect(spec.rest(250) == 420)
        #expect(spec.rest(-211) == -420)
        #expect(spec.claims(CGSize(width: 12, height: 5)) && !spec.claims(CGSize(width: 3, height: -9)))
        let free = try #require(DragSpec(try Value(json: #"{"axis": "both"}"#)))
        #expect(free.follow(CGSize(width: 7, height: -9)) == CGSize(width: 7, height: -9))
        #expect(free.rest(123) == 0 && free.claims(CGSize(width: 0, height: 9)))
        #expect(DragSpec(.null) == nil)
    }

    @Test func timelineValuesComposeOntoTheTransformLayer() {
        var s = TimelineSample()
        s.compose("opacity", 0.5); s.compose("opacity", 0.5)
        s.compose("scale", 2); s.compose("offsetX", 10); s.compose("offsetX", 5)
        s.compose("rotation", 90); s.compose("offsetY", -4); s.compose("width", 99)
        #expect(s == TimelineSample(opacity: 0.25, scale: 2, offsetX: 15, offsetY: -4, rotation: 90))
    }

    @Test func scrollOffsetIsMeasuredFromTheRestingPosition() {
        let insets = EdgeInsets(top: 62, leading: 0, bottom: 34, trailing: 0)
        #expect(ScrollDriver.offset(contentOffset: CGPoint(x: 0, y: -62), insets: insets) == .zero)
        #expect(ScrollDriver.offset(contentOffset: CGPoint(x: 0, y: 38), insets: insets) == CGPoint(x: 0, y: 100))
        #expect(ScrollDriver.offset(contentOffset: CGPoint(x: 0, y: -100), insets: insets).y == -38) // overscroll
    }

    @Test func aspectDerivesTheMissingDimension() {
        func frame(_ w: Dim, _ h: Dim, _ a: CGFloat) -> BoxFrame {
            BoxFrame(width: w, height: h, aspect: a, alignment: .center)
        }
        let offer = ProposedViewSize(width: 300, height: 800)
        #expect(frame(.fixed(100), .wrap, 2).aspectSize(offer) == CGSize(width: 100, height: 50))
        #expect(frame(.fill, .wrap, 1.5).aspectSize(offer) == CGSize(width: 300, height: 200))
        #expect(frame(.wrap, .fixed(40), 2).aspectSize(offer) == CGSize(width: 80, height: 40))
        #expect(frame(.wrap, .wrap, 2).aspectSize(ProposedViewSize(width: 300, height: 100)) == CGSize(width: 200, height: 100))
        #expect(frame(.fixed(10), .fixed(10), 2).aspectSize(offer) == nil) // both given: aspect ignored
        #expect(frame(.wrap, .wrap, 2).aspectSize(.unspecified) == nil)
    }
}

@MainActor
@Suite struct TimelineRegistryTests {
    private func timeline(_ json: String) throws -> Document {
        try Document(json: #"{"format": 1, "screen": "s", "root": {"t": "stack", "id": "root"}, "timelines": "# + json + "}")
    }

    private let fadeIn = #"""
    {"intro": {"duration": 400, "repeat": 1, "autoplay": true, "tracks": [
       {"target": "hero", "prop": "opacity", "stagger": 100, "keys": [{"t": 0, "v": 0}, {"t": 400, "v": 1}]},
       {"target": "hero", "prop": "offsetY", "keys": [{"t": 0, "v": 20}, {"t": 400, "v": 0}]}]},
     "spin": {"duration": 1000, "repeat": -1, "tracks": [{"target": "hero", "prop": "rotation", "keys": [{"t": 0, "v": 0}, {"t": 1000, "v": 360}]}]},
     "bump": {"duration": 200, "repeat": 1, "tracks": [{"target": "pill", "prop": "scale", "keys": [{"t": 0, "v": 1}, {"t": 100, "v": 1.5}, {"t": 200, "v": 1}]}]}}
    """#

    @Test func autoplayPlayStopAndSampling() throws {
        let t0 = Date(timeIntervalSince1970: 1000)
        let tl = Timelines(try timeline(fadeIn).timelines, now: t0)
        #expect(tl.started["intro"] == t0 && tl.running == ["intro"])     // autoplay from the first render
        #expect(tl.isTarget("hero") && tl.isTarget("pill") && !tl.isTarget("root"))
        #expect(tl.sample("hero", index: 0, at: t0) == TimelineSample(opacity: 0, offsetY: 20))
        let mid = tl.sample("hero", index: 0, at: t0 + 0.2) // Date arithmetic: compare with a tolerance
        #expect(abs(mid.opacity - 0.5) < 1e-5 && abs(mid.offsetY - 10) < 1e-4)
        #expect(abs(tl.sample("hero", index: 1, at: t0 + 0.2).opacity - 0.25) < 1e-5) // stagger by instance index
        #expect(tl.sample("hero", index: 0, at: t0 + 9) == TimelineSample()) // finished: holds the end values
        #expect(tl.sample("pill", index: 0, at: t0 + 0.1) == TimelineSample())  // not played: no effect
        tl.play("bump", at: t0)
        #expect(abs(tl.sample("pill", index: 0, at: t0 + 0.1).scale - 1.5) < 1e-5)
        tl.play("spin", at: t0)
        #expect(tl.isRunning("hero") && tl.sample("hero", index: 0, at: t0 + 9.25).rotation == 90) // + intro's end state
        tl.stop("spin")
        tl.stop("intro")
        #expect(tl.sample("hero", index: 0, at: t0 + 9.25) == TimelineSample() && !tl.isRunning("hero"))
    }

    @Test func durationCountsMaterializedInstances() throws {
        let tl = Timelines(try timeline(fadeIn).timelines, now: Date())
        #expect(tl.duration("intro") == 400)
        for i in 0..<3 { tl.register("hero", index: i) }
        tl.register("hero", index: 1)
        #expect(tl.duration("intro") == 600) // 400 + 100·(3−1)
        #expect(tl.duration("spin") == .infinity)
        #expect(tl.duration("nope") == 0)
    }

    @Test func reduceMotionRendersFinalStateAndNeverTicks() throws {
        let tl = Timelines(try timeline(fadeIn).timelines, now: Date(), reduceMotion: { true })
        #expect(tl.running.isEmpty && tl.started["intro"] != nil)
        #expect(tl.sample("hero", index: 0, at: Date.distantPast) == TimelineSample())
        tl.play("spin")
        #expect(!tl.isRunning("hero") && tl.sample("hero", index: 0, at: Date()).rotation == 360) // one cycle's end
    }

    @Test func finiteTimelineStopsTicking() async throws {
        let tl = Timelines(try timeline(#"{"t": {"duration": 30, "repeat": 1, "tracks": [{"target": "a", "prop": "scale", "keys": [{"t": 0, "v": 0}, {"t": 30, "v": 1}]}]}}"#).timelines)
        tl.play("t")
        #expect(tl.isRunning("a"))
        try await Task.sleep(for: .milliseconds(200))
        #expect(!tl.isRunning("a") && tl.started["t"] != nil) // holds the last value, no more frames
    }
}
