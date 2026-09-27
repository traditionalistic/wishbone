import Foundation

/// Host/test surface for `call`/`do` statements. Recorded effect shapes (contract/README.md
/// harness rule 3): `{"effect": name, "args": [...]}`; calls as
/// `{"effect": "call", "op": id, "args": <argsRecord>}`.
///
/// Note: the wire/spec field is named `await`, but that's a Swift keyword inside an `async`
/// function body (ambiguous with the `await` expression keyword), so the parameter here is
/// `awaited` — same meaning, kept out of collision.
public protocol EffectSink: Sendable {
    /// The call-result record `{ok, value?, error?, fieldErrors?}`.
    func call(op: String, args: Value) async -> Value
    func perform(effect: String, args: [Value], awaited: Bool) async
    func error(_ message: String)
}
