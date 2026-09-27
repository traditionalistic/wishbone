import CoreText
import Foundation
import SwiftUI

/// Fonts bundled from contract/assets/fonts. File names are `<Family>-<Style>.<ext>` and equal the
/// PostScript name; the family matches the theme's `type.*.family`.
enum Fonts {
    struct Face: Equatable {
        let family: String
        let weight: Int
        let postScriptName: String
    }

    static let styleWeights: [String: Int] = [
        "Thin": 100, "ExtraLight": 200, "Light": 300, "Regular": 400, "Medium": 500,
        "SemiBold": 600, "Bold": 700, "ExtraBold": 800, "Black": 900, "Heavy": 900,
    ]

    /// `Manrope-SemiBold` → (Manrope, 600). A trailing `Italic` is ignored for the weight.
    static func face(fileName: String) -> Face? {
        let base = (fileName as NSString).deletingPathExtension
        let parts = base.split(separator: "-", maxSplits: 1).map(String.init)
        guard parts.count == 2 else { return nil }
        var style = parts[1]
        if style.hasSuffix("Italic") { style.removeLast("Italic".count) }
        let weight = styleWeights[style.isEmpty ? "Regular" : style] ?? 400
        return Face(family: parts[0], weight: weight, postScriptName: base)
    }

    static let bundledURLs: [URL] = {
        guard let dir = Bundle.module.url(forResource: "Fonts", withExtension: nil),
              let files = try? FileManager.default.contentsOfDirectory(at: dir, includingPropertiesForKeys: nil)
        else { return [] }
        return files.filter { ["ttf", "otf"].contains($0.pathExtension.lowercased()) }
    }()

    static let faces: [Face] = bundledURLs.compactMap { face(fileName: $0.lastPathComponent) }

    /// Registers the bundled fonts once per process (idempotent).
    static let register: Void = {
        for url in bundledURLs {
            CTFontManagerRegisterFontsForURL(url as CFURL, .process, nil)
        }
    }()

    /// Nearest registered weight of `family` (ties → heavier), or nil when the family isn't bundled.
    static func postScriptName(family: String, weight: Int) -> String? {
        faces.filter { $0.family == family }
            .min { a, b in
                let da = abs(a.weight - weight), db = abs(b.weight - weight)
                return da != db ? da < db : a.weight > b.weight
            }?.postScriptName
    }

    static func font(_ style: TypeStyle) -> Font {
        _ = register
        if let name = postScriptName(family: style.family, weight: style.weight) {
            return .custom(name, size: style.size) // scales with Dynamic Type relative to .body
        }
        return .system(size: style.size, weight: systemWeight(style.weight))
    }

    static func systemWeight(_ w: Int) -> Font.Weight {
        switch w {
        case ..<150: .ultraLight
        case ..<250: .thin
        case ..<350: .light
        case ..<450: .regular
        case ..<550: .medium
        case ..<650: .semibold
        case ..<750: .bold
        case ..<850: .heavy
        default: .black
        }
    }
}
