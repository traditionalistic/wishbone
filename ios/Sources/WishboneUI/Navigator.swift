import Foundation
import Observation
import SwiftUI
import WishboneCore

/// One entry of the SDK-owned navigation stack.
struct Route: Hashable, Identifiable, Sendable {
    let id = UUID()
    let screen: String
    let params: [String: Value]
    var fade = false
    /// Set for the live-preview root: documents come from the session, not from the network.
    var preview: PreviewClient?

    static func == (a: Route, b: Route) -> Bool { a.id == b.id }
    func hash(into h: inout Hasher) { h.combine(id) }
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
    /// `env.screen*` / `env.safe*` of this stack.
    @ObservationIgnored let geometry = ScreenGeometry()

    init(presenter: Navigator? = nil) {
        self.presenter = presenter
    }

    func navigate(_ route: Route) {
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
