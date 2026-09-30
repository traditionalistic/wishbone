import Foundation
import WishboneCore

/// A `render` job of the preview session (spec 2026-09-30 §3): the document to draw with its data, answered with exactly
/// one `Shot`. Parsed at the `PreviewClient` boundary.
struct RenderJob: Identifiable {
    let id: Int
    let document: Document
    let data: [String: Value]
}

/// The answer to a render job: a JPEG with its size in points and whether the screen had settled (§2.2), or why there is
/// no picture.
enum Shot: Equatable {
    case image(jpeg: Data, size: CGSize, settled: Bool)
    case error(String)

    /// Wire form (§3).
    func message(job: Int) -> [String: Any] {
        switch self {
        case let .image(jpeg, size, settled):
            ["type": "shot", "job": job, "image": jpeg.base64EncodedString(), "width": Double(size.width), "height": Double(size.height),
             "settled": settled]
        case let .error(message):
            ["type": "shot", "job": job, "error": message]
        }
    }
}

/// When a shot is taken (§2.2), one rule for both platforms: never before `floor` seconds — the longest motion token and
/// the platform transitions the counters don't see — then once `quietPolls` polls in a row (one every `poll` seconds) find
/// no image load in flight and no finite timeline running; at `deadline` regardless, unsettled. Infinite timelines
/// (spinners) never hold it up. The three numbers are the calibration knobs.
struct Settle {
    static let floor = 0.7, poll = 0.05, deadline = 6.0
    static let quietPolls = 2

    enum Verdict: Equatable {
        case wait
        case capture(settled: Bool)
    }

    /// Quiet polls in a row so far.
    private(set) var quiet = 0

    mutating func poll(elapsed: Double, imagesInFlight: Int, finiteTimelineRunning: Bool) -> Verdict {
        if elapsed >= Self.deadline { return .capture(settled: false) }
        guard elapsed >= Self.floor else { return .wait }
        quiet = imagesInFlight == 0 && !finiteTimelineRunning ? quiet + 1 : 0
        return quiet >= Self.quietPolls ? .capture(settled: true) : .wait
    }
}
