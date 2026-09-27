@_exported import WishboneCore
import Foundation
import Observation
import SwiftUI

/// What the host app provides to the SDK (spec §6.3). Every method has a default.
@MainActor
public protocol WishboneHost {
    func authHeaders() async -> [String: String]
    /// Only https, the server's own `/…` paths and `allowedSchemes` reach it.
    func open(url: URL)
    func track(event: String, params: [String: Any])
    func action(name: String, args: [String: Any]) async
    func nativeView(name: String, props: [String: Any]) -> AnyView?
    /// Extra URL schemes `open()` may pass besides https.
    var allowedSchemes: Set<String> { get }
}

extension WishboneHost {
    public func authHeaders() async -> [String: String] { [:] }
    public func open(url: URL) {}
    public func track(event: String, params: [String: Any]) {}
    public func action(name: String, args: [String: Any]) async {}
    public func nativeView(name: String, props: [String: Any]) -> AnyView? { nil }
    public var allowedSchemes: Set<String> { [] }
}

final class DefaultHost: WishboneHost {}

/// SDK entry point: server, brand, host, theme. One per app.
@MainActor @Observable
public final class Wishbone {
    public var baseURL: URL {
        didSet { if baseURL != oldValue { loadTheme() } }
    }
    public var brand: String {
        didSet { if brand != oldValue { loadTheme() } }
    }
    public private(set) var theme: Theme
    @ObservationIgnored public let host: any WishboneHost
    /// Compiled screen answers served without the network (playground fixtures), by screen id.
    @ObservationIgnored public var localScreens: [String: Data] = [:]
    /// Receives runtime errors of every screen (the preview client forwards them to the admin).
    @ObservationIgnored public var onLog: ((_ level: String, _ message: String) -> Void)?

    let env = EnvState()
    let clock = Clock()
    @ObservationIgnored let cache: DiskCache
    @ObservationIgnored private var themeTask: Task<Void, Never>?

    public convenience init(baseURL: URL, brand: String = "korpu", host: (any WishboneHost)? = nil) {
        self.init(baseURL: baseURL, brand: brand, host: host ?? DefaultHost(), cache: .standard)
        loadTheme()
    }

    init(baseURL: URL, brand: String, host: any WishboneHost, cache: DiskCache) {
        self.baseURL = baseURL
        self.brand = brand
        self.host = host
        self.cache = cache
        theme = Self.cachedTheme(cache, brand) ?? .bundledKorpu
        _ = Fonts.register
    }

    func requestInfo() async -> RequestInfo {
        RequestInfo(baseURL: baseURL, brand: brand, locale: env.locale, headers: await host.authHeaders())
    }

    func log(_ level: String, _ message: String) {
        #if DEBUG
        print("[wishbone] \(level): \(message)")
        #endif
        onLog?(level, message)
    }

    // MARK: theme (cached per brand, revalidated with ETag)

    private static func cachedTheme(_ cache: DiskCache, _ brand: String) -> Theme? {
        cache.read(DiskCache.themeKey(brand: brand)).flatMap { try? Theme(json: $0) }
    }

    private func loadTheme() {
        if let cached = Self.cachedTheme(cache, brand) { theme = cached }
        themeTask?.cancel()
        themeTask = Task { [weak self] in
            guard let self else { return }
            let key = DiskCache.themeKey(brand: brand)
            let etag = cache.read(key + "|etag").map { String(decoding: $0, as: UTF8.self) }
            let req = API.theme(await requestInfo(), etag: etag)
            do {
                let (body, http) = try await API.send(req)
                guard let body, !Task.isCancelled else { return }
                theme = try Theme(json: body)
                cache.write(key, body)
                if let tag = http.value(forHTTPHeaderField: "ETag") { cache.write(key + "|etag", Data(tag.utf8)) }
            } catch {
                log("warn", "theme \(brand): \(error)")
            }
        }
    }
}
