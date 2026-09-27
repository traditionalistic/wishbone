import Foundation

/// Wire statement AST (spec §3.8), used in handler/event/timeline-effect bodies.
public indirect enum Stmt: Sendable {
    case set(scope: String, name: String, path: [Expr], value: Expr) // ["set", "s"|"i", name, [path...], expr]
    case letStmt(name: String, value: Expr) // ["let", name, expr]
    case ifStmt(Expr, then: [Stmt], else: [Stmt]) // ["if", cond, [then...], [else...]]
    case anim(motion: Expr, body: [Stmt]) // ["anim", motionExpr, [body...]]
    case call(op: String, args: Expr, resultName: String?, awaited: Bool) // ["call", opId, argsExpr, resultName|null, await]
    case doEffect(effect: String, args: [Expr], awaited: Bool) // ["do", effect, [args...], await]
    case run(handler: String, args: [Expr]) // ["run", handler, [args...]]
    /// Unrecognized tag: decodes successfully, throws (aborting the run) on execution.
    case unknown(String)
}

extension Stmt {
    public init(json: Data) throws {
        self = try Stmt(wire: try JSONValue.parse(json))
    }

    public init(json text: String) throws {
        self = try Stmt(wire: try JSONValue.parse(text))
    }

    static func array(json: Data) throws -> [Stmt] {
        guard let arr = try JSONValue.parse(json).asArray else {
            throw ASTDecodeError.malformed("expected a JSON array of statements")
        }
        return try arr.map { try Stmt(wire: $0) }
    }

    init(wire: JSONValue) throws {
        guard case let .array(elements) = wire, let first = elements.first, case let .string(tag) = first else {
            throw ASTDecodeError.malformed("expected a non-empty JSON array with a string tag, got \(wire)")
        }
        func arg(_ i: Int) throws -> JSONValue {
            guard i < elements.count else { throw ASTDecodeError.malformed("\(tag): missing element \(i)") }
            return elements[i]
        }
        func str(_ i: Int) throws -> String {
            guard let s = try arg(i).asString else { throw ASTDecodeError.malformed("\(tag): element \(i) is not a string") }
            return s
        }
        func exprAt(_ i: Int) throws -> Expr { try Expr(wire: try arg(i)) }
        func exprArray(_ i: Int) throws -> [Expr] {
            guard let arr = try arg(i).asArray else { throw ASTDecodeError.malformed("\(tag): element \(i) is not an array") }
            return try arr.map { try Expr(wire: $0) }
        }
        func stmtArray(_ i: Int) throws -> [Stmt] {
            guard let arr = try arg(i).asArray else { throw ASTDecodeError.malformed("\(tag): element \(i) is not an array") }
            return try arr.map { try Stmt(wire: $0) }
        }
        func boolAt(_ i: Int) throws -> Bool {
            guard let b = try arg(i).asBool else { throw ASTDecodeError.malformed("\(tag): element \(i) is not a bool") }
            return b
        }

        switch tag {
        case "set":
            self = .set(scope: try str(1), name: try str(2), path: try exprArray(3), value: try exprAt(4))
        case "let":
            self = .letStmt(name: try str(1), value: try exprAt(2))
        case "if":
            self = .ifStmt(try exprAt(1), then: try stmtArray(2), else: try stmtArray(3))
        case "anim":
            self = .anim(motion: try exprAt(1), body: try stmtArray(2))
        case "call":
            let resultNameValue = try arg(3)
            let resultName: String? = resultNameValue.isNull ? nil : resultNameValue.asString
            self = .call(op: try str(1), args: try exprAt(2), resultName: resultName, awaited: try boolAt(4))
        case "do":
            self = .doEffect(effect: try str(1), args: try exprArray(2), awaited: try boolAt(3))
        case "run":
            self = .run(handler: try str(1), args: try exprArray(2))
        default:
            self = .unknown(tag)
        }
    }
}
