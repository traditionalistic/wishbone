import Foundation
import Testing
@testable import WishboneCore

@Suite struct ExprDecodingTests {
    @Test func decodesLiteralAndReferenceForms() throws {
        #expect(try Expr(json: #"["v", 1]"#).isLiteral(1))
        if case let .state(n) = try Expr(json: #"["s", "count"]"#) { #expect(n == "count") } else { Issue.record("expected .state") }
        if case let .instanceState(n) = try Expr(json: #"["i", "on"]"#) { #expect(n == "on") } else { Issue.record("expected .instanceState") }
        if case let .computed(n) = try Expr(json: #"["c", "total"]"#) { #expect(n == "total") } else { Issue.record("expected .computed") }
        if case let .data(n) = try Expr(json: #"["d", "me"]"#) { #expect(n == "me") } else { Issue.record("expected .data") }
        if case let .local(n) = try Expr(json: #"["l", "x"]"#) { #expect(n == "x") } else { Issue.record("expected .local") }
        if case let .param(n) = try Expr(json: #"["p", "id"]"#) { #expect(n == "id") } else { Issue.record("expected .param") }
        if case let .env(n) = try Expr(json: #"["e", "platform"]"#) { #expect(n == "platform") } else { Issue.record("expected .env") }
    }

    @Test func decodesTokenAndDriverForms() throws {
        if case let .token(g, n) = try Expr(json: #"["t", "space", "lg"]"#) {
            #expect(g == "space"); #expect(n == "lg")
        } else { Issue.record("expected .token") }
        if case let .driver(k, id, axis) = try Expr(json: #"["dr", "scroll", "page", "y"]"#) {
            #expect(k == "scroll"); #expect(id == "page"); #expect(axis == "y")
        } else { Issue.record("expected .driver") }
        if case .selfPressed = try Expr(json: #"["self", "pressed"]"#) {} else { Issue.record("expected .selfPressed") }
    }

    @Test func decodesGetAtOpUnIfFAndU() throws {
        if case let .get(_, field) = try Expr(json: #"["get", ["l", "x"], "name"]"#) { #expect(field == "name") }
        else { Issue.record("expected .get") }
        if case .at = try Expr(json: #"["at", ["l", "xs"], ["v", 0]]"#) {} else { Issue.record("expected .at") }
        if case let .binary(op, _, _) = try Expr(json: #"["op", "+", ["v", 1], ["v", 2]]"#) { #expect(op == "+") }
        else { Issue.record("expected .binary") }
        if case let .unary(op, _) = try Expr(json: #"["un", "-", ["v", 1]]"#) { #expect(op == "-") }
        else { Issue.record("expected .unary") }
        if case .ternary = try Expr(json: #"["if", ["v", true], ["v", 1], ["v", 2]]"#) {} else { Issue.record("expected .ternary") }
        if case let .stdlib(name, args) = try Expr(json: #"["f", "min", [["v", 1], ["v", 2]]]"#) {
            #expect(name == "min"); #expect(args.count == 2)
        } else { Issue.record("expected .stdlib") }
        if case let .userFn(name, args) = try Expr(json: #"["u", "tax", [["v", 1]]]"#) {
            #expect(name == "tax"); #expect(args.count == 1)
        } else { Issue.record("expected .userFn") }
    }

    @Test func decodesLambdaListRecordInterpolate() throws {
        if case let .lambda(params, _) = try Expr(json: #"["lam", ["x", "i"], ["l", "x"]]"#) { #expect(params == ["x", "i"]) }
        else { Issue.record("expected .lambda") }
        if case let .list(items) = try Expr(json: #"["list", [["v", 1], ["v", 2]]]"#) { #expect(items.count == 2) }
        else { Issue.record("expected .list") }
        if case let .record(fields) = try Expr(json: #"["rec", {"a": ["v", 1]}]"#) { #expect(fields["a"] != nil) }
        else { Issue.record("expected .record") }
        if case let .interpolate(parts) = try Expr(json: #"["str", [["v", "a"], ["v", 1]]]"#) { #expect(parts.count == 2) }
        else { Issue.record("expected .interpolate") }
    }

    @Test func unknownTagDecodesButThrowsOnEval() throws {
        let expr = try Expr(json: #"["totallyNewForm", 1, 2]"#)
        if case let .unknown(tag) = expr { #expect(tag == "totallyNewForm") } else { Issue.record("expected .unknown") }
        let interpreter = Interpreter()
        let scope = MapScope()
        #expect(throws: (any Error).self) { try interpreter.eval(expr, scope: scope) }
    }

    @Test func malformedShapeThrowsOnDecode() {
        #expect(throws: (any Error).self) { try Expr(json: #"["get", ["l", "x"]]"#) } // missing field
        #expect(throws: (any Error).self) { try Expr(json: #"42"#) } // not a tagged array
    }

    @Test func decodesStatementForms() throws {
        if case let .set(scope, name, path, _) = try Stmt(json: #"["set", "s", "count", [], ["v", 1]]"#) {
            #expect(scope == "s"); #expect(name == "count"); #expect(path.isEmpty)
        } else { Issue.record("expected .set") }
        if case .ifStmt(_, let thenB, let elseB) = try Stmt(json: #"["if", ["v", true], [["let", "x", ["v", 1]]], []]"#) {
            #expect(thenB.count == 1); #expect(elseB.isEmpty)
        } else { Issue.record("expected .ifStmt") }
        if case let .call(op, _, resultName, awaited) = try Stmt(json: #"["call", "demo.x", ["v", {}], "r", true]"#) {
            #expect(op == "demo.x"); #expect(resultName == "r"); #expect(awaited)
        } else { Issue.record("expected .call") }
        if case let .call(_, _, resultName, _) = try Stmt(json: #"["call", "demo.x", ["v", {}], null, false]"#) {
            #expect(resultName == nil)
        } else { Issue.record("expected .call with null result") }
        if case let .doEffect(effect, args, awaited) = try Stmt(json: #"["do", "wait", [["v", 300]], true]"#) {
            #expect(effect == "wait"); #expect(args.count == 1); #expect(awaited)
        } else { Issue.record("expected .doEffect") }
        if case let .run(handler, args) = try Stmt(json: #"["run", "inc", [["v", 1]]]"#) {
            #expect(handler == "inc"); #expect(args.count == 1)
        } else { Issue.record("expected .run") }
    }

    @Test func decodesNodeWithRepeatOnAndA11y() throws {
        let json = #"""
        {"t": "text", "id": "chip",
         "p": {"text": ["l", "p"]},
         "rep": {"over": ["d", "plans"], "as": "p"},
         "on": {"tap": [["set", "s", "picked", [], ["get", ["l", "p"], "id"]]]},
         "a11y": {"label": ["v", "x"], "role": "button"}}
        """#
        let node = try Node(json: Data(json.utf8))
        #expect(node.type == "text")
        #expect(node.id == "chip")
        #expect(node.rep?.asName == "p")
        #expect(node.rep?.indexName == "index") // default
        #expect(node.on["tap"]?.count == 1)
        #expect(node.a11y?.role == "button")
    }

    @Test func decodesMinimalDocument() throws {
        let json = #"""
        {"format": 1, "screen": "s", "revision": 1,
         "state": [["page", ["v", 0]]],
         "computed": {"last": ["op", "==", ["s", "page"], ["v", 2]]},
         "handlers": {"go": {"params": [], "body": [["set", "s", "page", [], ["v", 1]]]}},
         "root": {"t": "stack", "id": "root", "c": []}}
        """#
        let doc = try Document(json: Data(json.utf8))
        #expect(doc.screen == "s")
        #expect(doc.state.count == 1)
        #expect(doc.state[0].name == "page")
        #expect(doc.computed["last"] != nil)
        #expect(doc.handlers["go"]?.body.count == 1)
        #expect(doc.root.id == "root")
    }
}

private extension Expr {
    func isLiteral(_ n: Double) -> Bool {
        if case let .literal(.number(x)) = self { return x == n }
        return false
    }
}
