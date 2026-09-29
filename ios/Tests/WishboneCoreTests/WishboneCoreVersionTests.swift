import Testing
@testable import WishboneCore

@Test func runtimeVersionIsSet() {
    #expect(WishboneCoreVersion.runtime == "1.1.0")
}
