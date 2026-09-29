import CoreGraphics
import Foundation
import Testing
@testable import WishboneUI
#if canImport(UIKit)
import UIKit
#else
import AppKit
#endif

/// `image` loading: the shared memory cache and the decode step (the network part is plain URLSession).
@Suite struct ImageLoaderTests {
    /// A 1×1 PNG (base64 literal: `swift test` runs on macOS too, where UIGraphicsImageRenderer is missing).
    private let png = Data(base64Encoded: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")!

    @Test func decodesImagesAndRejectsGarbage() {
        #expect(ImageLoader.decode(png) != nil)
        #expect(ImageLoader.decode(Data("<html>404</html>".utf8)) == nil)
        #expect(ImageLoader.decode(Data()) == nil)
    }

    @Test func cacheIsKeyedByAbsoluteURL() throws {
        let image = try #require(ImageLoader.decode(png))
        let url = URL(string: "https://example.com/\(UUID().uuidString).png")!
        ImageLoader.cache(image, for: url)
        #expect(ImageLoader.cached(url) === image)
        #expect(ImageLoader.cached(URL(string: "https://example.com/other-\(UUID().uuidString).png")!) == nil)
    }

    /// Cells showing the same picture share one download.
    @Test func concurrentLoadsOfOneURLFetchOnce() async throws {
        let url = URL(string: "https://example.com/\(UUID().uuidString).png")!
        let fetches = Changes(), png = png
        let fetch: @Sendable (URL) async throws -> Data = { _ in
            fetches.bump()
            try await Task.sleep(for: .milliseconds(100))
            return png
        }
        let images = try await withThrowingTaskGroup(of: PlatformImage?.self) { group in
            for _ in 0..<5 { group.addTask { try await ImageLoader.load(url, fetch: fetch) } }
            return try await group.reduce(into: []) { $0.append($1) }
        }
        #expect(fetches.value == 1 && images.count == 5 && images.allSatisfy { $0 != nil && $0 === images[0] })
        #expect(ImageLoader.cached(url) === images[0])
    }

    /// The memory cache is bounded by bytes: an entry costs its decoded bitmap.
    @Test func costIsTheDecodedBitmapSize() throws {
        let ctx = try #require(CGContext(data: nil, width: 100, height: 50, bitsPerComponent: 8, bytesPerRow: 0,
                                         space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
        let cg = try #require(ctx.makeImage())
        #if canImport(UIKit)
        let image = UIImage(cgImage: cg)
        #else
        let image = NSImage(cgImage: cg, size: CGSize(width: 100, height: 50))
        #endif
        #expect(ImageLoader.cost(image) >= 100 * 50 * 4)
    }
}
