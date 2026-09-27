import Foundation
@testable import WishboneCore

/// Minimal theme (space/radius only — the interpreter never needs to resolve color/type/
/// icon/motion tokens to concrete platform values; that's the renderer's job) loaded from
/// contract/tokens/korpu.json, per core-decisions.md.
struct Theme {
    let space: [String: Double]
    let radius: [String: Double]

    static func loadKorpu() -> Theme {
        let url = repoRoot.appendingPathComponent("contract/tokens/korpu.json")
        guard let data = try? Data(contentsOf: url),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
        else {
            return Theme(space: [:], radius: [:])
        }
        func numDict(_ key: String) -> [String: Double] {
            guard let obj = json[key] as? [String: Any] else { return [:] }
            var result: [String: Double] = [:]
            for (k, v) in obj { if let n = v as? NSNumber { result[k] = n.doubleValue } }
            return result
        }
        return Theme(space: numDict("space"), radius: numDict("radius"))
    }
}

/// `ios/Tests/WishboneCoreTests/MapScope.swift` -> repo root (4 levels up).
let repoRoot: URL = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent() // WishboneCoreTests
    .deletingLastPathComponent() // Tests
    .deletingLastPathComponent() // ios
    .deletingLastPathComponent() // repo root

/// Default env per contract/README.md: `{now: 0, platform: "test", locale: "ru", brand:
/// "korpu", colorScheme: "light", reduceMotion: false, screenWidth: 390, screenHeight: 844,
/// safeTop: 0, safeBottom: 0, timeZone: "UTC"}`.
func defaultEnv(overrides: [String: Value] = [:]) -> [String: Value] {
    var env: [String: Value] = [
        "now": .number(0), "platform": .string("test"), "locale": .string("ru"),
        "brand": .string("korpu"), "colorScheme": .string("light"), "reduceMotion": .bool(false),
        "screenWidth": .number(390), "screenHeight": .number(844), "safeTop": .number(0),
        "safeBottom": .number(0), "timeZone": .string("UTC"),
    ]
    for (k, v) in overrides { env[k] = v }
    return env
}

/// `Scope` test double over plain dictionaries. `set` applies the same copy-on-write path
/// update the real store will use, and records every call for assertions.
@MainActor
final class MapScope: Scope {
    let interpreter: Interpreter
    let theme: Theme
    var stateValues: [String: Value]
    var instanceValues: [String: Value]
    var computedExprs: [String: Expr]
    var dataValues: [String: Value]
    var paramValues: [String: Value]
    var envValues: [String: Value]
    var driverValues: [String: Double]
    var pressed: Bool

    struct SetCall {
        let scope: String
        let name: String
        let path: [Value]
        let value: Value
        let transaction: Value?
    }

    private(set) var setCalls: [SetCall] = []

    init(
        interpreter: Interpreter = Interpreter(), theme: Theme = Theme.loadKorpu(),
        state: [String: Value] = [:], instance: [String: Value] = [:],
        computed: [String: Expr] = [:], data: [String: Value] = [:],
        params: [String: Value] = [:], env: [String: Value] = defaultEnv(),
        drivers: [String: Double] = [:], pressed: Bool = false
    ) {
        self.interpreter = interpreter
        self.theme = theme
        stateValues = state
        instanceValues = instance
        computedExprs = computed
        dataValues = data
        paramValues = params
        envValues = env
        driverValues = drivers
        self.pressed = pressed
    }

    /// Harness rule 1 (contract/README.md): initializes state from `document.state` in
    /// order (each initializer sees params/data/env/earlier state), then overlays
    /// `stateOverrides`.
    convenience init(
        document: Document, interpreter: Interpreter, params: [String: Value] = [:],
        data: [String: Value] = [:], env: [String: Value] = defaultEnv(),
        stateOverrides: [String: Value] = [:], theme: Theme = Theme.loadKorpu()
    ) {
        self.init(
            interpreter: interpreter, theme: theme, computed: document.computed,
            data: data, params: params, env: env
        )
        // Params not overridden by the vector fall back to their compiled default expr
        // (document.params holds each param's default, per spec `param id: string = "x"`).
        for (name, defaultExpr) in document.params where paramValues[name] == nil {
            paramValues[name] = (try? interpreter.eval(defaultExpr, scope: self)) ?? .null
        }
        for (name, initializer) in document.state {
            stateValues[name] = (try? interpreter.eval(initializer, scope: self)) ?? .null
        }
        for (k, v) in stateOverrides { stateValues[k] = v }
    }

    func state(_ name: String) -> Value { stateValues[name] ?? .null }
    func instanceState(_ name: String) -> Value { instanceValues[name] ?? .null }

    func computed(_ name: String) -> Value {
        guard let expr = computedExprs[name] else { return .null }
        return (try? interpreter.eval(expr, scope: self)) ?? .null
    }

    func data(_ name: String) -> Value { dataValues[name] ?? .null }
    func param(_ name: String) -> Value { paramValues[name] ?? .null }
    func env(_ name: String) -> Value { envValues[name] ?? .null }

    func token(_ group: String, _ name: String) -> Value {
        switch group {
        case "space": return .number(theme.space[name] ?? 0)
        case "radius": return .number(theme.radius[name] ?? 0)
        default: return .record(["$\(group)": .string(name)])
        }
    }

    func driver(_ kind: String, _ id: String, _ axis: String) -> Double {
        driverValues["\(kind):\(id):\(axis)"] ?? 0
    }

    func selfPressed() -> Bool { pressed }

    func set(scope: String, name: String, path: [Value], value: Value, transaction: Value?) {
        setCalls.append(SetCall(scope: scope, name: name, path: path, value: value, transaction: transaction))
        if scope == "i" {
            instanceValues[name] = (instanceValues[name] ?? .null).settingPath(path, to: value)
        } else {
            stateValues[name] = (stateValues[name] ?? .null).settingPath(path, to: value)
        }
    }
}

/// `EffectSink` test double implementing contract/README.md harness rule 3: effects recorded
/// as `{"effect": name, "args": [...]}`, calls as `{"effect": "call", "op": id, "args": ...}`,
/// answered from `calls[op]` (or a `NETWORK`/`no mock` failure).
///
/// Controller ruling: recording happens synchronously here regardless of `awaited`; when
/// `awaited` is `false`, this starts its own `Task` for the "real" work and returns `.null`
/// immediately instead of returning the resolved response.
final class RecordingEffectSink: EffectSink, @unchecked Sendable {
    private(set) var effects: [Value] = []
    private(set) var errors: [String] = []
    var calls: [String: Value] = [:]

    func call(op: String, args: Value, awaited: Bool) async -> Value {
        effects.append(.record(["effect": .string("call"), "op": .string(op), "args": args]))
        let response = calls[op] ?? .record([
            "ok": .bool(false),
            "error": .record(["code": .string("NETWORK"), "message": .string("no mock")]),
        ])
        guard awaited else {
            Task { _ = response } // fire-and-forget "real work"; the mock has none to do
            return .null
        }
        return response
    }

    func perform(effect: String, args: [Value], awaited: Bool) async {
        effects.append(.record(["effect": .string(effect), "args": .list(args)]))
    }

    func error(_ message: String) {
        errors.append(message)
    }
}
