import Lottie
import SwiftUI
import WishboneUI

/// `lottie` primitive adapter (spec §4.4): `url`, `playing`, `loop`, `speed`, `progress` (0…1, drives the
/// frame when set), `end` event. Call `WishboneLottie.register()` once at startup.
public enum WishboneLottie {
    public static let runtime = WishboneUIVersion.runtime

    @MainActor
    public static func register() {
        WishboneRenderers.register("lottie") { AnyView(LottieNode(ctx: $0)) }
    }

    /// Props → playback mode (pure, unit-tested).
    static func mode(playing: Bool, loop: Bool, progress: Double?) -> LottiePlaybackMode {
        if let progress { return .paused(at: .progress(min(max(progress, 0), 1))) }
        guard playing else { return .paused(at: .currentFrame) }
        return .playing(.fromProgress(nil, toProgress: 1, loopMode: loop ? .loop : .playOnce))
    }
}

struct LottieNode: View {
    let ctx: WishboneNodeContext

    var body: some View {
        let url = ctx.url()
        LottieView {
            guard let url else { return nil }
            return await LottieAnimation.loadedFrom(url: url)
        }
        .resizable()
        .playbackMode(WishboneLottie.mode(playing: ctx.bool("playing") ?? true, loop: ctx.bool("loop") ?? true, progress: ctx.number("progress")))
        .animationSpeed(ctx.number("speed") ?? 1)
        .animationDidFinish { completed in
            if completed { ctx.fire("end") }
        }
        .id(url)
    }
}
