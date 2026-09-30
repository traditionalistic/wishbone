import Foundation
import Observation
import WishboneCore

/// Active timelines of one screen: `{name → start}` (renderer-decisions: Timelines). `running` drives
/// `TimelineView` (frames only while something moves); a finished timeline stays started and holds
/// its final values until `stop`.
@MainActor @Observable
final class Timelines {
    private(set) var started: [String: Date] = [:]
    private(set) var running: Set<String> = []

    @ObservationIgnored let timelines: [String: Timeline]
    /// Target node id → names of the timelines animating it.
    @ObservationIgnored let byTarget: [String: [String]]
    /// Materialized instance indices per target (for `await play` and stagger duration).
    @ObservationIgnored private var instances: [String: Set<Int>] = [:]
    @ObservationIgnored private var tokens: [String: UUID] = [:]
    @ObservationIgnored let reduceMotion: () -> Bool

    /// `previous` — the registry of the same screen before a document reload: timelines that still
    /// exist keep their start (a new revision doesn't replay the entrance).
    init(_ timelines: [String: Timeline], now: Date = Date(), previous: Timelines? = nil,
         reduceMotion: @escaping () -> Bool = { false }) {
        self.timelines = timelines
        self.reduceMotion = reduceMotion
        var byTarget: [String: [String]] = [:]
        for (name, t) in timelines {
            for target in Set(t.tracks.map(\.target)) { byTarget[target, default: []].append(name) }
        }
        self.byTarget = byTarget.mapValues { $0.sorted() }
        for (name, t) in timelines {
            if let start = previous?.started[name] {
                play(name, at: start)
            } else if t.autoplay {
                play(name, at: now)
            }
        }
    }

    func isTarget(_ id: String) -> Bool { byTarget[id] != nil }

    func register(_ id: String, index: Int) {
        if instances[id]?.contains(index) != true { instances[id, default: []].insert(index) }
    }

    /// `play` restarts; `await play` resumes after `duration(name)`.
    func play(_ name: String, at now: Date = Date()) {
        guard let timeline = timelines[name] else { return }
        started[name] = now
        let token = UUID()
        tokens[name] = token
        guard !reduceMotion() else { running.remove(name); return }
        running.insert(name)
        guard timeline.repeatCount != -1 else { return }
        // Instances materialize after `play` (autoplay runs before the first render), so the end is
        // re-evaluated after every sleep: stop ticking only once every instance has finished.
        Task { [weak self] in
            while let left = self?.pendingEnd(name, token: token) {
                try? await Task.sleep(for: .milliseconds(Int(min(left, 86_400_000)) + 16))
            }
        }
    }

    /// ms still to wait for this run of `name`, or nil (restarted, stopped, or finished — then it
    /// leaves `running`).
    private func pendingEnd(_ name: String, token: UUID) -> Double? {
        guard tokens[name] == token, let left = remaining(name) else { return nil }
        guard left > 0 else {
            running.remove(name)
            return nil
        }
        return left
    }

    /// ms left until every materialized instance of `name` finished (nil when not started).
    func remaining(_ name: String, now: Date = Date()) -> Double? {
        guard let start = started[name] else { return nil }
        return duration(name) - now.timeIntervalSince(start) * 1000
    }

    func stop(_ name: String) {
        started[name] = nil
        running.remove(name)
        tokens[name] = nil
    }

    /// ms until every materialized instance finished (infinite for repeat -1).
    func duration(_ name: String) -> Double {
        guard let t = timelines[name] else { return 0 }
        let count = Set(t.tracks.map(\.target)).map { instances[$0]?.count ?? 1 }.max() ?? 1
        return TimelineSampler.duration(t, instanceCount: max(1, count))
    }

    func isRunning(_ id: String) -> Bool {
        byTarget[id]?.contains { running.contains($0) } ?? false
    }

    /// A finite timeline is playing (a shot waits for it, spec 2026-09-30 §2.2); infinite ones (spinners) don't count.
    var finiteRunning: Bool { running.contains { timelines[$0]?.repeatCount != -1 } }

    /// Composed values of every started timeline targeting `id`, for instance `index` at `date`: only this target's
    /// tracks are sampled (per frame).
    func sample(_ id: String, index: Int, at date: Date) -> TimelineSample {
        var s = TimelineSample()
        guard let names = byTarget[id] else { return s }
        let reduce = reduceMotion()
        for name in names {
            guard let start = started[name], let t = timelines[name] else { continue }
            // Reduce motion: the final state, the end of the last cycle (an infinite timeline ends one cycle).
            let timeline = reduce ? Self.once(t) : t
            let ms = reduce ? 1e15 : date.timeIntervalSince(start) * 1000
            for track in t.tracks where track.target == id {
                s.compose(track.prop, TimelineSampler.sampleTrack(timeline, track: track, ms: ms, index: index))
            }
        }
        return s
    }

    /// `t` with an infinite repeat cut to one cycle.
    static func once(_ t: Timeline) -> Timeline {
        guard t.repeatCount == -1 else { return t }
        return Timeline(name: t.name, duration: t.duration, repeatCount: 1, autoreverse: t.autoreverse, autoplay: t.autoplay, tracks: t.tracks)
    }
}
