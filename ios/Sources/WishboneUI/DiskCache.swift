import CryptoKit
import Foundation
import WishboneCore

/// Last good screen/theme answers on disk (stale-while-revalidate source, spec §6.3).
struct DiskCache: Sendable {
    let dir: URL
    /// Ruling R2: screen keys include params, so entries would grow without bound — after a write only the newest
    /// `maxEntries` by modification time stay (nil: no cap).
    var maxEntries: Int? = 64

    static let standard = DiskCache(
        dir: FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0].appendingPathComponent("wishbone", isDirectory: true)
    )

    /// Theme answers and their etags: a directory of their own, never capped — an evicted theme (a 304 never rewrites
    /// it) would show the bundled one at the next cold start, then swap.
    var themes: DiskCache { DiskCache(dir: dir.appendingPathComponent("theme", isDirectory: true), maxEntries: nil) }

    func read(_ key: String) -> Data? {
        try? Data(contentsOf: url(key))
    }

    func write(_ key: String, _ data: Data) {
        let file = url(key)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        guard (try? data.write(to: file, options: .atomic)) != nil, let maxEntries else { return }
        prune(to: maxEntries, keeping: file)
    }

    /// Deletes the entry; a missing one is fine.
    func remove(_ key: String) {
        try? FileManager.default.removeItem(at: url(key))
    }

    /// A theme entry; one an older version kept among the screens moves to `themes` on first read.
    func readTheme(_ key: String) -> Data? {
        let themes = self.themes
        if let data = themes.read(key) { return data }
        guard let old = read(key) else { return nil }
        themes.write(key, old)
        remove(key)
        return old
    }

    /// Keeps the newest `max` entries, the one just written always among them (its time may tie with an older one's).
    private func prune(to max: Int, keeping written: URL) {
        let fm = FileManager.default
        guard let files = try? fm.contentsOfDirectory(at: dir, includingPropertiesForKeys: [.contentModificationDateKey], options: .skipsHiddenFiles)
        else { return }
        let others = files.filter { $0.pathExtension == "json" && $0.lastPathComponent != written.lastPathComponent }
        guard others.count >= max else { return }
        func modified(_ url: URL) -> Date { (try? url.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast }
        for (url, _) in others.map({ ($0, modified($0)) }).sorted(by: { $0.1 > $1.1 }).dropFirst(max - 1) {
            try? fm.removeItem(at: url)
        }
    }

    private func url(_ key: String) -> URL {
        var name: [UInt8] = []
        name.reserveCapacity(64)
        for byte in SHA256.hash(data: Data(key.utf8)) {
            name.append(Self.hexDigits[Int(byte >> 4)])
            name.append(Self.hexDigits[Int(byte & 0x0F)])
        }
        return dir.appendingPathComponent(String(decoding: name, as: UTF8.self) + ".json")
    }

    private static let hexDigits = Array("0123456789abcdef".utf8)

    /// One entry per cache scope (server × host scope × brand, `Wishbone.cacheScope`) × screen × params.
    static func screenKey(scope: String, id: String, params: [String: Value]) -> String {
        "screen|\(scope)|\(id)|\(API.jsonString(.record(params)))"
    }

    static func themeKey(scope: String) -> String { "theme|\(scope)" }
}
