#if canImport(AppKit)
import AppKit
import Foundation
import SwiftUI
import Testing
@testable import WishboneUI

/// The live preview's tree follows the pushed screen (spec 2026-09-30 §1.1): another screen is a fresh stack — its root
/// appears anew and no route pushed by the previous screen stays on top — while an edit of the same screen keeps the tree.
/// Rendered for real in an `NSHostingView`, observed through the host's `track` events. Waits `await` (never a nested run
/// loop): the test body holds the main actor, and handlers run as main-actor tasks.
@MainActor
@Suite struct PreviewScreenTests {
    /// A root stack `id: root` whose `appear` tracks `appear:<screen>` and then runs `then`.
    private func message(screen: String, then: String = "") -> Data {
        Data("""
        {"type": "document", "screen": "\(screen)", "data": {}, "document": {"format": 1, "screen": "\(screen)", "revision": 1,
         "root": {"t": "stack", "id": "root", "on": {"appear": [["do", "track", [["v", "appear:\(screen)"]], false]\(then)]}}}}
        """.utf8)
    }

    /// True once `condition` holds (false after `seconds`).
    private func wait(_ seconds: Double = 5, for condition: () -> Bool) async throws -> Bool {
        let deadline = Date(timeIntervalSinceNow: seconds)
        while !condition() {
            if Date() > deadline { return false }
            try await Task.sleep(for: .milliseconds(20))
        }
        return true
    }

    @Test func anotherScreenStartsAFreshStackAndAnEditKeepsIt() async throws {
        let host = RecordingHost()
        let wb = Wishbone(baseURL: URL(string: "http://localhost:1")!, brand: "korpu", host: host, cache: DiskCache(dir: tempDir()))
        // The screen `a` navigates to on appear: local, so it renders without a network.
        wb.localScreens["next"] = Data("""
        {"document": {"format": 1, "screen": "next", "root": {"t": "stack", "id": "root",
         "on": {"appear": [["do", "track", [["v", "appear:next"]], false]], "disappear": [["do", "track", [["v", "disappear:next"]], false]]}}}}
        """.utf8)
        let client = PreviewClient(wb, code: "TEST")
        _ = NSApplication.shared
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 400, height: 800), styleMask: [.titled], backing: .buffered, defer: false)
        window.isReleasedWhenClosed = false // `close()` would release it under ARC's own reference: a crash in the next CA commit
        window.contentView = NSHostingView(rootView: WishbonePreviewScreen(wb, preview: client))
        window.orderFrontRegardless()
        defer { window.close() }

        client.handle(message(screen: "a", then: #", ["do", "navigate", [["v", "next"]], false]"#))
        #expect(try await wait { host.events.contains("track:appear:next") }, "a appeared and pushed next: \(host.events)")
        #expect(host.events == ["track:appear:a", "track:appear:next"])
        #expect(wb.clock.users == 2) // screens on stage: a's root and next


        client.handle(message(screen: "a")) // an edit of the same screen: the tree, and the pushed route, stay
        try await Task.sleep(for: .milliseconds(300))
        #expect(host.events == ["track:appear:a", "track:appear:next"] && wb.clock.users == 2)

        client.handle(message(screen: "b")) // another screen with the same root id: a fresh stack
        #expect(try await wait { host.events.contains("track:appear:b") }, "b appeared: \(host.events)")
        #expect(wb.clock.users == 1, "only b's root is on stage, the route pushed by a is gone")
    }
}
#endif
