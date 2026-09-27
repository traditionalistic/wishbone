import SwiftUI
import Testing
@testable import WishboneUI

@Suite struct LayoutTests {
    private func props(_ type: String, _ values: [String: Value]) -> Props { Props(type: type, values: values) }

    @Test func dimensions() {
        #expect(Dim(.string("fill")) == .fill)
        #expect(Dim(.string("wrap")) == .wrap)
        #expect(Dim(.number(40)) == .fixed(40))
        #expect(Dim(.null) == .wrap)
    }

    @Test func paddingNumberAndRecord() {
        #expect(Insets(.number(8)) == Insets(top: 8, bottom: 8, start: 8, end: 8))
        let rec: Value = .record(["h": .number(16), "v": .number(4), "start": .number(8), "bottom": .number(0)])
        #expect(Insets(rec) == Insets(top: 4, bottom: 0, start: 8, end: 16)) // specific edges win over h/v
    }

    @Test func hostileNumbersAreSanitized() {
        #expect(Dim(.number(.nan)) == .wrap && Dim(.number(.infinity)) == .wrap && Dim(.number(-5)) == .fixed(0))
        #expect(Insets(.number(-4)) == Insets(top: 0, bottom: 0, start: 0, end: 0))
        #expect(Insets(.record(["h": .number(.nan), "top": .number(-1), "v": .number(3)])) == Insets(top: 0, bottom: 3, start: 0, end: 0))
        let b = BoxProps(props("stack", [
            "opacity": .number(.nan), "scale": .number(.infinity), "offsetX": .number(-.infinity), "corner": .number(-8),
            "minWidth": .number(-10), "aspect": .number(0), "shadow": .record(["color": color("shadow"), "radius": .number(.nan)]),
        ]))
        #expect(b.opacity == 1 && b.scale == 1 && b.offsetX == 0 && b.corner == 0 && b.minWidth == 0 && b.aspect == nil && b.shadow == nil)
        #expect(props("grid", ["columns": .number(1e300)]).count("columns", in: 1...64) == 64)
        #expect(props("text", ["maxLines": .number(.nan)]).count("maxLines", in: 0...10) == nil)
    }

    @Test func boxDefaultsAreIdentity() {
        let b = BoxProps(props("stack", [:]))
        #expect(b.width == .wrap && b.height == .wrap)
        #expect(b.padding == Insets(.number(0)))
        #expect(b.opacity == 1 && b.scale == 1 && b.rotation == 0 && b.offsetX == 0 && b.zIndex == 0)
        #expect(b.corner == 0 && !b.clip && b.background == nil && b.border == nil && b.shadow == nil)
    }

    @Test func boxDecorations() {
        let b = BoxProps(props("stack", [
            "width": .string("fill"), "height": .number(40), "maxWidth": .number(300), "aspect": .number(1.5),
            "border": .record(["color": color("el3"), "width": .number(1)]),
            "shadow": .record(["color": color("shadow"), "radius": .number(8), "y": .number(2)]),
            "corner": .number(9999), "clip": .bool(true), "opacity": .number(0.5),
        ]))
        #expect(b.width == .fill && b.height == .fixed(40) && b.maxWidth == 300 && b.aspect == 1.5)
        #expect(b.border?.width == 1 && b.border?.color == color("el3"))
        #expect(b.shadow?.radius == 8 && b.shadow?.x == 0 && b.shadow?.y == 2)
        #expect(b.corner == 9999 && b.clip && b.opacity == 0.5)
    }

    @Test func stackAlignmentMapping() {
        #expect(Align.horizontal(nil) == .leading)
        #expect(Align.horizontal("center") == .center)
        #expect(Align.horizontal("end") == .trailing)
        #expect(Align.vertical(nil) == .top)
        #expect(Align.vertical("end") == .bottom)
        #expect(Align.z(nil) == .center)
        let nine = ["topStart", "top", "topEnd", "start", "center", "end", "bottomStart", "bottom", "bottomEnd"]
        let expected: [Alignment] = [.topLeading, .top, .topTrailing, .leading, .center, .trailing, .bottomLeading, .bottom, .bottomTrailing]
        #expect(nine.map(Align.z) == expected)
    }

    @Test func contentPlacementInsideALargerBox() {
        #expect(Align.frame(type: "stack", props: props("stack", [:])) == .topLeading)
        #expect(Align.frame(type: "stack", props: props("stack", ["align": .string("center")])) == .top)
        #expect(Align.frame(type: "stack", props: props("stack", ["axis": .string("h"), "align": .string("center")])) == .leading)
        #expect(Align.frame(type: "stack", props: props("stack", ["axis": .string("z")])) == .center)
        #expect(Align.frame(type: "stack", props: props("stack", ["axis": .string("z"), "align": .string("bottomEnd")])) == .bottomTrailing)
        #expect(Align.frame(type: "text", props: props("text", ["textAlign": .string("end")])) == .topTrailing)
        #expect(Align.frame(type: "scroll", props: props("scroll", ["axis": .string("h")])) == .topLeading)
        #expect(Align.frame(type: "shape", props: props("shape", [:])) == .center)
    }

    @Test func cssGradientAngles() {
        func pts(_ a: Double, _ w: Double = 100, _ h: Double = 100) -> [Double] {
            let p = Align.gradientPoints(angle: a, size: CGSize(width: w, height: h))
            return [p.start.x, p.start.y, p.end.x, p.end.y].map { ($0 * 1e9).rounded() / 1e9 }
        }
        #expect(pts(0) == [0.5, 1, 0.5, 0])     // to top
        #expect(pts(90) == [0, 0.5, 1, 0.5])    // to right
        #expect(pts(180) == [0.5, 0, 0.5, 1])   // to bottom
        #expect(pts(45) == [0, 1, 1, 0])        // corner to corner on a square
        // 45deg on 200×100: line length = (200+100)/√2, endpoints are the corner projections.
        let p = pts(45, 200, 100)
        #expect(abs(p[0] - 0.125) < 1e-9 && abs(p[1] - 1.25) < 1e-9 && abs(p[2] - 0.875) < 1e-9 && abs(p[3] + 0.25) < 1e-9)
    }

    @Test func cornerClampsToCapsule() {
        let r = CGRect(x: 0, y: 0, width: 100, height: 40)
        #expect(BoxShape.radius(9999, in: r) == 20)
        #expect(BoxShape.radius(8, in: r) == 8)
        #expect(BoxShape.radius(-3, in: r) == 0)
    }

    @Test func frameSizing() {
        typealias F = BoxFrame
        // fixed: exact (clamped by min/max), whatever is offered
        #expect(F.size(.fixed(80), 300, child: 20, min: nil, max: nil) == 80)
        #expect(F.size(.fixed(80), nil, child: 20, min: nil, max: 60) == 60)
        #expect(F.childProposal(.fixed(80), 300, min: nil, max: nil) == 80)
        // fill: takes the offer (clamped); unspecified offer (scroll axis) → content size
        #expect(F.size(.fill, 300, child: 20, min: nil, max: nil) == 300)
        #expect(F.size(.fill, 300, child: 20, min: nil, max: 120) == 120)
        #expect(F.size(.fill, .infinity, child: 20, min: nil, max: nil) == .infinity)
        #expect(F.size(.fill, nil, child: 20, min: nil, max: nil) == 20)
        #expect(F.size(.fill, 0, child: 20, min: nil, max: nil) == 0)
        #expect(F.childProposal(.fill, 300, min: nil, max: 120) == 120)
        // wrap: content within max, reported within min…max
        #expect(F.size(.wrap, 300, child: 20, min: 44, max: nil) == 44)
        #expect(F.size(.wrap, 300, child: 200, min: nil, max: 120) == 120)
        #expect(F.childProposal(.wrap, 300, min: 44, max: 120) == 120)
        #expect(F.childProposal(.wrap, nil, min: nil, max: 120) == nil)
    }
}
