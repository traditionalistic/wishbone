import SwiftUI
import WishboneCore

/// What an adapter-rendered primitive (`lottie`, `rive`, …) gets: evaluated props and a way to fire
/// the node's events.
@MainActor
public struct WishboneNodeContext {
    public let props: [String: Value]
    public let baseURL: URL
    let fireEvent: @MainActor (String, Value) -> Void

    /// Runs the node's `on[event]` with `event` = payload.
    public func fire(_ event: String, _ payload: Value = .record([:])) { fireEvent(event, payload) }

    public func number(_ key: String) -> Double? {
        if case let .number(n)? = props[key], n.isFinite { n } else { nil }
    }

    public func bool(_ key: String) -> Bool? {
        if case let .bool(b)? = props[key] { b } else { nil }
    }

    public func string(_ key: String) -> String? {
        if case let .string(s)? = props[key] { s } else { nil }
    }

    /// Resolves a (possibly relative) `url` prop against the server.
    public func url(_ key: String = "url") -> URL? {
        guard case let .string(s)? = props[key] else { return nil }
        return URL(string: s, relativeTo: baseURL)?.absoluteURL
    }
}

/// Registry of primitives rendered by optional adapters (WishboneLottie, WishboneRive). A node
/// whose type has no renderer renders nothing.
@MainActor
public enum WishboneRenderers {
    public typealias Make = @MainActor (WishboneNodeContext) -> AnyView
    private(set) static var registry: [String: Make] = [:]

    public static func register(_ type: String, _ make: @escaping Make) {
        registry[type] = make
    }
}
