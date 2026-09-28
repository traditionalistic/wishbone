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
