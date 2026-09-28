import Foundation
import Testing
@testable import WishboneUI

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
