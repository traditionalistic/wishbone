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
