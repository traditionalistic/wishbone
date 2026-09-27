import Foundation

/// Immutable linked chain of local bindings (`let`, handler/lambda params, `repeat` vars,
/// `event`) threaded explicitly through `eval`/statement execution — locals are not part of
/// `Scope` because they're lexical, not storage.
public final class Locals: Sendable {
    public let name: String
    public let value: Value
    public let parent: Locals?

    public init(name: String, value: Value, parent: Locals?) {
        self.name = name
        self.value = value
        self.parent = parent
    }

    public func lookup(_ name: String) -> Value? {
        var node: Locals? = self
        while let current = node {
            if current.name == name { return current.value }
            node = current.parent
        }
        return nil
    }
}

/// The read/write surface the interpreter needs from whatever owns document state (a test
/// double here; the real store in a later task). `@MainActor` so a `@MainActor` store
/// conforms directly (no per-read actor hop for every `state`/`computed`/`env` access on the
/// render-hot path) — `Interpreter` is `@MainActor` too, to match. Not constrained to
/// `AnyObject`, but `set` has no `mutating` keyword, so a real implementation is a reference
/// type.
@MainActor
public protocol Scope {
    func state(_ name: String) -> Value
    func instanceState(_ name: String) -> Value
    /// Implementations evaluate the named `computed` expression fresh on every read.
    func computed(_ name: String) -> Value
    /// `.null` when the source is absent or failed.
    func data(_ name: String) -> Value
    func param(_ name: String) -> Value
    func env(_ name: String) -> Value
    func token(_ group: String, _ name: String) -> Value
    func driver(_ kind: String, _ id: String, _ axis: String) -> Double
    func selfPressed() -> Bool
    /// `path` elements: `.string` → record field, `.number` → list index.
    func set(scope: String, name: String, path: [Value], value: Value, transaction: Value?)
}
