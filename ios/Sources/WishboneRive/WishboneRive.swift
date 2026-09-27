import RiveRuntime
import SwiftUI
import WishboneUI

/// `rive` primitive adapter (spec §4.4): `url`, `artboard`, `stateMachine`, `inputs` (bool/number
/// state-machine inputs, re-applied on change), Rive events → `event` `{name}`. The view doesn't
/// intercept taps (V1). Call `WishboneRive.register()` once at startup.
public enum WishboneRive {
    public static let runtime = WishboneUIVersion.runtime

    @MainActor
    public static func register() {
        WishboneRenderers.register("rive") { AnyView(RiveNode(ctx: $0)) }
    }
}

/// A view model that forwards Rive events and re-applies inputs once the file has loaded.
final class RiveNodeModel: RiveViewModel {
    var onEvent: ((String) -> Void)?
    var inputs: [String: Value] = [:]

    func apply(_ inputs: [String: Value]) {
        self.inputs = inputs
        for (name, value) in inputs {
            switch value {
            case let .bool(b): setInput(name, value: b)
            case let .number(n) where n.isFinite: setInput(name, value: n)
            default: break
            }
        }
    }

    override func riveFileDidLoad(_ riveFile: RiveFile) throws {
        try super.riveFileDidLoad(riveFile)
        apply(inputs)
    }

    @objc func onRiveEventReceived(onRiveEvent riveEvent: RiveEvent) {
        onEvent?(riveEvent.name())
    }
}

struct RiveNode: View {
    let ctx: WishboneNodeContext
    @State private var model: RiveNodeModel?

    private var inputs: [String: Value] {
        if case let .record(r)? = ctx.props["inputs"] { r } else { [:] }
    }

    var body: some View {
        let url = ctx.url()?.absoluteString ?? ""
        let artboard = ctx.string("artboard") ?? "", stateMachine = ctx.string("stateMachine") ?? ""
        ZStack {
            Color.clear // keeps the view (and its .task) alive before the model exists
            if let model { model.view() }
        }
        .allowsHitTesting(false)
        .task(id: [url, artboard, stateMachine]) {
            guard !url.isEmpty else { return }
            let m = RiveNodeModel(
                webURL: url, stateMachineName: stateMachine.isEmpty ? nil : stateMachine,
                artboardName: artboard.isEmpty ? nil : artboard
            )
            let fire = ctx.fire
            m.onEvent = { name in fire("event", .record(["name": .string(name)])) }
            m.inputs = inputs
            model = m
        }
        .onChange(of: inputs) { _, new in model?.apply(new) }
    }
}
