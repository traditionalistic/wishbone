import SwiftUI
import WishboneCore

/// `textField`: controlled when the node has a `value` prop (shows the evaluated value; an edit
/// fires `change` and is reverted unless the screen stores it), uncontrolled otherwise.
struct TextFieldNode: View {
    let node: Node
    let props: Props
    let ctx: RenderContext
    @State private var text: String

    init(node: Node, props: Props, ctx: RenderContext) {
        self.node = node
        self.props = props
        self.ctx = ctx
        _text = State(initialValue: props.string("value") ?? "")
    }

    private var controlled: Bool { node.props["value"] != nil }

    var body: some View {
        let wb = ctx.store.wishbone
        let style = wb.theme.textStyle(props["style"])
        let color = wb.theme.color(props["color"], dark: wb.env.dark)?.color ?? .primary
        let value = props.string("value") ?? ""
        let prompt = Text(verbatim: props.string("placeholder") ?? "").foregroundStyle(color.opacity(0.4))
        field(prompt: prompt)
            .textFieldStyle(.plain)
            .font(style.map { Fonts.resolved($0).font } ?? .body)
            .foregroundStyle(color)
            .modifier(KeyboardModifier(kind: props.string("keyboard") ?? "text"))
            .onSubmit { fire("submit") }
            .onChange(of: value) { if controlled { text = value } }
            .onChange(of: text) {
                guard text != value || !controlled else { return }
                let run = fire("change")
                guard controlled else { return }
                Task {
                    await run?.value
                    let fresh = node.props["value"].flatMap { ctx.eval($0) }
                    if case let .string(s)? = fresh, s != text { text = s }
                }
            }
    }

    @ViewBuilder
    private func field(prompt: Text) -> some View {
        if props.bool("secure") {
            SecureField("", text: $text, prompt: prompt)
        } else {
            TextField("", text: $text, prompt: prompt)
        }
    }

    @discardableResult
    private func fire(_ event: String) -> Task<Void, Never>? {
        guard let stmts = node.on[event] else { return nil }
        return ctx.store.fire(stmts, ctx: ctx, event: .record(["value": .string(text)]))
    }
}

struct KeyboardModifier: ViewModifier {
    let kind: String

    func body(content: Content) -> some View {
        #if os(iOS)
        switch kind {
        case "email":
            content.keyboardType(.emailAddress).textContentType(.emailAddress)
                .textInputAutocapitalization(.never).autocorrectionDisabled()
        case "number":
            content.keyboardType(.decimalPad)
        case "phone":
            content.keyboardType(.phonePad).textContentType(.telephoneNumber)
        default:
            content
        }
        #else
        content
        #endif
    }
}
