import Foundation
import WishboneCore

/// sRGB color with alpha, 0…1 components.
public struct RGBA: Equatable, Sendable {
    public var r, g, b, a: Double

    public init(r: Double, g: Double, b: Double, a: Double) {
        self.r = r; self.g = g; self.b = b; self.a = a
    }

    /// `#RRGGBBAA` or `#RRGGBB`.
    public init?(hex: String) {
        let s = hex.hasPrefix("#") ? String(hex.dropFirst()) : hex
        guard s.count == 6 || s.count == 8, let v = UInt64(s, radix: 16) else { return nil }
        let rgba = s.count == 6 ? v << 8 | 0xFF : v
        self.init(
            r: Double(rgba >> 24 & 0xFF) / 255, g: Double(rgba >> 16 & 0xFF) / 255,
            b: Double(rgba >> 8 & 0xFF) / 255, a: Double(rgba & 0xFF) / 255
        )
    }
}

public struct SchemeColor: Equatable, Sendable, Decodable {
    public let light: RGBA
    public let dark: RGBA

    public init(from decoder: Decoder) throws {
        let raw = try [String: String](from: decoder)
        guard let l = raw["light"].flatMap(RGBA.init(hex:)), let d = raw["dark"].flatMap(RGBA.init(hex:)) else {
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "bad color"))
        }
        light = l
        dark = d
    }
}

public struct TypeStyle: Hashable, Sendable, Decodable {
    public let family: String
    public let size: Double
    public let weight: Int
    public let lineHeight: Double?
    public let tracking: Double?
}

/// `{kind: "spring", response, damping} | {kind: "curve", duration, bezier}` (spec §6.4).
public struct MotionSpec: Equatable, Sendable, Decodable {
    public let kind: String
    public let response: Double?
    public let damping: Double?
    public let duration: Double?
    public let bezier: [Double]?
}

/// Brand theme (spec §6.4) — the body of `GET /v1/theme`.
public struct Theme: Equatable, Sendable, Decodable {
    public let brand: String
    public let colors: [String: SchemeColor]
    public let type: [String: TypeStyle]
    public let space: [String: Double]
    public let radius: [String: Double]
    public let motion: [String: MotionSpec]

    public init(json: Data) throws {
        self = try JSONDecoder().decode(Theme.self, from: json)
    }

    static let bundledURL = Bundle.module.url(forResource: "korpu", withExtension: "json")!

    /// Snapshot of contract/tokens/korpu.json shipped in the SDK (a unit test keeps it equal).
    public static let bundledKorpu: Theme = try! Theme(json: Data(contentsOf: bundledURL))
}

/// A resolved `paint` value: a color or a linear gradient with a CSS angle.
public enum Paint: Equatable, Sendable {
    case color(RGBA)
    case gradient([RGBA], angle: Double)
}

extension Theme {
    /// `{"$color": name, "alpha"?: a}` → concrete color for the scheme.
    public func color(_ value: Value, dark: Bool) -> RGBA? {
        guard case let .record(fields) = value, case let .string(name)? = fields["$color"],
              let scheme = colors[name] else { return nil }
        var c = dark ? scheme.dark : scheme.light
        if case let .number(a)? = fields["alpha"] { c.a *= a }
        return c
    }

    /// `color` or `{"$gradient": [colors], "angle": deg}`.
    public func paint(_ value: Value, dark: Bool) -> Paint? {
        if let c = color(value, dark: dark) { return .color(c) }
        guard case let .record(fields) = value, case let .list(items)? = fields["$gradient"] else { return nil }
        let stops = items.compactMap { color($0, dark: dark) }
        guard !stops.isEmpty else { return nil }
        let angle: Double = if case let .number(a)? = fields["angle"] { a } else { 180 }
        return .gradient(stops, angle: angle)
    }

    /// `{"$type": name}` → text style.
    public func textStyle(_ value: Value) -> TypeStyle? {
        guard case let .record(fields) = value, case let .string(name)? = fields["$type"] else { return nil }
        return type[name]
    }

    /// `{"$motion": name}` (theme token) or `{"$motion": {spec}}` (stdlib constructor).
    public func motion(_ value: Value) -> MotionSpec? {
        guard case let .record(fields) = value else { return nil }
        switch fields["$motion"] {
        case let .string(name)?:
            return motion[name]
        case let .record(spec)?:
            func num(_ k: String) -> Double? { if case let .number(n)? = spec[k] { n } else { nil } }
            guard case let .string(kind)? = spec["kind"] else { return nil }
            let bezier: [Double]? = if case let .list(l)? = spec["bezier"] {
                l.compactMap { if case let .number(n) = $0 { n } else { nil } }
            } else { nil }
            return MotionSpec(kind: kind, response: num("response"), damping: num("damping"), duration: num("duration"), bezier: bezier)
        default:
            return nil
        }
    }
}
