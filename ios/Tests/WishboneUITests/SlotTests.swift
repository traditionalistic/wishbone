import Foundation
import Synchronization
import Testing
@testable import WishboneUI

/// Screen requests answered in memory, counted; `document: null` when `If-None-Match` matches the etag.
final class FakeServer: Sendable {
    private let state = Mutex<(requests: Int, notFound: Bool)>((0, false))
    let etag = "e1"
    let document: String

    init(document: String = #"{"format": 1, "screen": "slot.x", "root": {"t": "stack", "id": "root"}}"#) {
        self.document = document
    }

    var requests: Int { state.withLock { $0.requests } }
    /// From now on the screen is unpublished (`SCREEN_NOT_FOUND`).
    func unpublish() { state.withLock { $0.notFound = true } }

    @Sendable func send(_ req: URLRequest) async throws -> (Data?, HTTPURLResponse) {
        let notFound = state.withLock { s in s.requests += 1; return s.notFound }
        let http = HTTPURLResponse(url: req.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
        if notFound { throw API.error(status: 404, body: Data(#"{"error": {"code": "SCREEN_NOT_FOUND", "message": "", "retryable": false}}"#.utf8)) }
        let document = req.value(forHTTPHeaderField: "If-None-Match") == etag ? "null" : document
        return (Data(#"{"etag": "\#(etag)", "revision": 1, "data": {"promo": {"ok": true, "value": 1}}, "document": \#(document)}"#.utf8), http)
    }
}

/// A monotonic clock the test moves by hand.
final class TestClock: Sendable {
    private let instant = Mutex(ContinuousClock.now)
    @Sendable func now() -> ContinuousClock.Instant { instant.withLock { $0 } }
    func advance(_ d: Duration) { instant.withLock { $0 = $0.advanced(by: d) } }
}

/// Slots (spec 2026-09-28 §2.3): an unpublished slot drops its cache; other errors keep it.
@MainActor
@Suite struct SlotTests {
    private func server(_ code: String, _ status: Int) -> WishboneError {
        .server(status: status, code: code, message: "", retryable: false)
    }

    @Test func onlyASlotsNotFoundDropsTheCache() {
        #expect(ScreenModel.dropsCache(after: server("SCREEN_NOT_FOUND", 404), slot: true))
        #expect(!ScreenModel.dropsCache(after: server("SCREEN_NOT_FOUND", 404), slot: false)) // a screen keeps showing its cache
        #expect(!ScreenModel.dropsCache(after: server("DATA_REQUIRED_FAILED", 502), slot: true))
        #expect(!ScreenModel.dropsCache(after: WishboneError.network("offline"), slot: true))
        #expect(!ScreenModel.dropsCache(after: CancellationError(), slot: true))
    }

    @Test func aSlotShowsWhenItsRootsWhenHolds() throws {
        let wb = testWishbone()
        let doc = try Document(json: #"{"format": 1, "screen": "slot.x", "state": [["on", ["v", true]]], "root": {"t": "stack", "id": "root", "when": ["s", "on"]}}"#)
        let store = Store(document: doc, params: [:], data: [:], wishbone: wb)
        #expect(!slotVisible(nil)) // nothing loaded (yet), unpublished or failed
        #expect(slotVisible(store))
        RenderContext(store: store).set(scope: "s", name: "on", path: [], value: .bool(false), transaction: nil)
        #expect(!slotVisible(store)) // e.g. the raffle is over
        let plain = try Document(json: #"{"format": 1, "screen": "slot.x", "root": {"t": "stack", "id": "root"}}"#)
        #expect(slotVisible(Store(document: plain, params: [:], data: [:], wishbone: wb)))
    }

    /// A cached slot is there in its first frame (no enter animation): the cache is read before any await.
    @Test func aCachedSlotIsShownSynchronously() {
        let wb = testWishbone()
        let key = DiskCache.screenKey(scope: wb.cacheScope, id: "slot.x", params: [:])
        wb.cache.write(key, Data(#"{"etag": "e", "revision": 1, "data": {}, "document": {"format": 1, "screen": "slot.x", "root": {"t": "stack", "id": "root"}}}"#.utf8))
        let model = ScreenModel(wishbone: wb, route: Route(screen: "slot.x", params: [:]), navigator: Navigator(), slot: true)
        #expect(model.showCached() == key)
        #expect(slotVisible(model.store))
    }

    /// A slot is a row of the host's feed: at most 480 tall (a taller lazy row can livelock `LazyVStack`), content below clipped.
    @Test func aSlotIsAtMost480Tall() {
        #expect(Reveal.height(content: 120, fraction: 1) == 120) // content-sized below the cap
        #expect(Reveal.height(content: 900, fraction: 1) == 480)
        #expect(Reveal.height(content: 900, fraction: 0.5) == 240) // the reveal animates the capped height
        #expect(Reveal.height(content: 120, fraction: -0.1) == 0) // a spring overshooting below 0
    }

    /// Ruling R1: a slot that appears again renders its last network answer from memory at once and asks the server
    /// again only once that answer is 30 s old; params are part of the key.
    @Test func aSlotRevalidatesOnlyOnceItsLastNetworkAnswerIs30sOld() async {
        let clock = TestClock(), server = FakeServer()
        let wb = Wishbone(baseURL: URL(string: "http://localhost:1")!, brand: "korpu", host: DefaultHost(), cache: DiskCache(dir: tempDir()),
                          now: clock.now, send: server.send)
        func slot(_ params: [String: Value] = [:]) -> ScreenModel {
            ScreenModel(wishbone: wb, route: Route(screen: "slot.x", params: params), navigator: Navigator(), slot: true)
        }
        let first = slot()
        first.showCached() // appearance: nothing cached yet
        #expect(first.store == nil)
        await first.load()
        #expect(server.requests == 1 && slotVisible(first.store))

        let key = DiskCache.screenKey(scope: wb.cacheScope, id: "slot.x", params: [:])
        let onDisk = wb.cache.read(key)
        #expect(onDisk != nil)
        wb.cache.remove(key) // the memo needs no disk
        clock.advance(.milliseconds(29_999))
        let again = slot()
        again.showCached()
        #expect(slotVisible(again.store)) // in its first frame
        await again.load()
        #expect(server.requests == 1)

        wb.cache.write(key, onDisk!)
        let written = wb.cache.read(key)
        clock.advance(.milliseconds(1)) // 30 s
        let stale = slot()
        stale.showCached()
        await stale.load()
        #expect(server.requests == 2 && slotVisible(stale.store))
        #expect(wb.cache.read(key) == written) // same etag and data: no disk write

        await slot().load()
        #expect(server.requests == 2) // the revalidation counts as the last network answer

        let other = slot(["x": .number(1)])
        other.showCached()
        #expect(other.store == nil) // another key
        await other.load()
        #expect(server.requests == 3)
    }

    /// A slot rendered from memory still shows only when its root's `when` holds.
    @Test func aMemoHitStillHonoursTheRootsWhen() async {
        let server = FakeServer(document: #"""
        {"format": 1, "screen": "slot.x", "state": [["on", ["v", false]]], "root": {"t": "stack", "id": "root", "when": ["s", "on"]}}
        """#)
        let wb = Wishbone(baseURL: URL(string: "http://localhost:1")!, brand: "korpu", host: DefaultHost(), cache: DiskCache(dir: tempDir()),
                          now: TestClock().now, send: server.send)
        func slot() -> ScreenModel { ScreenModel(wishbone: wb, route: Route(screen: "slot.x", params: [:]), navigator: Navigator(), slot: true) }
        await slot().load()
        let again = slot()
        again.showCached()
        #expect(again.store != nil && !slotVisible(again.store))
        #expect(server.requests == 1)
    }

    /// An unpublished slot's `SCREEN_NOT_FOUND` drops its memo entry together with its disk cache and shows nothing.
    @Test func aSlotsNotFoundDropsItsMemoAndDiskCache() async {
        let clock = TestClock(), server = FakeServer()
        let wb = Wishbone(baseURL: URL(string: "http://localhost:1")!, brand: "korpu", host: DefaultHost(), cache: DiskCache(dir: tempDir()),
                          now: clock.now, send: server.send)
        func slot() -> ScreenModel { ScreenModel(wishbone: wb, route: Route(screen: "slot.x", params: [:]), navigator: Navigator(), slot: true) }
        let key = DiskCache.screenKey(scope: wb.cacheScope, id: "slot.x", params: [:])
        await slot().load()
        #expect(wb.cache.read(key) != nil)

        server.unpublish()
        clock.advance(.seconds(30))
        let gone = slot()
        gone.showCached()
        #expect(slotVisible(gone.store)) // the memo, until the server says otherwise
        await gone.load()
        #expect(server.requests == 2 && !slotVisible(gone.store))
        #expect(wb.cache.read(key) == nil)

        let next = slot() // within 30 s of the last answer, but there is no entry to trust
        next.showCached()
        #expect(next.store == nil)
        await next.load()
        #expect(server.requests == 3)
    }

    @Test func removeDeletesTheEntry() {
        let cache = DiskCache(dir: tempDir())
        let key = DiskCache.screenKey(scope: "s", id: "slot.x", params: [:])
        cache.write(key, Data("{}".utf8))
        cache.write("other", Data("{}".utf8))
        cache.remove(key)
        cache.remove("missing") // no entry: nothing happens
        #expect(cache.read(key) == nil)
        #expect(cache.read("other") != nil)
    }
}
