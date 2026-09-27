import Foundation

/// `rep` — repeat binding on a node (spec §3.8 `"rep"`).
public struct RepeatBinding: Sendable {
    public let over: Expr
    public let asName: String
    public let indexName: String
    public let key: Expr?

    public init(over: Expr, asName: String, indexName: String, key: Expr?) {
        self.over = over
        self.asName = asName
        self.indexName = indexName
        self.key = key
    }
}

/// `a11y` — accessibility hints on a node.
public struct Accessibility: Sendable {
    public let label: Expr?
    public let role: String?
    public let hidden: Expr?

    public init(label: Expr?, role: String?, hidden: Expr?) {
        self.label = label
        self.role = role
        self.hidden = hidden
    }
}

/// A tree node (spec §3.8 "Узел"). `drag` and `tr` (transition) are literal JSON on the
/// wire (not DSL expressions) and are kept as raw `Value` blobs — their interpretation is a
/// renderer concern (tasks 9/10), not the interpreter's.
public struct Node: Sendable {
    public let type: String // "t" — kept verbatim even when unrecognized (renderer skips it)
    public let id: String
    public let props: [String: Expr] // "p"
    public let children: [Node] // "c"
    public let when: Expr?
    public let rep: RepeatBinding?
    public let on: [String: [Stmt]]
    public let animate: [String: Expr] // "anim": prop|"*" -> motion expr
    public let transitionIn: Value?
    public let transitionOut: Value?
    public let match: Expr?
    public let drag: Value?
    public let instanceState: [String: Expr] // "inst.state"
    public let a11y: Accessibility?

    public init(
        type: String, id: String, props: [String: Expr] = [:], children: [Node] = [],
        when: Expr? = nil, rep: RepeatBinding? = nil, on: [String: [Stmt]] = [:],
        animate: [String: Expr] = [:], transitionIn: Value? = nil, transitionOut: Value? = nil,
        match: Expr? = nil, drag: Value? = nil, instanceState: [String: Expr] = [:],
        a11y: Accessibility? = nil
    ) {
        self.type = type
        self.id = id
        self.props = props
        self.children = children
        self.when = when
        self.rep = rep
        self.on = on
        self.animate = animate
        self.transitionIn = transitionIn
        self.transitionOut = transitionOut
        self.match = match
        self.drag = drag
        self.instanceState = instanceState
        self.a11y = a11y
    }
}

extension Node {
    public init(json: Data) throws {
        self = try Node(wire: try JSONValue.parse(json))
    }

    init(wire: JSONValue) throws {
        guard let obj = wire.asObject else { throw ASTDecodeError.malformed("node: expected an object") }
        guard let type = obj["t"]?.asString else { throw ASTDecodeError.malformed("node: missing 't'") }
        guard let id = obj["id"]?.asString else { throw ASTDecodeError.malformed("node: missing 'id'") }

        var props: [String: Expr] = [:]
        if let p = obj["p"]?.asObject { for (k, v) in p { props[k] = try Expr(wire: v) } }

        var children: [Node] = []
        if let c = obj["c"]?.asArray { children = try c.map { try Node(wire: $0) } }

        let when: Expr? = try obj["when"].map { try Expr(wire: $0) }

        var rep: RepeatBinding? = nil
        if let r = obj["rep"]?.asObject {
            guard let overWire = r["over"] else { throw ASTDecodeError.malformed("rep: missing 'over'") }
            let over = try Expr(wire: overWire)
            let asName = r["as"]?.asString ?? "item"
            let indexName = r["index"]?.asString ?? "index"
            let key: Expr? = try r["key"].map { try Expr(wire: $0) }
            rep = RepeatBinding(over: over, asName: asName, indexName: indexName, key: key)
        }

        var on: [String: [Stmt]] = [:]
        if let onObj = obj["on"]?.asObject {
            for (event, stmtsWire) in onObj {
                guard let arr = stmtsWire.asArray else { throw ASTDecodeError.malformed("on.\(event): expected an array") }
                on[event] = try arr.map { try Stmt(wire: $0) }
            }
        }

        var animate: [String: Expr] = [:]
        if let animObj = obj["anim"]?.asObject { for (k, v) in animObj { animate[k] = try Expr(wire: v) } }

        let tr = obj["tr"]?.asObject
        let transitionIn: Value? = tr?["in"].map { Value(wire: $0) }
        let transitionOut: Value? = tr?["out"].map { Value(wire: $0) }

        let match: Expr? = try obj["match"].map { try Expr(wire: $0) }
        let drag: Value? = obj["drag"].map { Value(wire: $0) }

        var instanceState: [String: Expr] = [:]
        if let inst = obj["inst"]?.asObject, let stateObj = inst["state"]?.asObject {
            for (k, v) in stateObj { instanceState[k] = try Expr(wire: v) }
        }

        var a11y: Accessibility? = nil
        if let a = obj["a11y"]?.asObject {
            let label: Expr? = try a["label"].map { try Expr(wire: $0) }
            let hidden: Expr? = try a["hidden"].map { try Expr(wire: $0) }
            a11y = Accessibility(label: label, role: a["role"]?.asString, hidden: hidden)
        }

        self.init(
            type: type, id: id, props: props, children: children, when: when, rep: rep, on: on,
            animate: animate, transitionIn: transitionIn, transitionOut: transitionOut,
            match: match, drag: drag, instanceState: instanceState, a11y: a11y
        )
    }
}
