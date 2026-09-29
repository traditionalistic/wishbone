import Foundation
import Observation
import SwiftUI
import WishboneCore

/// Shared `env.now` source: ms UTC, ticks on whole seconds while at least one screen is visible.
@MainActor @Observable
final class Clock {
    private(set) var now = Date().timeIntervalSince1970 * 1000
    @ObservationIgnored private var users = 0
    @ObservationIgnored private var ticker: Task<Void, Never>?

    func retain() {
        users += 1
        guard users == 1 else { return }
        now = Date().timeIntervalSince1970 * 1000
        ticker = Task { [weak self] in
            while !Task.isCancelled {
                let ms = Date().timeIntervalSince1970 * 1000
                try? await Task.sleep(for: .milliseconds(1000 - Int(ms.truncatingRemainder(dividingBy: 1000))))
                guard let self else { return }
                now = Date().timeIntervalSince1970 * 1000
            }
        }
    }

    func release() {
        users = max(0, users - 1)
        if users == 0 { ticker?.cancel(); ticker = nil }
    }
}

/// Screen size and safe insets of one navigation stack (a sheet has its own), written by its root.
@MainActor @Observable
final class ScreenGeometry {
    var screenSize = CGSize(width: 390, height: 844)
    var safeTop = 0.0
    var safeBottom = 0.0
}

/// App-wide system values behind `env.*` (scheme, reduce motion, locale).
@MainActor @Observable
final class EnvState {
    var dark = false
    var reduceMotion = false

    /// Language code only, lowercase ("ru-RU" → "ru"), as on Android; also the `Accept-Language` header.
    let locale = Locale(identifier: Locale.preferredLanguages.first ?? "en").language.languageCode?.identifier.lowercased() ?? "en"

    func value(_ name: String, clock: Clock, brand: String, geometry g: ScreenGeometry) -> Value {
        switch name {
        case "now": .number(clock.now)
        case "platform": .string("ios")
        case "locale": .string(locale)
        case "brand": .string(brand)
        case "colorScheme": .string(dark ? "dark" : "light")
        case "reduceMotion": .bool(reduceMotion)
        case "screenWidth": .number(g.screenSize.width)
        case "screenHeight": .number(g.screenSize.height)
        case "safeTop": .number(g.safeTop)
        case "safeBottom": .number(g.safeBottom)
        case "timeZone": .string(TimeZone.current.identifier)
        default: .null
        }
    }
}

/// Feeds `EnvState` from the SwiftUI environment and geometry of the screen root.
struct EnvReader: ViewModifier {
    struct Geometry: Equatable {
        var size: CGSize
        var insets: EdgeInsets
    }

    let env: EnvState
    let geometry: ScreenGeometry
    /// A slot (spec 2026-09-28 §2.3): no safe insets, width = the slot's, height 0 (it follows the content).
    var inline = false
    @Environment(\.colorScheme) private var scheme
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content
            .onChange(of: scheme, initial: true) { env.dark = scheme == .dark }
            .onChange(of: reduceMotion, initial: true) { env.reduceMotion = reduceMotion }
            .background {
                // Measured outside keyboard avoidance: safe insets are system bars + cutout only.
                Color.clear
                    .ignoresSafeArea(.keyboard)
                    .onGeometryChange(for: Geometry.self) { Geometry(size: $0.size, insets: $0.safeAreaInsets) } action: { g in
                        if inline { // safeTop/safeBottom stay at their default 0
                            geometry.screenSize = CGSize(width: g.size.width, height: 0)
                            return
                        }
                        geometry.safeTop = g.insets.top
                        geometry.safeBottom = g.insets.bottom
                        geometry.screenSize = CGSize(width: g.size.width, height: g.size.height + g.insets.top + g.insets.bottom)
                    }
            }
    }
}
