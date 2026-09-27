import Foundation
@testable import WishboneUI

/// `ios/Tests/WishboneUITests/Support.swift` → repo root.
let repoRoot = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()

func contractFile(_ path: String) throws -> Data {
    try Data(contentsOf: repoRoot.appendingPathComponent("contract/\(path)"))
}

func tempDir() -> URL {
    FileManager.default.temporaryDirectory.appendingPathComponent("wb-tests-\(UUID().uuidString)", isDirectory: true)
}

/// A Wishbone with a throwaway cache and no theme fetch (bundled korpu).
@MainActor
func testWishbone() -> Wishbone {
    Wishbone(baseURL: URL(string: "http://localhost:1")!, brand: "korpu", host: DefaultHost(), cache: DiskCache(dir: tempDir()))
}

func color(_ name: String, alpha: Double? = nil) -> Value {
    var r: [String: Value] = ["$color": .string(name)]
    if let alpha { r["alpha"] = .number(alpha) }
    return .record(r)
}
