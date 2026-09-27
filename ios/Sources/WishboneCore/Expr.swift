import Foundation

/// Wire expression AST (spec §3.8). Decoded from JSON arrays `["tag", ...]`.
public indirect enum Expr: Sendable {
    case literal(Value) // ["v", json]
    case state(String) // ["s", name]
    case instanceState(String) // ["i", name]
    case computed(String) // ["c", name]
    case data(String) // ["d", name]
    case local(String) // ["l", name]
    case param(String) // ["p", name]
    case env(String) // ["e", name]
    case token(group: String, name: String) // ["t", group, name]
    case driver(kind: String, id: String, axis: String) // ["dr", kind, id, axis]
    case selfPressed // ["self", "pressed"]
    case get(Expr, field: String) // ["get", obj, field]
    case at(Expr, Expr) // ["at", obj, key]  key: number (list) or string (map)
    case binary(op: String, Expr, Expr) // ["op", op, a, b]
    case unary(op: String, Expr) // ["un", op, a]
    case ternary(Expr, Expr, Expr) // ["if", c, a, b]
    case stdlib(name: String, args: [Expr]) // ["f", name, [args]]
    case userFn(name: String, args: [Expr]) // ["u", name, [args]]
    case lambda(params: [String], body: Expr) // ["lam", [params], body]
    case list([Expr]) // ["list", [e...]]
    case record([String: Expr]) // ["rec", {k: e}]
    case interpolate([Expr]) // ["str", [e...]]
    /// Unrecognized tag: decodes successfully (robustness rule), throws on evaluation.
    case unknown(String)
}

public enum ASTDecodeError: Error, Sendable, CustomStringConvertible {
    case malformed(String)

    public var description: String {
        switch self {
        case let .malformed(message): return "AST decode error: \(message)"
        }
    }
}

extension Expr {
    public init(json: Data) throws {
        self = try Expr(wire: try JSONValue.parse(json))
    }

    public init(json text: String) throws {
        self = try Expr(wire: try JSONValue.parse(text))
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

        switch tag {
        case "v":
            self = .literal(Value(wire: try arg(1)))
        case "s":
            self = .state(try str(1))
        case "i":
            self = .instanceState(try str(1))
        case "c":
            self = .computed(try str(1))
        case "d":
            self = .data(try str(1))
        case "l":
            self = .local(try str(1))
        case "p":
            self = .param(try str(1))
        case "e":
            self = .env(try str(1))
        case "t":
            self = .token(group: try str(1), name: try str(2))
        case "dr":
            self = .driver(kind: try str(1), id: try str(2), axis: try str(3))
        case "self":
            self = .selfPressed
        case "get":
            self = .get(try exprAt(1), field: try str(2))
        case "at":
            self = .at(try exprAt(1), try exprAt(2))
        case "op":
            self = .binary(op: try str(1), try exprAt(2), try exprAt(3))
        case "un":
            self = .unary(op: try str(1), try exprAt(2))
        case "if":
            self = .ternary(try exprAt(1), try exprAt(2), try exprAt(3))
        case "f":
            self = .stdlib(name: try str(1), args: try exprArray(2))
        case "u":
            self = .userFn(name: try str(1), args: try exprArray(2))
        case "lam":
            guard let paramsArr = try arg(1).asArray else { throw ASTDecodeError.malformed("lam: params not an array") }
            let params = try paramsArr.map { v -> String in
                guard let s = v.asString else { throw ASTDecodeError.malformed("lam: param not a string") }
                return s
            }
            self = .lambda(params: params, body: try exprAt(2))
        case "list":
            self = .list(try exprArray(1))
        case "rec":
            guard let obj = try arg(1).asObject else { throw ASTDecodeError.malformed("rec: element 1 is not an object") }
            var fields: [String: Expr] = [:]
            for (k, v) in obj { fields[k] = try Expr(wire: v) }
            self = .record(fields)
        case "str":
            self = .interpolate(try exprArray(1))
        default:
            self = .unknown(tag)
        }
    }
}
