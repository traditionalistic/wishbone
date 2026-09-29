import Foundation
import Synchronization
import Testing
import WishboneCore
@testable import WishboneUI

/// Effects of handler runs: a `call` waits until the test answers it; other effects are recorded.
final class ScriptedEffects: EffectSink {
    private struct State {
        var pending: [(op: String, reply: CheckedContinuation<Value, Never>)] = []
        var performed: [String] = []
        var errors: [String] = []
    }

    private let state = Mutex(State())

    var performed: [String] { state.withLock { $0.performed } }
    var errors: [String] { state.withLock { $0.errors } }

    func call(op: String, args: Value, awaited: Bool) async -> Value {
        await withCheckedContinuation { reply in state.withLock { $0.pending.append((op, reply)) } }
    }

    func perform(effect: String, args: [Value], awaited: Bool) async {
        state.withLock { $0.performed.append(([effect] + args.map(API.jsonString)).joined(separator: " ")) }
    }

    func error(_ message: String) { state.withLock { $0.errors.append(message) } }

    /// Waits (up to 5 s) until `count` calls of `op` are pending.
    func waitUntilPending(_ op: String, count: Int = 1) async {
        for _ in 0..<5000 {
            if state.withLock({ $0.pending.filter { $0.op == op }.count }) >= count { return }
            try? await Task.sleep(for: .milliseconds(1))
        }
        Issue.record("no \(count) pending \(op)")
    }

    /// Answers the oldest pending call of `op`, once it is made.
    func answer(_ op: String, _ result: Value) async {
        await waitUntilPending(op)
        let reply = state.withLock { s in s.pending.firstIndex { $0.op == op }.map { s.pending.remove(at: $0).reply } }
        reply?.resume(returning: result)
    }
}

/// contract/screens/raffle.main's leaderboard handlers (compiled: contract/conformance/compiled-screens, kept current by
/// the backend's `TestCompiledScreensGolden`), run against a `Store` whose `call`s the test answers by hand.
@MainActor
@Suite struct RaffleMainTests {
    private func ok(_ value: Value) -> Value { .record(["ok": .bool(true), "value": value]) }

    private func page(_ from: Int) -> Value {
        .list((from..<from + 20).map { i in
            .record(["key": .string("u\(i + 1)"), "position": .number(Double(i + 1)), "name": .string("Имя \(i + 1)"),
                     "tickets": .number(Double(999 - i)), "isMe": .bool(false)])
        })
    }

    private func store() throws -> Store {
        let doc = try Document(json: contractFile("conformance/compiled-screens/raffle.main.json"))
        return Store(document: doc, params: [:],
                     data: ["o": ok(.record(["hidden": .bool(false)])), "board": ok(.record(["items": page(0), "total": .number(60)]))],
                     wishbone: testWishbone())
    }

    private func run(_ handler: String, _ store: Store, _ effects: ScriptedEffects) -> Task<Void, Never> {
        Task { await store.interpreter.run(handler: handler, args: [], scope: RenderContext(store: store), effects: effects) }
    }

    private func rows(_ store: Store) -> Int {
        if case let .list(l) = RenderContext(store: store).state("rows") { l.count } else { -1 }
    }

    private func flag(_ store: Store, _ name: String) -> Bool { RenderContext(store: store).state(name) == .bool(true) }

    @Test func hidingTheNameStartsTheLeaderboardOver() async throws {
        let s = try store(), fx = ScriptedEffects()
        let more = run("loadMore", s, fx)
        await fx.answer("raffle.leaderboard", ok(.record(["items": page(20)])))
        await more.value
        #expect(rows(s) == 20)

        let toggle = run("toggleHidden", s, fx)
        await fx.answer("raffle.toggleHidden", ok(.record(["hidden": .bool(true)])))
        await toggle.value
        // page 2 has the old names: dropped, reloaded as the table scrolls; page 1 (`board`) is refreshed
        #expect(rows(s) == 0)
        #expect(!flag(s, "boardDone") && !flag(s, "moreFailed") && !flag(s, "loadingMore"))
        #expect(fx.performed == [#"refresh "board""#])

        let again = run("loadMore", s, fx)
        await fx.answer("raffle.leaderboard", ok(.record(["items": page(20)])))
        await again.value
        #expect(rows(s) == 20)
        #expect(fx.errors.isEmpty)
    }

    @Test func aPageInFlightDuringTheToggleIsDropped() async throws {
        let s = try store(), fx = ScriptedEffects()
        let stale = run("loadMore", s, fx) // page 2, asked for under the old masking
        await fx.waitUntilPending("raffle.leaderboard")
        let toggle = run("toggleHidden", s, fx)
        await fx.answer("raffle.toggleHidden", ok(.record(["hidden": .bool(true)])))
        await toggle.value
        #expect(!flag(s, "loadingMore"))

        let fresh = run("loadMore", s, fx) // the table asks again at once, not held up by the stale request
        await fx.waitUntilPending("raffle.leaderboard", count: 2)
        await fx.answer("raffle.leaderboard", ok(.record(["items": page(20)]))) // the stale page 2
        await stale.value
        #expect(rows(s) == 0)
        await fx.answer("raffle.leaderboard", ok(.record(["items": page(20)]))) // page 2 under the new masking
        await fresh.value
        #expect(rows(s) == 20 && !flag(s, "loadingMore"))
        #expect(fx.errors.isEmpty)
    }
}
