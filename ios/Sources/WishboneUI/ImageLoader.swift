import SwiftUI
#if canImport(UIKit)
import UIKit
typealias PlatformImage = UIImage
#else
import AppKit
typealias PlatformImage = NSImage
#endif

/// Loads `image` node pictures (AsyncImage never retries a load that failed or was cancelled, e.g. by a lazy container):
/// URLSession.shared (URLCache applies), decoding off the main actor, and a small shared memory cache
/// so a cell a lazy container re-creates shows its picture at once.
enum ImageLoader {
    /// NSCache is thread-safe; keyed by absolute URL string, bounded by decoded bytes as well as by count.
    private nonisolated(unsafe) static let memory: NSCache<NSString, PlatformImage> = {
        let cache = NSCache<NSString, PlatformImage>()
        cache.countLimit = 100
        cache.totalCostLimit = 64 << 20
        return cache
    }()

    static func cached(_ url: URL) -> PlatformImage? { memory.object(forKey: url.absoluteString as NSString) }

    static func cache(_ image: PlatformImage, for url: URL) {
        memory.setObject(image, forKey: url.absoluteString as NSString, cost: cost(image))
    }

    /// The decoded bitmap's size in bytes (1 when there is no bitmap to measure).
    static func cost(_ image: PlatformImage) -> Int {
        #if canImport(UIKit)
        let cg = image.cgImage
        #else
        let cg = image.cgImage(forProposedRect: nil, context: nil, hints: nil)
        #endif
        return cg.map { max($0.bytesPerRow * $0.height, 1) } ?? 1
    }

    /// nil when the bytes are not an image (e.g. an HTML error page).
    static func decode(_ data: Data) -> PlatformImage? {
        #if canImport(UIKit)
        guard let image = UIImage(data: data) else { return nil }
        return image.preparingForDisplay() ?? image // decode here, not at the first draw on the main thread
        #else
        return NSImage(data: data)
        #endif
    }

    /// Throws when cancelled or offline; nil when the response is not an image.
    @concurrent static func load(_ url: URL) async throws -> PlatformImage? {
        if let hit = cached(url) { return hit }
        let (data, _) = try await URLSession.shared.data(from: url)
        guard let image = decode(data) else { return nil }
        cache(image, for: url)
        return image
    }
}

extension Image {
    init(platformImage: PlatformImage) {
        #if canImport(UIKit)
        self.init(uiImage: platformImage)
        #else
        self.init(nsImage: platformImage)
        #endif
    }
}
