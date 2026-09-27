import Lottie
import Testing
@testable import WishboneLottie
import WishboneRive
@testable import WishboneUI

@MainActor
@Suite struct AdapterTests {
    @Test func adaptersRegisterTheirPrimitives() {
        WishboneLottie.register()
        WishboneRive.register()
        #expect(WishboneRenderers.registry["lottie"] != nil && WishboneRenderers.registry["rive"] != nil)
    }

    @Test func lottiePlaybackModeFromProps() {
        #expect(WishboneLottie.mode(playing: true, loop: true, progress: nil) == .playing(.fromProgress(nil, toProgress: 1, loopMode: .loop)))
        #expect(WishboneLottie.mode(playing: true, loop: false, progress: nil) == .playing(.fromProgress(nil, toProgress: 1, loopMode: .playOnce)))
        #expect(WishboneLottie.mode(playing: false, loop: true, progress: nil) == .paused(at: .currentFrame))
        // progress drives the frame (clamped), whatever `playing` says
        #expect(WishboneLottie.mode(playing: true, loop: true, progress: 0.4) == .paused(at: .progress(0.4)))
        #expect(WishboneLottie.mode(playing: true, loop: true, progress: 7) == .paused(at: .progress(1)))
    }
}
