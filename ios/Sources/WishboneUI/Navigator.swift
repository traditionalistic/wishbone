import Foundation
import Observation
import SwiftUI
import WishboneCore

/// One entry of the SDK-owned navigation stack.
struct Route: Hashable, Identifiable, Sendable {
    let id: UUID
    let screen: String
    let params: [String: Value]
    var fade = false
    /// Set for the live-preview root: documents come from the session, not from the network.
    var preview: PreviewClient?

    init(id: UUID = UUID(), screen: String, params: [String: Value], fade: Bool = false, preview: PreviewClient? = nil) {
        self.id = id
        self.screen = screen
        self.params = params
        self.fade = fade
        self.preview = preview
    }

    /// Rebuilds an internal `Route` from a host-issued `WishboneRoute` (`WishboneScreen(route:)`), keeping
    /// the same `id` so the host's stack record and the SDK's route agree.
    init(_ r: WishboneRoute) {
        self.init(id: r.id, screen: r.screen, params: r.params, fade: r.transition == "fade")
    }

    static func == (a: Route, b: Route) -> Bool { a.id == b.id }
    func hash(into h: inout Hasher) { h.combine(id) }
}

/// One entry of the HOST's navigation stack (spec §1.1): pushed via `Navigator.forward` when a screen
/// is running in host-stack mode. Each push is a separate stack record, identified by `id`.
public struct WishboneRoute: Hashable, Identifiable, Sendable {
    public let id: UUID
    public let screen: String
    public let params: [String: Value]
    /// "push" | "fade" | "present" (slots only) | nil — a hint for the host.
    public let transition: String?

    public init(screen: String, params: [String: Value] = [:], transition: String? = nil) {
        id = UUID()
        self.screen = screen
        self.params = params
        self.transition = transition
    }

    init(_ route: Route, transition: String? = nil) {
        id = route.id
        screen = route.screen
        params = route.params
        self.transition = transition ?? (route.fade ? "fade" : "push")
    }

    // `params` holds `Value`, which is Equatable but not Hashable (see WishboneCore/Value.swift);
    // identity is `id`, same as `Route`.
    public static func == (a: WishboneRoute, b: WishboneRoute) -> Bool { a.id == b.id }
    public func hash(into h: inout Hasher) { h.combine(id) }
}

/// Stack of one `WishboneScreen` (or of a presented sheet): `navigate` pushes, `present` shows a
/// sheet with its own navigator, `back` pops (or closes the sheet / leaves the host screen at the
/// root), `dismiss` closes the sheet this navigator lives in, else pops.
@MainActor @Observable
final class Navigator {
    var path: [Route] = []
    var sheet: Route?
    @ObservationIgnored weak var presenter: Navigator?
    /// Leaves the SDK at the root (host's `dismiss`).
    @ObservationIgnored var exit: (() -> Void)?
    /// Set when this screen runs in the host's stack (`WishboneScreen(onNavigate:)`): `navigate` forwards
    /// to the host instead of pushing the SDK's own stack. Never set on a sheet's navigator — a sheet's
    /// stack always stays inside the SDK (spec §1.2).
    @ObservationIgnored var forward: ((WishboneRoute) -> Void)?
    /// Slots only: `present` also forwards to the host (as `transition: "present"`) instead of opening
    /// an SDK sheet — a slot has no sheet of its own.
    @ObservationIgnored var forwardsPresent = false
    /// `env.screen*` / `env.safe*` of this stack.
    @ObservationIgnored let geometry = ScreenGeometry()

    init(presenter: Navigator? = nil) {
        self.presenter = presenter
    }

    func navigate(_ route: Route) {
        if let forward { return forward(WishboneRoute(route)) }
        guard route.fade else { return path.append(route) }
        var t = Transaction()
        t.disablesAnimations = true
        withTransaction(t) { path.append(route) }
    }

    func back() {
        if !path.isEmpty {
            path.removeLast()
        } else if let presenter {
            presenter.sheet = nil
        } else {
            exit?()
        }
    }

    func present(_ route: Route) {
        if forwardsPresent, let forward { return forward(WishboneRoute(route, transition: "present")) }
        sheet = route
    }

    func dismiss() {
        if let presenter { presenter.sheet = nil } else { back() }
    }
}

#if os(iOS)
import UIKit

/// The system bar is hidden (screens draw their own headers), which also disables UIKit's edge-swipe
/// back; this re-enables it for the SDK's own navigation controller only (never at its root).
struct SwipeBackEnabler: UIViewControllerRepresentable {
    /// The iOS 26 full-width swipe would fight horizontal `drag` nodes; screens with drags keep the edge swipe only.
    let allowsContentPop: Bool

    func makeUIViewController(context: Context) -> Controller { Controller() }
    func updateUIViewController(_ controller: Controller, context: Context) {
        controller.allowsContentPop = allowsContentPop
    }

    final class Controller: UIViewController, UIGestureRecognizerDelegate {
        var allowsContentPop = true

        override func viewDidAppear(_ animated: Bool) {
            super.viewDidAppear(animated)
            navigationController?.interactivePopGestureRecognizer?.delegate = self
            if #available(iOS 26, *) { navigationController?.interactiveContentPopGestureRecognizer?.delegate = self }
        }

        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard (navigationController?.viewControllers.count ?? 0) > 1 else { return false }
            if #available(iOS 26, *), gestureRecognizer === navigationController?.interactiveContentPopGestureRecognizer {
                return allowsContentPop
            }
            return true
        }
    }
}
#endif
