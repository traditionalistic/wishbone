#if canImport(UIKit)
import SwiftUI
import UIKit

/// The capture layer (spec 2026-09-30 §2.3): a `UIHostingController` with the job's screen, hosted under the live preview
/// in the same window, so it has the same size, safe area and scheme. It waits for the screen to settle (§2.2), draws its
/// view hierarchy at 2× into a JPEG and answers once through `finish`. Torn down with the job: its store's handler runs
/// (pending `call`s included) are cancelled and the store goes with the controller.
struct CaptureLayer: UIViewControllerRepresentable {
    let wishbone: Wishbone
    let job: RenderJob
    let finish: (Shot) -> Void

    func makeUIViewController(context: Context) -> CaptureController {
        CaptureController(wishbone: wishbone, job: job, finish: finish)
    }

    func updateUIViewController(_ controller: CaptureController, context: Context) {}

    static func dismantleUIViewController(_ controller: CaptureController, coordinator: ()) {
        controller.tearDown()
    }
}

final class CaptureController: UIHostingController<CaptureContent> {
    private let store: Store
    private var loop: Task<Void, Never>?

    init(wishbone: Wishbone, job: RenderJob, finish: @escaping (Shot) -> Void) {
        // Params are the document's defaults; no navigator — a shot has nothing to move.
        store = Store(document: job.document, params: [:], data: job.data, wishbone: wishbone, capture: true)
        super.init(rootView: CaptureContent(store: store))
        view.isUserInteractionEnabled = false // never touchable, never read by VoiceOver: the live screen above is the UI
        view.accessibilityElementsHidden = true
        loop = Task { [weak self] in
            let start = ContinuousClock.now
            var settle = Settle()
            try? await Task.sleep(for: .seconds(Settle.floor))
            while !Task.isCancelled, let self {
                let elapsed = start.duration(to: .now) / .seconds(1)
                switch settle.poll(elapsed: elapsed, imagesInFlight: ImageLoader.inFlight, finiteTimelineRunning: store.timelines.finiteRunning) {
                case .wait: try? await Task.sleep(for: .seconds(Settle.poll))
                case let .capture(settled): return finish(capture(settled: settled))
                }
            }
        }
    }

    @available(*, unavailable) required init?(coder: NSCoder) { nil }

    func tearDown() {
        loop?.cancel()
        store.tasks.cancelAll()
    }

    /// The layer's pixels as a JPEG, 2 px per point, size in points; an app in the background can't draw.
    private func capture(settled: Bool) -> Shot {
        guard let scene = view.window?.windowScene else { return .error("capture layer is not on screen") }
        guard scene.activationState == .foregroundActive || scene.activationState == .foregroundInactive else {
            return .error("app is in the background")
        }
        let bounds = view.bounds
        let format = UIGraphicsImageRendererFormat()
        format.scale = 2
        var drawn = false
        let image = UIGraphicsImageRenderer(bounds: bounds, format: format).image { _ in
            drawn = view.drawHierarchy(in: bounds, afterScreenUpdates: true)
        }
        guard drawn, let jpeg = image.jpegData(compressionQuality: 0.7) else { return .error("could not draw the capture layer") }
        return .image(jpeg: jpeg, size: bounds.size, settled: settled)
    }
}

/// The job's screen, laid out like a live one: `ScreenContent` with `EnvReader` on the store's own geometry.
struct CaptureContent: View {
    let store: Store

    var body: some View {
        ScreenContent(store: store)
            .modifier(EnvReader(env: store.wishbone.env, geometry: store.geometry))
    }
}
#endif
