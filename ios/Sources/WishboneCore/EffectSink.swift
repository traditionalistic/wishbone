import Foundation

/// Host/test surface for `call`/`do` statements. Recorded effect shapes (contract/README.md
/// harness rule 3): `{"effect": name, "args": [...]}`; calls as
/// `{"effect": "call", "op": id, "args": <argsRecord>}`.
///
/// Note: the wire/spec field is named `await`, but that's a Swift keyword inside an `async`
/// function body (ambiguous with the `await` expression keyword), so the parameter here is
/// `awaited` — same meaning, kept out of collision.
///
/// Controller ruling on `call`/`awaited`: the interpreter always calls `call(op:args:awaited:)`
/// synchronously, in statement order — recording the effect is this method's job, and it
/// happens whether or not the call is awaited. When `awaited` is `false`, this method is
/// expected to start its own (unstructured) `Task` for the real work and return `.null`
/// immediately, rather than suspending the caller; when `true`, it awaits the real result and
/// returns it. (The Android `:wishbone-core` gets the identical signature/contract.)
public protocol EffectSink: Sendable {
    /// The call-result record `{ok, value?, error?, fieldErrors?}` — `.null` if `awaited` is
    /// `false` (see above).
    func call(op: String, args: Value, awaited: Bool) async -> Value
    func perform(effect: String, args: [Value], awaited: Bool) async
    func error(_ message: String)
}
