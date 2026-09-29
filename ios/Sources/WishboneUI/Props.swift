import WishboneCore

/// Evaluated props of one node, falling back to contract/primitives.json defaults.
struct Props {
    let type: String
    let values: [String: Value]

    subscript(_ key: String) -> Value {
        values[key] ?? Props.fallback[type, default: Props.common][key] ?? .null
    }

    private static let common = defaults["common", default: [:]]
    /// Per type: its defaults over the common ones (one lookup for a prop the node didn't set).
    private static let fallback = defaults.mapValues { common.merging($0) { _, own in own } }

    /// Finite numbers only: a server document must never crash layout (NaN/±Inf are dropped).
    func number(_ key: String) -> Double? {
        if case let .number(n) = self[key], n.isFinite { n } else { nil }
    }

    /// A size/length: finite and ≥ 0.
    func length(_ key: String) -> Double? { number(key).map { Swift.max(0, $0) } }

    /// An integer count clamped to `range`.
    func count(_ key: String, in range: ClosedRange<Int>) -> Int? {
        number(key).map { Int(Swift.min(Swift.max($0.rounded(.down), Double(range.lowerBound)), Double(range.upperBound))) }
    }

    func string(_ key: String) -> String? {
        if case let .string(s) = self[key] { s } else { nil }
    }

    func bool(_ key: String) -> Bool {
        if case let .bool(b) = self[key] { b } else { false }
    }

    private static func token(_ group: String, _ name: String) -> Value { .record(["$\(group)": .string(name)]) }

    /// Literal defaults from contract/primitives.json (a unit test compares them).
    static let defaults: [String: [String: Value]] = [
        "common": [
            "width": .string("wrap"), "height": .string("wrap"), "padding": .number(0), "corner": .number(0),
            "clip": .bool(false), "opacity": .number(1), "offsetX": .number(0), "offsetY": .number(0),
            "scale": .number(1), "rotation": .number(0), "zIndex": .number(0), "ignoreSafeArea": .string("none"),
        ],
        "stack": ["axis": .string("v"), "spacing": .number(0)],
        "scroll": [
            "axis": .string("v"), "spacing": .number(0), "align": .string("start"), "paging": .bool(false),
            "indicators": .bool(false), "lazy": .bool(false),
        ],
        "grid": ["spacing": .number(0), "rowSpacing": .number(0)],
        "spacer": ["min": .number(0)],
        "text": [
            "style": token("type", "body"), "color": token("color", "el16"), "textAlign": .string("start"),
            "maxLines": .number(0),
        ],
        "image": ["fit": .string("fill"), "placeholder": token("color", "bg2")],
        "icon": ["size": .number(24), "color": token("color", "el16")],
        "shape": ["kind": .string("rect")],
        "textField": [
            "value": .string(""), "placeholder": .string(""), "keyboard": .string("text"), "secure": .bool(false),
            "style": token("type", "body"), "color": token("color", "el16"),
        ],
        "lottie": ["loop": .bool(true), "playing": .bool(true), "speed": .number(1)],
        "rive": ["artboard": .string(""), "stateMachine": .string("")],
    ]
}
