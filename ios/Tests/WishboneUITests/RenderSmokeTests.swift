import SwiftUI
import Testing
@testable import WishboneUI

/// Renders every bundled playground fixture offscreen: every node must evaluate without runtime errors.
@MainActor
@Suite struct RenderSmokeTests {
    nonisolated static let fixtures: [URL] = {
        let dir = repoRoot.appendingPathComponent("ios/Playground/Resources/Fixtures")
        return ((try? FileManager.default.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil)) ?? [])
            .filter { $0.pathExtension == "json" }.sorted { $0.path < $1.path }
    }()

    /// Garbage the compiler can't rule out (IEEE specials from expressions, negative sizes, nested
    /// same-axis scrolls, unknown icon/primitive, duplicate repeat keys) must render, not crash.
    @Test func hostileDocumentRenders() throws {
        let doc = try Document(json: #"""
        {"format": 1, "screen": "s", "root": {"t": "scroll", "id": "outer", "c": [
          {"t": "scroll", "id": "inner", "p": {"height": ["op", "/", ["v", 1], ["v", 0]]}, "c": [
            {"t": "text", "id": "t", "p": {"text": ["v", "x"], "maxLines": ["op", "/", ["v", 0], ["v", 0]], "width": ["op", "/", ["v", 0], ["v", 0]]}}]},
          {"t": "grid", "id": "g", "p": {"columns": ["v", 1e300], "spacing": ["op", "/", ["v", -1], ["v", 0]], "padding": ["v", -20]}, "c": [
            {"t": "icon", "id": "i", "p": {"name": ["v", {"$icon": "nope"}], "size": ["v", -3]}},
            {"t": "shape", "id": "sh", "p": {"width": ["v", -1], "corner": ["op", "/", ["v", 1], ["v", 0]], "stroke": ["v", {"$color": "el3"}]}},
            {"t": "mystery", "id": "m"},
            {"t": "text", "id": "r", "rep": {"over": ["v", [1, 1, 2]], "as": "item", "index": "index", "key": ["l", "item"]}, "p": {"text": ["v", "dup"]}}]}
        ]}}
        """#)
        let wb = testWishbone()
        var errors: [String] = []
        wb.onLog = { level, message in if level == "error" { errors.append(message) } }
        let store = Store(document: doc, params: [:], data: [:], wishbone: wb)
        let view = VStack { ForEach(doc.root.children, id: \.id) { NodeView(node: $0, ctx: RenderContext(store: store).child(axis: "v")) } }.frame(width: 390)
        #expect(ImageRenderer(content: view).cgImage != nil)
        #expect(errors.isEmpty, "\(errors)")
    }

    /// A `drag` node's cells are made on its first drag, never while a body runs (an observable write in a view
    /// update); before that its props read 0.
    @Test func renderingADragNodeMakesNoDragCells() throws {
        let doc = try Document(json: #"""
        {"format": 1, "screen": "s", "root": {"t": "stack", "id": "root", "c": [
          {"t": "shape", "id": "knob", "drag": {"axis": "x"}, "p": {"width": ["v", 40], "height": ["v", 40],
           "scale": ["op", "+", ["v", 1], ["dr", "drag", "knob", "x"]]}}]}}
        """#)
        let store = Store(document: doc, params: [:], data: [:], wishbone: testWishbone())
        #expect(ImageRenderer(content: NodeView(node: doc.root, ctx: RenderContext(store: store))).cgImage != nil)
        #expect(store.existingDriver("drag:knob:x") == nil && store.existingDriver("drag:knob:y") == nil)
        #expect(RenderContext(store: store).driver("drag", "knob", "x") == 0)
    }

    @Test(arguments: fixtures)
    func fixtureRendersWithoutErrors(_ url: URL) throws {
        let wb = testWishbone()
        var errors: [String] = []
        wb.onLog = { level, message in if level == "error" { errors.append(message) } }
        let doc = try Document(json: try API.parseScreen(try Data(contentsOf: url)).document!)
        let store = Store(document: doc, params: [:], data: [:], wishbone: wb)
        // ImageRenderer can't draw a ScrollView's content, so render the root's children in a VStack.
        let ctx = RenderContext(store: store).child(axis: "v")
        let nodes = doc.root.type == "scroll" ? doc.root.children : [doc.root]
        let view = VStack(spacing: 0) { ForEach(nodes, id: \.id) { NodeView(node: $0, ctx: ctx) } }.frame(width: 390)
        #expect(ImageRenderer(content: view).cgImage != nil)
        #expect(errors.isEmpty, "\(errors)")
    }
}
