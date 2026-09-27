import SwiftUI
import WishboneUI

@main
struct PlaygroundApp: App {
    @State private var model = PlaygroundModel()

    var body: some Scene {
        WindowGroup {
            HomeView(model: model)
                .overlay(alignment: .top) { BannerView(host: model.host) }
                .preferredColorScheme(model.scheme.colorScheme)
        }
    }
}

enum SchemeChoice: String, CaseIterable, Identifiable {
    case system, light, dark
    var id: String { rawValue }
    var colorScheme: ColorScheme? {
        switch self {
        case .system: nil
        case .light: .light
        case .dark: .dark
        }
    }
}

/// What the home screen opens full-screen.
enum Opened: Identifiable {
    case screen(String)
    case preview(String)

    var id: String {
        switch self {
        case let .screen(id): "screen:\(id)"
        case let .preview(code): "preview:\(code)"
        }
    }
}

@MainActor @Observable
final class PlaygroundModel {
    static let fixtures: [String: Data] = {
        let urls = Bundle.main.urls(forResourcesWithExtension: "json", subdirectory: "Fixtures") ?? []
        return Dictionary(uniqueKeysWithValues: urls.compactMap { url in
            (try? Data(contentsOf: url)).map { (url.deletingPathExtension().lastPathComponent, $0) }
        })
    }()

    let host = PlaygroundHost()
    let wishbone: Wishbone
    var serverURL: String { didSet { UserDefaults.standard.set(serverURL, forKey: "serverURL") } }
    var brand: String { didSet { wishbone.brand = brand; UserDefaults.standard.set(brand, forKey: "brand") } }
    var scheme: SchemeChoice { didSet { UserDefaults.standard.set(scheme.rawValue, forKey: "scheme") } }
    var offline: Bool { didSet { applyOffline(); UserDefaults.standard.set(offline, forKey: "offline") } }
    var sessions: [PreviewSession] = []
    var sessionsError: String?
    var opened: Opened?

    init() {
        let d = UserDefaults.standard
        let server = d.string(forKey: "serverURL") ?? "http://localhost:8080"
        let brand = d.string(forKey: "brand") ?? "korpu"
        serverURL = server
        self.brand = brand
        scheme = SchemeChoice(rawValue: d.string(forKey: "scheme") ?? "") ?? .system
        offline = d.bool(forKey: "offline")
        wishbone = Wishbone(baseURL: URL(string: server) ?? URL(string: "http://localhost:8080")!, brand: brand, host: host)
        applyOffline()
        // Launch shortcuts for automation: WB_OPEN=<screen id>, WB_PREVIEW=<session code>.
        let env = ProcessInfo.processInfo.environment
        if let id = env["WB_OPEN"] { opened = .screen(id) } else if let code = env["WB_PREVIEW"] { opened = .preview(code) }
    }

    func applyServer() {
        if let url = URL(string: serverURL) { wishbone.baseURL = url }
        Task { await loadSessions() }
    }

    func loadSessions() async {
        do {
            sessions = try await PreviewClient.sessions(wishbone)
            sessionsError = nil
        } catch {
            sessions = []
            sessionsError = String(describing: error)
        }
    }

    private func applyOffline() {
        // fixture.* never exist on the server; offline mode also serves the bundled showcase.* copies.
        wishbone.localScreens = offline ? Self.fixtures : Self.fixtures.filter { $0.key.hasPrefix("fixture.") }
    }
}

/// Logs track/host/open to the console; host actions also show a banner.
@MainActor @Observable
final class PlaygroundHost: WishboneHost {
    var banner: String?

    func open(url: URL) {
        print("[host] open \(url)")
        banner = "open \(url.absoluteString)"
    }

    func track(event: String, params: [String: Any]) {
        print("[host] track \(event) \(params)")
    }

    func action(name: String, args: [String: Any]) async {
        print("[host] action \(name) \(args)")
        banner = "\(name): \(args["message"] as? String ?? "\(args)")"
    }

    func nativeView(name: String, props: [String: Any]) -> AnyView? {
        guard name == "badge" else { return nil }
        return AnyView(
            Label(props["text"] as? String ?? name, systemImage: "swift")
                .font(.footnote.bold())
                .padding(.horizontal, 10).padding(.vertical, 6)
                .background(.orange.opacity(0.2), in: Capsule())
        )
    }
}

struct BannerView: View {
    let host: PlaygroundHost

    var body: some View {
        if let text = host.banner {
            Text(text)
                .font(.footnote.weight(.semibold))
                .padding(.horizontal, 14).padding(.vertical, 10)
                .background(.thinMaterial, in: Capsule())
                .padding(.top, 8)
                .transition(.move(edge: .top).combined(with: .opacity))
                .task(id: text) {
                    try? await Task.sleep(for: .seconds(2))
                    withAnimation { host.banner = nil }
                }
        }
    }
}
