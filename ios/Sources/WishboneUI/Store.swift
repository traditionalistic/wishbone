import Foundation
import Observation
import Synchronization
import WishboneCore

/// Handler runs of one screen, cancelled when its store is discarded (an `await play` of an
/// infinite timeline, a long `wait`, … must not outlive the screen).
final class TaskBag: Sendable {
    private let tasks = Mutex<[UUID: Task<Void, Never>]>([:])

    func add(_ id: UUID, _ task: Task<Void, Never>) { tasks.withLock { $0[id] = task } }
    func remove(_ id: UUID) { _ = tasks.withLock { $0.removeValue(forKey: id) } }
    var count: Int { tasks.withLock { $0.count } }

    func cancelAll() {
        let running = tasks.withLock { t in defer { t.removeAll() }; return Array(t.values) }
        running.forEach { $0.cancel() }
    }
}

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
    /// The stack's geometry, or a default one for a store rendered outside a stack (tests).
    let geometry: ScreenGeometry
    private(set) var params: [String: Value] = [:]
    private(set) var state: [String: Cell] = [:]
    private var data: [String: Cell] = [:]
    /// Template instance state by node id + repeat key path (survives push/pop and `when` toggles).
    private var instances: [String: InstanceScope] = [:]
    /// The previous document's instances: values of names that still exist carry over on reload.
    private let previousInstances: [String: InstanceScope]
    /// `scroll.<id>.x|y` and `drag.<id+repeat path>.x|y` cells, created on first read or write.
    private var drivers: [String: Cell] = [:]
    /// Changes whenever a drag cell is created.
    private let dragCells = Cell(.number(0))
    let timelines: Timelines
    /// The screen has `drag` nodes (the full-width swipe-back would fight them).
    let hasDrag: Bool

    /// `previous` — the store of the same screen before a document reload: state keys that still
    /// exist keep their values.
    let tasks: TaskBag

    init(document: Document, params: [String: Value], data results: [String: Value], wishbone: Wishbone,
         navigator: Navigator? = nil, previous: Store? = nil, tasks: TaskBag = TaskBag()) {
        self.document = document
        self.tasks = tasks
        self.wishbone = wishbone
        self.navigator = navigator
        geometry = navigator?.geometry ?? ScreenGeometry()
        interpreter = Interpreter(document: document)
        previousInstances = previous?.instances ?? [:]
        let env = wishbone.env
        timelines = Timelines(document.timelines, previous: previous?.timelines, reduceMotion: { env.reduceMotion })
        func anyDrag(_ n: Node) -> Bool { n.drag != nil || n.children.contains(where: anyDrag) }
        hasDrag = anyDrag(document.root)
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

    func driver(_ key: String) -> Cell {
        if let cell = drivers[key] { return cell }
        let cell = Cell(.number(0))
        drivers[key] = cell
        if key.hasPrefix("drag:") { dragCells.value = .number(Double(drivers.count)) }
        return cell
    }

    /// An existing cell, or nil after subscribing to drag-cell creation (so the reader re-reads once it exists).
    func existingDriver(_ key: String) -> Cell? {
        if let cell = drivers[key] { return cell }
        _ = dragCells.value
        return nil
    }

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

    private var logged: Set<String> = []

    /// For conditions every render would repeat (eval errors, unknown primitive, missing adapter).
    func logOnce(_ message: String, level: String = "error") {
        if logged.insert(level + message).inserted { log(message, level: level) }
    }

    func warnOnce(_ message: String) { logOnce(message, level: "warn") }

    // MARK: handlers

    /// Runs an `on.*` body with `event` bound, detached from the view update.
    @discardableResult
    func fire(_ stmts: [Stmt], ctx: RenderContext, event: Value) -> Task<Void, Never> {
        let locals = Locals(name: "event", value: event, parent: ctx.locals)
        let effects = StoreEffects(store: self)
        let id = UUID(), bag = tasks
        // Main-actor task: its body can't start before `add` below, so `remove` always follows it.
        let task = Task { [interpreter] in
            await interpreter.runStatements(stmts, scope: ctx, locals: locals, effects: effects)
            bag.remove(id)
        }
        bag.add(id, task)
        return task
    }

    func call(op: String, args: Value, awaited: Bool) async -> Value {
        guard !Task.isCancelled else { return API.callFailure(WishboneError.network("cancelled")) }
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
            store.logOnce("\(nodeID.map { "\($0): " } ?? "")\(error)")
            return nil
        }
    }
}
