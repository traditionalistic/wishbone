import Foundation
import Testing
@testable import WishboneUI

@Suite struct ThemeTests {
    @Test func bundledKorpuIsAnExactCopyOfTheContract() throws {
        let bundled = try Data(contentsOf: Theme.bundledURL)
        #expect(bundled == (try contractFile("tokens/korpu.json")))
        #expect(Theme.bundledKorpu == (try Theme(json: try contractFile("tokens/korpu.json"))))
    }

    @Test func bundledFontsAreExactCopiesOfTheContract() throws {
        let dir = repoRoot.appendingPathComponent("contract/assets/fonts")
        let contract = try FileManager.default.contentsOfDirectory(atPath: dir.path)
            .filter { ["ttf", "otf"].contains(($0 as NSString).pathExtension.lowercased()) }.sorted()
        #expect(Fonts.bundledURLs.map(\.lastPathComponent).sorted() == contract)
        for url in Fonts.bundledURLs {
            #expect(try Data(contentsOf: url) == Data(contentsOf: dir.appendingPathComponent(url.lastPathComponent)))
        }
        let license = try #require(Fonts.bundledURLs.first).deletingLastPathComponent().appendingPathComponent("OFL.txt")
        #expect(try Data(contentsOf: license) == Data(contentsOf: dir.appendingPathComponent("OFL.txt")))
    }

    @Test func hexColors() {
        #expect(RGBA(hex: "#FF000080") == RGBA(r: 1, g: 0, b: 0, a: 128.0 / 255))
        #expect(RGBA(hex: "#00FF00") == RGBA(r: 0, g: 1, b: 0, a: 1))
        #expect(RGBA(hex: "#XYZ") == nil)
    }

    @Test func colorTokensResolvePerSchemeWithAlpha() {
        let t = Theme.bundledKorpu
        #expect(t.color(color("el16"), dark: false) == RGBA(hex: "#080808FF"))
        #expect(t.color(color("el16"), dark: true) == RGBA(hex: "#F5F5F5FF"))
        #expect(t.color(color("el16", alpha: 0.5), dark: false)?.a == 0.5)
        #expect(t.color(color("nope"), dark: false) == nil)
        #expect(t.color(.string("el16"), dark: false) == nil)
    }

    @Test func paintIsColorOrGradient() {
        let t = Theme.bundledKorpu
        #expect(t.paint(color("bg1"), dark: false) == .color(RGBA(hex: "#F8F8F8FF")!))
        let g: Value = .record(["$gradient": .list([color("el1"), color("el16", alpha: 0.5)]), "angle": .number(90)])
        #expect(t.paint(g, dark: false) == .gradient([RGBA(hex: "#FFFFFFFF")!, RGBA(r: 8.0 / 255, g: 8.0 / 255, b: 8.0 / 255, a: 0.5)], angle: 90))
        #expect(t.paint(.null, dark: false) == nil)
    }

    @Test func textStyleAndMotionTokens() {
        let t = Theme.bundledKorpu
        #expect(t.textStyle(.record(["$type": .string("sheetTitle")])) == TypeStyle(family: "Montserrat", size: 24, weight: 600, lineHeight: 28, tracking: 0))
        #expect(t.motion(.record(["$motion": .string("snappy")])) == MotionSpec(kind: "spring", response: 180, damping: 0.86, duration: nil, bezier: nil))
        let spec: Value = .record(["$motion": .record(["kind": .string("curve"), "duration": .number(200), "bezier": .list([0, 0, 1, 1].map(Value.number))])])
        #expect(t.motion(spec) == MotionSpec(kind: "curve", response: nil, damping: nil, duration: 200, bezier: [0, 0, 1, 1]))
    }

    @Test func fontFacesAndNearestWeight() {
        #expect(Fonts.face(fileName: "Manrope-SemiBold.ttf") == .init(family: "Manrope", weight: 600, postScriptName: "Manrope-SemiBold"))
        #expect(Fonts.face(fileName: "GiorgioSansLCG-HeavyItalic.otf")?.weight == 900)
        #expect(Fonts.postScriptName(family: "Manrope", weight: 400) == "Manrope-Regular")
        #expect(Fonts.postScriptName(family: "Montserrat", weight: 700) == "Montserrat-ExtraBold") // tie → heavier
        #expect(Fonts.postScriptName(family: "GiorgioSansLCG", weight: 800) == "GiorgioSansLCG-HeavyItalic")
        #expect(Fonts.postScriptName(family: "Nope", weight: 400) == nil)
        for style in Theme.bundledKorpu.type.values {
            #expect(Fonts.postScriptName(family: style.family, weight: style.weight) != nil, "\(style.family) not bundled")
        }
    }

    @Test func iconTableMatchesContract() throws {
        let json = try JSONSerialization.jsonObject(with: try contractFile("icons.json")) as! [String: [String: String]]
        #expect(Icons.symbols == json.mapValues { $0["ios"]! })
    }

    @Test func propDefaultsMatchPrimitivesJSON() throws {
        let json = try JSONSerialization.jsonObject(with: try contractFile("primitives.json")) as! [String: Any]
        func literal(_ any: Any) throws -> Value {
            if let s = any as? String, s.hasPrefix("@") {
                let parts = s.dropFirst().split(separator: ".")
                return .record(["$\(parts[0])": .string(String(parts[1]))])
            }
            return try Value(json: JSONSerialization.data(withJSONObject: any, options: .fragmentsAllowed))
        }
        func check(_ type: String, _ props: [String: Any]) throws {
            for (name, spec) in props {
                guard let d = (spec as? [String: Any])?["default"] else { continue }
                #expect(Props(type: type, values: [:])[name] == (try literal(d)), "\(type).\(name)")
            }
        }
        try check("common", json["common"] as! [String: Any])
        for (type, p) in json["primitives"] as! [String: [String: Any]] {
            try check(type, p["props"] as! [String: Any])
        }
    }
}
