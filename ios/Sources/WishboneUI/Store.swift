import Foundation
import Observation
import WishboneCore

/// One observable value per state key: views re-render only for the cells their expressions read.
@MainActor @Observable
final class Cell {
    var value: Value
    init(_ value: Value) { self.value = value }
}

/// Template instance state (`inst.state`), owned by the materialized view; lookups walk up the
/// chain of enclosing instances until one declares the (template-qualified) name.
@MainActor
final class InstanceScope {
    let parent: InstanceScope?
    let cells: [String: Cell]

    init(parent: InstanceScope?, values: [String: Value]) {
        self.parent = parent
        cells = values.mapValues(Cell.init)
    }

    func cell(_ name: String) -> Cell? { cells[name] ?? parent?.cell(name) }
}

/// Runtime state of one rendered screen document (renderer-decisions: Store).
@MainActor
final class Store {
    let wishbone: Wishbone
    let document: Document
    let interpreter: Interpreter
    let navigator: Navigator?
    private(set) var params: [String: Value] = [:]
    private(set) var state: [String: Cell] = [:]
    private var data: [String: Cell] = [:]
    /// Template instance state by node id + repeat key path (survives push/pop and `when` toggles).
    private var instances: [String: InstanceScope] = [:]
    /// The previous document's instances: values of names that still exist carry over on reload.
    private let previousInstances: [String: InstanceScope]

    /// `previous` — the store of the same screen before a document reload: state keys that still
    /// exist keep their values.
    init(document: Document, params: [String: Value], data results: [String: Value], wishbone: Wishbone,
         navigator: Navigator? = nil, previous: Store? = nil) {
        self.document = document
        self.wishbone = wishbone
        self.navigator = navigator
        interpreter = Interpreter(document: document)
        previousInstances = previous?.instances ?? [:]
        let root = RenderContext(store: self)
        self.params = params
        for (name, def) in document.params where params[name] == nil { self.params[name] = root.eval(def) ?? .null }
        for decl in document.data { data[decl.name] = Cell(.null) }
        update(data: results)
        for (name, initial) in document.state {
            state[name] = Cell(previous?.state[name]?.value ?? root.eval(initial) ?? .null)
        }
    }

    func update(data results: [String: Value]) {
        for (name, result) in results {
            let value = ScreenResponse.dataValue(result)
            if let cell = data[name] { cell.value = value } else { data[name] = Cell(value) }
        }
    }

    func dataValue(_ name: String) -> Value { data[name]?.value ?? .null }

    /// The instance scope of a node carrying `inst`, created on first render with its initializers.
    /// ponytail: instances of removed repeat items stay for the screen's lifetime.
    func instance(_ node: Node, ctx: RenderContext) -> InstanceScope {
        let key = node.id + ctx.idSuffix
        if let scope = instances[key] { return scope }
        let old = previousInstances[key]
        let scope = InstanceScope(parent: ctx.instance, values: node.instanceState.reduce(into: [:]) { values, entry in
            values[entry.key] = old?.cells[entry.key]?.value ?? ctx.eval(entry.value) ?? .null
        })
        instances[key] = scope
        return scope
    }

    func computed(_ name: String) -> Value {
        guard let expr = document.computed[name] else { return .null }
        return RenderContext(store: self).eval(expr) ?? .null
    }

    func token(_ group: String, _ name: String) -> Value {
        switch group {
        case "space": .number(wishbone.theme.space[name] ?? 0)
        case "radius": .number(wishbone.theme.radius[name] ?? 0)
        default: .record(["$\(group)": .string(name)])
        }
    }

    func log(_ message: String, level: String = "error") {
        wishbone.log(level, "\(document.screen): \(message)")
    }

    private var warned: Set<String> = []

    /// For conditions every render would repeat (unknown primitive, missing adapter).
    func warnOnce(_ message: String) {
        if warned.insert(message).inserted { log(message, level: "warn") }
    }

    // MARK: handlers

    /// Runs an `on.*` body with `event` bound, detached from the view update.
    @discardableResult
    func fire(_ stmts: [Stmt], ctx: RenderContext, event: Value) -> Task<Void, Never> {
        let locals = Locals(name: "event", value: event, parent: ctx.locals)
        let effects = StoreEffects(store: self)
        return Task { await interpreter.runStatements(stmts, scope: ctx, locals: locals, effects: effects) }
    }

    func call(op: String, args: Value, awaited: Bool) async -> Value {
        guard awaited else {
            Task { _ = await self.sendCall(op: op, args: args) }
            return .null
        }
        return await sendCall(op: op, args: args)
    }

    private func sendCall(op: String, args: Value) async -> Value {
        let req = API.call(await wishbone.requestInfo(), screen: document.screen, revision: document.revision,
                           op: op, args: args, key: UUID().uuidString)
        do {
            let (body, _) = try await API.send(req)
            return try Value(json: body ?? Data())
        } catch {
            log("call \(op): \(error)", level: "warn")
            return API.callFailure(error)
        }
    }

    func refresh(_ name: String) async {
        let req = API.data(await wishbone.requestInfo(), screen: document.screen, revision: document.revision,
                           params: params, sources: [name])
        do {
            let (body, _) = try await API.send(req)
            if let body { update(data: try API.parseData(body)) }
        } catch {
            log("refresh \(name): \(error)", level: "warn")
        }
    }
}

extension RenderContext {
    /// Evaluates for rendering; errors are logged and yield nil (the caller renders nothing).
    func eval(_ expr: Expr) -> Value? {
        do {
            return try store.interpreter.eval(expr, scope: self, locals: locals)
        } catch {
            store.log(String(describing: error))
            return nil
        }
    }
}
