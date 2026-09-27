import Foundation
import WishboneCore

/// Scope of one materialized node: the screen store plus repeat locals, the enclosing template
/// instances and per-node state. Expressions evaluate against it inside `body`, so SwiftUI
/// observes exactly the cells they read.
struct RenderContext: Scope {
    let store: Store
    var locals: Locals?
    var instance: InstanceScope?
    /// `.<repeatKey>` segments of enclosing repeats (a11y identifiers).
    var idSuffix = ""
    /// Axis of the enclosing stack/scroll ("v"/"h"), for `spacer`.
    var parentAxis: String?
    var pressed = false
    var isRoot = false

    init(store: Store) {
        self.store = store
    }

    func state(_ name: String) -> Value { store.state[name]?.value ?? .null }
    func instanceState(_ name: String) -> Value { instance?.cell(name)?.value ?? .null }
    func computed(_ name: String) -> Value { store.computed(name) }
    func data(_ name: String) -> Value { store.dataValue(name) }
    func param(_ name: String) -> Value { store.params[name] ?? .null }
    func env(_ name: String) -> Value { store.wishbone.env.value(name, clock: store.wishbone.clock, brand: store.wishbone.brand) }
    func token(_ group: String, _ name: String) -> Value { store.token(group, name) }
    // ponytail: drivers arrive with motion (phase b).
    func driver(_ kind: String, _ id: String, _ axis: String) -> Double { 0 }
    func selfPressed() -> Bool { pressed }

    func set(scope: String, name: String, path: [Value], value: Value, transaction: Value?) {
        guard let cell = scope == "i" ? instance?.cell(name) : store.state[name] else {
            return store.log("assignment to undeclared \(scope == "i" ? "instance " : "")state \(name)")
        }
        cell.value = cell.value.settingPath(path, to: value)
    }

    func child(locals: Locals? = nil, instance: InstanceScope? = nil, idSuffix: String = "", axis: String?) -> RenderContext {
        var c = self
        if let locals { c.locals = locals }
        if let instance { c.instance = instance }
        c.idSuffix += idSuffix
        c.parentAxis = axis
        c.isRoot = false
        return c
    }
}

/// Effects of handler runs (renderer-decisions: Navigation & effects).
struct StoreEffects: EffectSink {
    let store: Store

    func call(op: String, args: Value, awaited: Bool) async -> Value {
        await store.call(op: op, args: args, awaited: awaited)
    }

    func perform(effect: String, args: [Value], awaited: Bool) async {
        await store.perform(effect: effect, args: args, awaited: awaited)
    }

    func error(_ message: String) {
        Task { @MainActor in store.log(message) }
    }
}

extension Store {
    func perform(effect: String, args: [Value], awaited: Bool) async {
        func str(_ i: Int) -> String? { if i < args.count, case let .string(s) = args[i] { s } else { nil } }
        func rec(_ i: Int) -> [String: Value] { if i < args.count, case let .record(r) = args[i] { r } else { [:] } }
        let host = wishbone.host
        switch effect {
        case "navigate":
            guard let id = str(0) else { return }
            navigator?.navigate(Route(screen: id, params: rec(1), fade: str(2) == "fade"))
        case "back":
            navigator?.back()
        case "present":
            guard let id = str(0) else { return }
            navigator?.present(Route(screen: id, params: rec(1)))
        case "dismiss":
            navigator?.dismiss()
        case "open":
            // `/…` is the Wishbone server's own path; otherwise https or a host-allowed scheme.
            let raw = str(0) ?? "", local = raw.hasPrefix("/")
            guard let url = URL(string: raw, relativeTo: wishbone.baseURL)?.absoluteURL, let scheme = url.scheme?.lowercased(),
                  local || scheme == "https" || host.allowedSchemes.contains(scheme)
            else { return log("open: ignored \(raw)", level: "warn") }
            host.open(url: url)
        case "haptic":
            Haptics.play(str(0) ?? "light")
        case "track":
            host.track(event: str(0) ?? "", params: rec(1).mapValues(\.foundation))
        case "host":
            let name = str(0) ?? "", hostArgs = rec(1).mapValues(\.foundation)
            if awaited { await host.action(name: name, args: hostArgs) } else { Task { await host.action(name: name, args: hostArgs) } }
        case "wait":
            if args.count > 0, case let .number(ms) = args[0], ms.isFinite, ms > 0 {
                try? await Task.sleep(for: .milliseconds(Int(min(ms, 86_400_000))))
            }
        case "refresh":
            if let name = str(0) { await refresh(name) }
        case "play", "stop":
            break // ponytail: timelines arrive with motion (phase b).
        default:
            log("unknown effect \(effect)")
        }
    }
}

extension Value {
    /// Foundation form for the host API (`[String: Any]` payloads).
    var foundation: Any {
        switch self {
        case .null, .lambda: NSNull()
        case let .bool(b): b
        case let .number(n): n
        case let .string(s): s
        case let .list(l): l.map(\.foundation)
        case let .record(r): r.mapValues(\.foundation)
        }
    }
}
