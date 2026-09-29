import CoreGraphics
import Foundation
import SwiftUI
import Testing
@testable import WishboneUI

/// `shape.trim` (contract/primitives.json): a circle's stroke drawn only from `from` to `to`, fractions of the
/// circumference from 12 o'clock clockwise, round caps. Same vectors as Android `ShapeTrimTest`.
@MainActor
@Suite struct ShapeTrimTests {
    @Test func parsesAndClamps() {
        func trim(_ r: [String: Value]) -> ShapeNode.Trim? { ShapeNode.trim(.record(r)) }
        #expect(trim(["from": .number(0), "to": .number(0.25)]) == .init(from: 0, to: 0.25))
        #expect(trim(["from": .number(-1), "to": .number(2)]) == .init(from: 0, to: 1))
        #expect(trim(["from": .number(0.5), "to": .number(0.2)]) == .init(from: 0.5, to: 0.2)) // draws nothing
        #expect(trim(["from": .number(0)]) == nil)
        #expect(trim(["from": .number(.nan), "to": .number(1)]) == nil)
        #expect(ShapeNode.trim(nil) == nil)
    }

    /// 100×100 circle, 10pt red stroke (inside the bounds: centerline radius 45) on white. Samples the ring at
    /// `degrees` clockwise from 12 o'clock; true when the stroke covers that point. (The white backdrop keeps the
    /// display list non-empty — ImageRenderer hands back a stale image for an empty one.)
    private func inked(trim: String?, at degrees: [Double]) throws -> [Bool] {
        let trimProp = trim.map { #", "trim": ["v", \#($0)]"# } ?? ""
        let doc = try Document(json: #"""
        {"format": 1, "screen": "s", "root": {"t": "shape", "id": "ring", "p": {"kind": ["v", "circle"], "width": ["v", 100], "height": ["v", 100],
          "stroke": ["v", {"color": {"$color": "error"}, "width": 10}]\#(trimProp)}}}
        """#)
        let store = Store(document: doc, params: [:], data: [:], wishbone: testWishbone())
        let renderer = ImageRenderer(content: NodeView(node: doc.root, ctx: RenderContext(store: store)).frame(width: 100, height: 100).background(Color.white))
        renderer.scale = 1
        let image = try #require(renderer.cgImage)
        let (w, h) = (image.width, image.height)
        var pixels = [UInt8](repeating: 0, count: w * h * 4)
        let ctx = try #require(CGContext(data: &pixels, width: w, height: h, bitsPerComponent: 8, bytesPerRow: w * 4,
                                         space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
        ctx.draw(image, in: CGRect(x: 0, y: 0, width: w, height: h))
        return degrees.map { deg in
            let a = deg * .pi / 180
            let x = Int((50 + 45 * sin(a)).rounded()), y = Int((50 - 45 * cos(a)).rounded()) // row 0 = top
            return pixels[(y * w + x) * 4 + 1] < 128 // green channel: white 255, red stroke ~0
        }
    }

    @Test func drawsOnlyTheTrimmedArcClockwiseFromTwelve() throws {
        // 0…0.25 = 12 → 3 o'clock; round caps reach ~6.4° past each end at this radius.
        #expect(try inked(trim: #"{"from": 0, "to": 0.25}"#, at: [10, 45, 80]) == [true, true, true])
        #expect(try inked(trim: #"{"from": 0, "to": 0.25}"#, at: [110, 180, 270, 340]) == [false, false, false, false])
    }

    @Test func noTrimDrawsTheWholeRingAndAnEmptyTrimNothing() throws {
        #expect(try inked(trim: nil, at: [45, 135, 225, 315]) == [true, true, true, true])
        #expect(try inked(trim: #"{"from": 0.5, "to": 0.2}"#, at: [0, 90, 180, 270]) == [false, false, false, false])
    }
}
