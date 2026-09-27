import Foundation

/// `fns.<name>` — a pure user function: evaluates its body with only its own params as
/// locals (no access to the caller's locals).
public struct Fn: Sendable {
    public let params: [String]
    public let body: Expr

    public init(params: [String], body: Expr) {
        self.params = params
        self.body = body
    }
}

/// `handlers.<name>`.
public struct Handler: Sendable {
    public let params: [String]
    public let body: [Stmt]

    public init(params: [String], body: [Stmt]) {
        self.params = params
        self.body = body
    }
}

/// `data[]` — a data source binding. `args` are plain JSON values, not DSL expressions: per
/// spec, `data` arguments may only reference literals and params, and the compiler lowers a
/// param reference to a `{"$param": name}` marker object rather than an `["p", name]` expr —
/// so this decodes with `Value(wire:)`, not `Expr(wire:)`. The store resolves `$param` markers
/// against the document's actual param values when it runs the operation.
public struct DataDecl: Sendable {
    public let name: String
    public let op: String
    public let args: [String: Value]
    public let required: Bool

    public init(name: String, op: String, args: [String: Value], required: Bool) {
        self.name = name
        self.op = op
        self.args = args
        self.required = required
    }
}

/// One keyframe on a timeline track. `ease` is a resolved 4-number cubic-bezier (or `nil`
/// for linear) — named eases are already expanded by the compiler before the wire form.
public struct TimelineKey: Sendable {
    public let t: Double
    public let v: Double
    public let ease: [Double]?

    public init(t: Double, v: Double, ease: [Double]?) {
        self.t = t
        self.v = v
        self.ease = ease
    }
}

public struct TimelineTrack: Sendable {
    public let target: String
    public let prop: String
    public let stagger: Double
    public let keys: [TimelineKey]

    public init(target: String, prop: String, stagger: Double, keys: [TimelineKey]) {
        self.target = target
        self.prop = prop
        self.stagger = stagger
        self.keys = keys
    }
}

/// `timelines.<name>`. `duration` is `D` in the sampling algorithm (contract/README.md):
/// the compiler pre-computes it as `max(key.t)` across all tracks. `autoplay` (renderer
/// concern, tasks 9/10): the timeline is considered running from the document's first render,
/// without needing an explicit `play()` — carried through here even though `TimelineSampler`
/// itself doesn't consult it.
public struct Timeline: Sendable {
    public let name: String
    public let duration: Double
    public let repeatCount: Int // -1 = infinite
    public let autoreverse: Bool
    public let autoplay: Bool
    public let tracks: [TimelineTrack]

    public init(name: String, duration: Double, repeatCount: Int, autoreverse: Bool, autoplay: Bool = false, tracks: [TimelineTrack]) {
        self.name = name
        self.duration = duration
        self.repeatCount = repeatCount
        self.autoreverse = autoreverse
        self.autoplay = autoplay
        self.tracks = tracks
    }
}

/// A compiled screen document (spec §3.8 "Документ").
public struct Document: Sendable {
    public let format: Int
    public let runtimeMin: String
    public let screen: String
    public let revision: Int
    public let params: [String: Expr]
    /// Ordered: initializers see earlier entries, in declaration order.
    public let state: [(name: String, initial: Expr)]
    public let computed: [String: Expr]
    public let fns: [String: Fn]
    public let handlers: [String: Handler]
    public let data: [DataDecl]
    public let timelines: [String: Timeline]
    public let root: Node

    public init(
        format: Int, runtimeMin: String, screen: String, revision: Int,
        params: [String: Expr], state: [(name: String, initial: Expr)], computed: [String: Expr],
        fns: [String: Fn], handlers: [String: Handler], data: [DataDecl],
        timelines: [String: Timeline], root: Node
    ) {
        self.format = format
        self.runtimeMin = runtimeMin
        self.screen = screen
        self.revision = revision
        self.params = params
        self.state = state
        self.computed = computed
        self.fns = fns
        self.handlers = handlers
        self.data = data
        self.timelines = timelines
        self.root = root
    }
}

extension Timeline {
    init(name: String, wire: JSONValue) throws {
        guard let obj = wire.asObject else { throw ASTDecodeError.malformed("timeline \(name): expected an object") }
        let duration = obj["duration"]?.asNumber ?? 0
        let repeatCount = obj["repeat"].flatMap { $0.asNumber.map { SafeNumeric.int($0, fallback: 1) } } ?? 1
        let autoreverse = obj["autoreverse"]?.asBool ?? false
        let autoplay = obj["autoplay"]?.asBool ?? false
        guard let tracksWire = obj["tracks"]?.asArray else { throw ASTDecodeError.malformed("timeline \(name): missing 'tracks'") }
        let tracks: [TimelineTrack] = try tracksWire.map { trackWire in
            guard let tObj = trackWire.asObject, let target = tObj["target"]?.asString, let prop = tObj["prop"]?.asString else {
                throw ASTDecodeError.malformed("timeline \(name): malformed track")
            }
            let stagger = tObj["stagger"]?.asNumber ?? 0
            guard let keysWire = tObj["keys"]?.asArray else { throw ASTDecodeError.malformed("timeline \(name): track missing 'keys'") }
            let keys: [TimelineKey] = try keysWire.map { keyWire in
                guard let kObj = keyWire.asObject, let t = kObj["t"]?.asNumber, let v = kObj["v"]?.asNumber else {
                    throw ASTDecodeError.malformed("timeline \(name): malformed key")
                }
                let ease: [Double]? = kObj["ease"]?.asArray?.compactMap { $0.asNumber }
                return TimelineKey(t: t, v: v, ease: ease)
            }
            // Sorted once here — TimelineSampler assumes `keys` is already ascending by `t`.
            return TimelineTrack(target: target, prop: prop, stagger: stagger, keys: keys.sorted { $0.t < $1.t })
        }
        self.init(name: name, duration: duration, repeatCount: repeatCount, autoreverse: autoreverse, autoplay: autoplay, tracks: tracks)
    }
}

extension Document {
    public init(json: Data) throws {
        self = try Document(wire: try JSONValue.parse(json))
    }

    public init(json text: String) throws {
        self = try Document(wire: try JSONValue.parse(text))
    }

    init(wire: JSONValue) throws {
        guard let obj = wire.asObject else { throw ASTDecodeError.malformed("document: expected an object") }
        guard let format = obj["format"]?.asNumber else { throw ASTDecodeError.malformed("document: missing 'format'") }
        guard let screen = obj["screen"]?.asString else { throw ASTDecodeError.malformed("document: missing 'screen'") }
        guard let rootWire = obj["root"] else { throw ASTDecodeError.malformed("document: missing 'root'") }
        let root = try Node(wire: rootWire)
        let runtimeMin = obj["runtimeMin"]?.asString ?? "1.0.0"
        let revision = obj["revision"].flatMap { $0.asNumber.map { SafeNumeric.int($0) } } ?? 0

        var params: [String: Expr] = [:]
        if let p = obj["params"]?.asObject { for (k, v) in p { params[k] = try Expr(wire: v) } }

        var state: [(name: String, initial: Expr)] = []
        if let stateArr = obj["state"]?.asArray {
            for entry in stateArr {
                guard let pair = entry.asArray, pair.count == 2, let name = pair[0].asString else {
                    throw ASTDecodeError.malformed("document.state: malformed entry")
                }
                state.append((name: name, initial: try Expr(wire: pair[1])))
            }
        }

        var computed: [String: Expr] = [:]
        if let c = obj["computed"]?.asObject { for (k, v) in c { computed[k] = try Expr(wire: v) } }

        var fns: [String: Fn] = [:]
        if let f = obj["fns"]?.asObject {
            for (name, fnWire) in f {
                guard let fnObj = fnWire.asObject, let paramsArr = fnObj["params"]?.asArray, let bodyWire = fnObj["body"] else {
                    throw ASTDecodeError.malformed("fns.\(name): malformed")
                }
                let ps = paramsArr.compactMap { $0.asString }
                fns[name] = Fn(params: ps, body: try Expr(wire: bodyWire))
            }
        }

        var handlers: [String: Handler] = [:]
        if let h = obj["handlers"]?.asObject {
            for (name, hWire) in h {
                guard let hObj = hWire.asObject, let paramsArr = hObj["params"]?.asArray, let bodyArr = hObj["body"]?.asArray else {
                    throw ASTDecodeError.malformed("handlers.\(name): malformed")
                }
                let ps = paramsArr.compactMap { $0.asString }
                handlers[name] = Handler(params: ps, body: try bodyArr.map { try Stmt(wire: $0) })
            }
        }

        var data: [DataDecl] = []
        if let dataArr = obj["data"]?.asArray {
            for entry in dataArr {
                guard let dObj = entry.asObject, let name = dObj["name"]?.asString, let op = dObj["op"]?.asString else {
                    throw ASTDecodeError.malformed("document.data: malformed entry")
                }
                var args: [String: Value] = [:]
                if let argsObj = dObj["args"]?.asObject { for (k, v) in argsObj { args[k] = Value(wire: v) } }
                data.append(DataDecl(name: name, op: op, args: args, required: dObj["required"]?.asBool ?? false))
            }
        }

        var timelines: [String: Timeline] = [:]
        if let t = obj["timelines"]?.asObject {
            for (name, tWire) in t { timelines[name] = try Timeline(name: name, wire: tWire) }
        }

        self.init(
            format: SafeNumeric.int(format, fallback: 1), runtimeMin: runtimeMin, screen: screen, revision: revision,
            params: params, state: state, computed: computed, fns: fns, handlers: handlers,
            data: data, timelines: timelines, root: root
        )
    }
}
