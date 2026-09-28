import CryptoKit
import Foundation
import WishboneCore

/// Last good screen/theme answers on disk (stale-while-revalidate source, spec §6.3).
struct DiskCache: Sendable {
    let dir: URL

    static let standard = DiskCache(
        dir: FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0].appendingPathComponent("wishbone", isDirectory: true)
    )

    func read(_ key: String) -> Data? {
        try? Data(contentsOf: url(key))
    }

    func write(_ key: String, _ data: Data) {
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        try? data.write(to: url(key), options: .atomic)
    }

    private func url(_ key: String) -> URL {
        let digest = SHA256.hash(data: Data(key.utf8)).map { String(format: "%02x", $0) }.joined()
        return dir.appendingPathComponent(digest + ".json")
    }

    /// One entry per cache scope (server × host scope × brand, `Wishbone.cacheScope`) × screen × params.
    static func screenKey(scope: String, id: String, params: [String: Value]) -> String {
        "screen|\(scope)|\(id)|\(API.jsonString(.record(params)))"
    }

    static func themeKey(scope: String) -> String { "theme|\(scope)" }
}
