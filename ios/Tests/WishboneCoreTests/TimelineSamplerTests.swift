import Testing
@testable import WishboneCore

/// Mirrors contract/conformance/timelines.json exactly (named eases resolved to their
/// contract/stdlib.json bezier control points, since the wire form only ever carries a
/// 4-number array or no `ease` at all — the compiler expands names before emitting it).
@Suite struct TimelineSamplerTests {
    private let easeIn: [Double] = [0.42, 0, 1, 1]
    private let easeOut: [Double] = [0, 0, 0.58, 1]
    private let easeInOut: [Double] = [0.42, 0, 0.58, 1]
    private let springEase: [Double] = [0.38, 0.7, 0.125, 1.0]

    private func track(_ target: String, _ prop: String, stagger: Double = 0, _ keys: [TimelineKey]) -> TimelineTrack {
        TimelineTrack(target: target, prop: prop, stagger: stagger, keys: keys)
    }

    private func timeline(duration: Double, repeatCount: Int = 1, autoreverse: Bool = false, _ tracks: [TimelineTrack]) -> Timeline {
        Timeline(name: "t", duration: duration, repeatCount: repeatCount, autoreverse: autoreverse, tracks: tracks)
    }

    // Harness rule 4 (contract/README.md): timeline samples compare with a flat 1e-6 tolerance.
    private func approx(_ a: Double, _ b: Double, tol: Double = 1e-6) -> Bool {
        abs(a - b) <= tol
    }

    @Test func linearInterpolationHoldsBeforeAndAfter() {
        let tl = timeline(duration: 400, [track("a", "opacity", [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 400, v: 1, ease: nil),
        ])])
        #expect(TimelineSampler.sample(tl, at: 0, index: 0)["a.opacity"] == 0)
        #expect(TimelineSampler.sample(tl, at: 100, index: 0)["a.opacity"] == 0.25)
        #expect(TimelineSampler.sample(tl, at: 400, index: 0)["a.opacity"] == 1.0)
        #expect(TimelineSampler.sample(tl, at: 1000, index: 0)["a.opacity"] == 1.0) // holds after end
    }

    @Test func easeOutMatchesReferenceBezierValues() {
        let tl = timeline(duration: 1000, [track("a", "offsetY", [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 1000, v: 100, ease: easeOut),
        ])])
        #expect(approx(TimelineSampler.sample(tl, at: 250, index: 0)["a.offsetY"]!, 37.813813083))
        #expect(approx(TimelineSampler.sample(tl, at: 500, index: 0)["a.offsetY"]!, 68.464318743))
        #expect(approx(TimelineSampler.sample(tl, at: 750, index: 0)["a.offsetY"]!, 90.653534928))
    }

    @Test func easeInThenEaseInOutAcrossThreeKeys() {
        let tl = timeline(duration: 200, [track("a", "scale", [
            TimelineKey(t: 0, v: 0, ease: nil),
            TimelineKey(t: 100, v: 1, ease: easeIn),
            TimelineKey(t: 200, v: 0, ease: easeInOut),
        ])])
        #expect(approx(TimelineSampler.sample(tl, at: 50, index: 0)["a.scale"]!, 0.315356813))
        #expect(approx(TimelineSampler.sample(tl, at: 150, index: 0)["a.scale"]!, 0.5))
        #expect(approx(TimelineSampler.sample(tl, at: 175, index: 0)["a.scale"]!, 0.129161931))
    }

    @Test func springEaseMatchesReferenceValues() {
        let tl = timeline(duration: 200, [track("a", "offsetY", [
            TimelineKey(t: 0, v: 40, ease: nil), TimelineKey(t: 200, v: 0, ease: springEase),
        ])])
        #expect(approx(TimelineSampler.sample(tl, at: 50, index: 0)["a.offsetY"]!, 15.333559586))
        #expect(approx(TimelineSampler.sample(tl, at: 100, index: 0)["a.offsetY"]!, 2.7664857))
        #expect(approx(TimelineSampler.sample(tl, at: 199, index: 0)["a.offsetY"]!, 0.000131286))
    }

    @Test func customBezierControlPoints() {
        let tl = timeline(duration: 1000, [track("a", "rotation", [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 1000, v: 90, ease: [0.25, 0.1, 0.25, 1]),
        ])])
        #expect(approx(TimelineSampler.sample(tl, at: 300, index: 0)["a.rotation"]!, 46.198364488))
        #expect(approx(TimelineSampler.sample(tl, at: 500, index: 0)["a.rotation"]!, 72.216304883))
    }

    @Test func staggerOffsetsEachRepeatInstance() {
        let tl = timeline(duration: 200, [track("item", "opacity", stagger: 100, [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 200, v: 1, ease: nil),
        ])])
        #expect(TimelineSampler.sample(tl, at: 150, index: 0)["item.opacity"] == 0.75)
        #expect(TimelineSampler.sample(tl, at: 150, index: 1)["item.opacity"] == 0.25)
        #expect(TimelineSampler.sample(tl, at: 150, index: 2)["item.opacity"] == 0)
        #expect(TimelineSampler.sample(tl, at: 500, index: 2)["item.opacity"] == 1.0)
    }

    @Test func repeatWithAutoreverseClampsAtTheEnd() {
        let tl = timeline(duration: 100, repeatCount: 2, autoreverse: true, [track("a", "offsetX", [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 100, v: 10, ease: nil),
        ])])
        #expect(TimelineSampler.sample(tl, at: 50, index: 0)["a.offsetX"] == 5.0)
        #expect(TimelineSampler.sample(tl, at: 130, index: 0)["a.offsetX"] == 7.0)
        #expect(TimelineSampler.sample(tl, at: 250, index: 0)["a.offsetX"] == 0)
    }

    @Test func infiniteRepeatNeverClamps() {
        let tl = timeline(duration: 100, repeatCount: -1, [track("a", "rotation", [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 100, v: 360, ease: nil),
        ])])
        #expect(TimelineSampler.sample(tl, at: 1050, index: 0)["a.rotation"] == 180.0)
        #expect(approx(TimelineSampler.sample(tl, at: 99, index: 0)["a.rotation"]!, 356.4))
    }

    @Test func multipleTracksSampleIndependently() {
        let tl = timeline(duration: 400, [
            track("a", "opacity", [TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 400, v: 1, ease: nil)]),
            track("a", "offsetY", [TimelineKey(t: 0, v: 40, ease: nil), TimelineKey(t: 200, v: 0, ease: springEase)]),
        ])
        let s1 = TimelineSampler.sample(tl, at: 100, index: 0)
        #expect(s1["a.opacity"] == 0.25)
        #expect(approx(s1["a.offsetY"]!, 2.7664857))
        let s2 = TimelineSampler.sample(tl, at: 300, index: 0)
        #expect(s2["a.opacity"] == 0.75)
        #expect(s2["a.offsetY"] == 0)
    }

    @Test func lateStartHoldsFirstKeyValueBeforeItsTime() {
        let tl = timeline(duration: 200, [track("a", "scale", [
            TimelineKey(t: 100, v: 5, ease: nil), TimelineKey(t: 200, v: 10, ease: nil),
        ])])
        #expect(TimelineSampler.sample(tl, at: 50, index: 0)["a.scale"] == 5)
        #expect(TimelineSampler.sample(tl, at: 150, index: 0)["a.scale"] == 7.5)
    }

    @Test func zeroDurationHoldsTheLastKeyValue() {
        let tl = timeline(duration: 0, [track("a", "opacity", [TimelineKey(t: 0, v: 3, ease: nil)])])
        #expect(TimelineSampler.sample(tl, at: 0, index: 0)["a.opacity"] == 3)
        #expect(TimelineSampler.sample(tl, at: 500, index: 0)["a.opacity"] == 3)
    }

    @Test func equalKeyTimesStepToTheLaterKey() {
        let tl = timeline(duration: 100, [track("a", "opacity", [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 50, v: 1, ease: nil), TimelineKey(t: 50, v: 5, ease: nil),
        ])])
        #expect(TimelineSampler.sample(tl, at: 50, index: 0)["a.opacity"] == 5)
    }

    @Test func interiorDuplicateKeyTimeStepsToTheLaterKey() {
        // The duplicate pair (50, 1) / (50, 5) sits in the *interior* of the key list, not at
        // the end — a naive first-matching-segment scan would land on [0,50]→1 (the earlier
        // key), not the later duplicate's value.
        let tl = timeline(duration: 100, [track("a", "opacity", [
            TimelineKey(t: 0, v: 0, ease: nil),
            TimelineKey(t: 50, v: 1, ease: nil),
            TimelineKey(t: 50, v: 5, ease: nil),
            TimelineKey(t: 100, v: 10, ease: nil),
        ])])
        #expect(TimelineSampler.sample(tl, at: 50, index: 0)["a.opacity"] == 5)
        // And the segment leading out of the later duplicate still interpolates normally.
        #expect(TimelineSampler.sample(tl, at: 75, index: 0)["a.opacity"] == 7.5)
    }

    /// The per-track entry point (what the renderer calls per frame) returns exactly what
    /// `sample` puts under that track's key — finite/autoreverse, infinite and zero-length.
    @Test func sampleTrackEqualsTheSampleEntry() {
        let keys = [TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 400, v: 1, ease: easeInOut)]
        let timelines = [
            timeline(duration: 400, repeatCount: 3, autoreverse: true, [
                track("a", "opacity", stagger: 60, keys),
                track("a", "offsetY", stagger: 60, [TimelineKey(t: 0, v: 40, ease: nil), TimelineKey(t: 200, v: 0, ease: springEase)]),
                track("b", "scale", [TimelineKey(t: 100, v: 0.5, ease: nil), TimelineKey(t: 300, v: 1, ease: easeOut)]),
            ]),
            timeline(duration: 400, repeatCount: -1, [track("a", "rotation", stagger: 25, keys)]),
            timeline(duration: 0, [track("a", "opacity", keys)]),
        ]
        for tl in timelines {
            for ms in stride(from: -100.0, through: 2000, by: 37) {
                for index in 0..<3 {
                    let all = TimelineSampler.sample(tl, at: ms, index: index)
                    for tr in tl.tracks {
                        #expect(TimelineSampler.sampleTrack(tl, track: tr, ms: ms, index: index) == all["\(tr.target).\(tr.prop)"])
                    }
                }
            }
        }
    }

    @Test func durationAccountsForRepeatAndStagger() {
        let tl = timeline(duration: 200, repeatCount: 2, [track("item", "opacity", stagger: 100, [
            TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 200, v: 1, ease: nil),
        ])])
        #expect(TimelineSampler.duration(tl, instanceCount: 3) == 200 * 2 + 100 * 2)
        let infinite = timeline(duration: 200, repeatCount: -1, [track("a", "opacity", [TimelineKey(t: 0, v: 0, ease: nil), TimelineKey(t: 200, v: 1, ease: nil)])])
        #expect(TimelineSampler.duration(infinite, instanceCount: 1) == .infinity)
    }
}
