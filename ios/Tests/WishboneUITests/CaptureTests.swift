import Foundation
import Testing
@testable import WishboneUI

/// The "settled" rule of a shot (spec 2026-09-30 §2.2) and what feeds it.
@Suite struct CaptureTests {
    @Test func waitsTheFloorThenTwoQuietPolls() {
        var s = Settle()
        #expect(s.poll(elapsed: 0.3, imagesInFlight: 0, finiteTimelineRunning: false) == .wait) // inside the floor
        #expect(s.poll(elapsed: 0.7, imagesInFlight: 0, finiteTimelineRunning: false) == .wait) // first quiet poll
        #expect(s.poll(elapsed: 0.75, imagesInFlight: 0, finiteTimelineRunning: false) == .capture(settled: true))
    }

    @Test func activityRestartsTheQuietStreak() {
        var s = Settle()
        #expect(s.poll(elapsed: 0.7, imagesInFlight: 0, finiteTimelineRunning: false) == .wait)
        #expect(s.poll(elapsed: 0.75, imagesInFlight: 1, finiteTimelineRunning: false) == .wait) // a load: streak reset
        #expect(s.poll(elapsed: 0.8, imagesInFlight: 0, finiteTimelineRunning: false) == .wait)
        #expect(s.poll(elapsed: 0.85, imagesInFlight: 0, finiteTimelineRunning: true) == .wait) // a timeline: reset again
        #expect(s.poll(elapsed: 0.9, imagesInFlight: 0, finiteTimelineRunning: false) == .wait)
        #expect(s.poll(elapsed: 0.95, imagesInFlight: 0, finiteTimelineRunning: false) == .capture(settled: true))
    }

    @Test func deadlineCapturesUnsettled() {
        var s = Settle()
        #expect(s.poll(elapsed: 5.95, imagesInFlight: 2, finiteTimelineRunning: true) == .wait)
        #expect(s.poll(elapsed: 6, imagesInFlight: 2, finiteTimelineRunning: true) == .capture(settled: false))
    }

    /// Only finite timelines hold a shot: a spinner (`repeat: -1`) runs for the screen's whole life.
    @MainActor @Test func finiteTimelinesRunningIgnoresSpinners() throws {
        let doc = try Document(json: #"""
        {"format": 1, "screen": "s", "root": {"t": "stack", "id": "root"}, "timelines": {
          "spin": {"duration": 1000, "repeat": -1, "autoplay": true, "tracks": [{"target": "a", "prop": "rotation", "keys": [{"t": 0, "v": 0}, {"t": 1000, "v": 360}]}]},
          "intro": {"duration": 400, "repeat": 1, "tracks": [{"target": "a", "prop": "opacity", "keys": [{"t": 0, "v": 0}, {"t": 400, "v": 1}]}]}}}
        """#)
        let tl = Timelines(doc.timelines)
        #expect(tl.running == ["spin"] && !tl.finiteRunning)
        tl.play("intro")
        #expect(tl.finiteRunning)
        tl.stop("intro")
        #expect(!tl.finiteRunning)
    }

    @Test func shotWireForm() {
        let image = Shot.image(jpeg: Data([0xFF, 0xD8, 0xFF]), size: CGSize(width: 402, height: 874), settled: true).message(job: 17)
        #expect(image["type"] as? String == "shot" && image["job"] as? Int == 17 && image["image"] as? String == "/9j/")
        #expect(image["width"] as? Double == 402 && image["height"] as? Double == 874 && image["settled"] as? Bool == true)
        let error = Shot.error("app is in the background").message(job: 17)
        #expect(error["job"] as? Int == 17 && error["error"] as? String == "app is in the background" && error["image"] == nil)
    }
}
