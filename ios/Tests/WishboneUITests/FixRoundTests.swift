import Foundation
import Testing
@testable import WishboneUI

/// 9a review fixes that are plain logic.
@MainActor
@Suite struct FixRoundTests {
    private let doc = try! Document(json: #"{"format": 1, "screen": "s", "root": {"t": "stack", "id": "root"}}"#)

    final class RecordingHost: WishboneHost {
        var opened: [URL] = []
        func open(url: URL) { opened.append(url) }
    }

    @Test func safeInsetsAndScreenSizeArePerNavigationStack() {
        let wb = testWishbone()
        let screen = Navigator(), sheet = Navigator(presenter: screen)
        screen.geometry.safeBottom = 34
        sheet.geometry.safeBottom = 0
        sheet.geometry.screenSize = CGSize(width: 402, height: 780)
        let under = RenderContext(store: Store(document: doc, params: [:], data: [:], wishbone: wb, navigator: screen))
        let inSheet = RenderContext(store: Store(document: doc, params: [:], data: [:], wishbone: wb, navigator: sheet))
        #expect(under.env("safeBottom") == .number(34) && inSheet.env("safeBottom") == .number(0))
        #expect(inSheet.env("screenHeight") == .number(780) && under.env("screenHeight") == .number(844))
    }

    @Test func openRejectsProtocolRelativeURLs() async {
        let host = RecordingHost()
        let wb = Wishbone(baseURL: URL(string: "http://localhost:8080")!, brand: "korpu", host: host, cache: DiskCache(dir: tempDir()))
        let store = Store(document: doc, params: [:], data: [:], wishbone: wb)
        for raw in ["//evil.example/x", "http://plain.example", "/assets/a.png", "https://ok.example"] {
            await store.perform(effect: "open", args: [.string(raw)], awaited: false)
        }
        #expect(host.opened.map(\.absoluteString) == ["http://localhost:8080/assets/a.png", "https://ok.example"])
    }

    @Test func relativeURLsKeepTheBasePath() {
        let base = URL(string: "http://h:8080/api/")!
        #expect(Wishbone.resolve("/assets/a.png", base: base)?.absoluteString == "http://h:8080/api/assets/a.png")
        #expect(Wishbone.resolve("//cdn.example/a.png", base: base)?.absoluteString == "//cdn.example/a.png")
        #expect(Wishbone.resolve("https://x.example/a", base: base)?.absoluteString == "https://x.example/a")
        #expect(Wishbone.resolve("a.png", base: base)?.absoluteString == "a.png")
    }

    @Test func dragReaderUsesTheNearestRepeatPathWithACell() {
        let store = Store(document: doc, params: [:], data: [:], wishbone: testWishbone())
        store.driver("drag:card.a:x").value = .number(12)
        var ctx = RenderContext(store: store)
        ctx.idSuffix = ".a.inner"                 // a reader nested deeper than the dragged instance
        #expect(ctx.driver("drag", "card", "x") == 12)
        ctx.idSuffix = ".b"                       // another instance, not dragged yet
        #expect(ctx.driver("drag", "card", "x") == 0)
        #expect(store.existingDriver("drag:card.b:x") == nil) // reading creates no cells
    }

    @Test func evalErrorsAreLoggedOncePerNodeAndMessage() {
        let wb = testWishbone()
        var logs: [String] = []
        wb.onLog = { _, m in logs.append(m) }
        var ctx = RenderContext(store: Store(document: doc, params: [:], data: [:], wishbone: wb))
        let bad = Expr.binary(op: "+", .literal(.number(1)), .literal(.string("x")))
        ctx.nodeID = "a"
        for _ in 0..<3 { _ = ctx.eval(bad) }
        ctx.nodeID = "b"
        _ = ctx.eval(bad)
        #expect(logs.count == 2 && logs[0].contains("a:") && logs[1].contains("b:"))
    }

    @Test func brandWithoutCacheFallsBackToBundledKorpu() {
        let cache = DiskCache(dir: tempDir())
        var kz = try! JSONSerialization.jsonObject(with: contractFile("tokens/kz.json")) as! [String: Any]
        kz["version"] = "v1"
        let base = URL(string: "http://localhost:1")!
        cache.write(DiskCache.themeKey(scope: Wishbone.cacheScope(base, DefaultHost(), "kz")), try! JSONSerialization.data(withJSONObject: kz))
        let wb = Wishbone(baseURL: base, brand: "korpu", host: DefaultHost(), cache: cache)
        wb.brand = "kz"
        #expect(wb.theme.brand == "kz")
        wb.brand = "unknown"
        #expect(wb.theme == .bundledKorpu)
    }

    @Test func previewLogDoesNotReplaceTheHostLogger() {
        let wb = testWishbone()
        var host: [String] = []
        wb.onLog = { _, m in host.append(m) }
        let preview = PreviewClient(wb, code: "X")
        preview.connect()
        wb.log("error", "boom")
        preview.disconnect()
        wb.log("error", "after")
        #expect(host == ["boom", "after"] && wb.previewLog == nil)
    }
}
