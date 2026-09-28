import Testing
@testable import WishboneUI

@MainActor
@Suite struct NavigatorTests {
    @Test func pushAndBack() {
        let nav = Navigator()
        nav.navigate(Route(screen: "a", params: [:]))
        nav.navigate(Route(screen: "b", params: [:], fade: true))
        #expect(nav.path.map(\.screen) == ["a", "b"])
        #expect(nav.path.last?.fade == true)
        nav.back()
        #expect(nav.path.map(\.screen) == ["a"])
    }

    @Test func backAtTheRootLeavesTheSDK() {
        let nav = Navigator()
        var exited = false
        nav.exit = { exited = true }
        nav.back()
        #expect(exited)
    }

    @Test func presentAndDismissFromTheSheet() {
        let root = Navigator()
        root.navigate(Route(screen: "a", params: [:]))
        root.present(Route(screen: "sheet", params: [:]))
        #expect(root.sheet?.screen == "sheet")
        let sheet = Navigator(presenter: root)
        sheet.navigate(Route(screen: "inner", params: [:]))
        sheet.dismiss() // closes the whole sheet, even with a pushed screen inside
        #expect(root.sheet == nil)
        #expect(root.path.map(\.screen) == ["a"])
    }

    @Test func backInsideASheet() {
        let root = Navigator()
        root.present(Route(screen: "sheet", params: [:]))
        let sheet = Navigator(presenter: root)
        sheet.navigate(Route(screen: "inner", params: [:]))
        sheet.back()
        #expect(sheet.path.isEmpty && root.sheet != nil) // pops inside the sheet first
        sheet.back()
        #expect(root.sheet == nil)                        // then closes it
    }

    @Test func dismissWithoutASheetPops() {
        let nav = Navigator()
        nav.navigate(Route(screen: "a", params: [:]))
        nav.dismiss()
        #expect(nav.path.isEmpty)
    }

    @Test func hostStackReceivesPushes() {
        let nav = Navigator()
        var pushed: [WishboneRoute] = []
        var exited = false
        nav.forward = { pushed.append($0) }
        nav.exit = { exited = true }
        nav.navigate(Route(screen: "raffle.tickets", params: ["n": .number(1)], fade: true))
        #expect(nav.path.isEmpty)
        #expect(pushed.map(\.screen) == ["raffle.tickets"])
        #expect(pushed.first?.transition == "fade")
        #expect(pushed.first?.params["n"] == .number(1))
        nav.back()
        #expect(exited) // the host pops
    }

    @Test func hostStackKeepsSheetsInTheSDK() {
        let root = Navigator()
        var pushed = 0
        root.forward = { _ in pushed += 1 }
        root.present(Route(screen: "raffle.rules", params: [:]))
        #expect(root.sheet?.screen == "raffle.rules")
        let sheet = Navigator(presenter: root)
        sheet.navigate(Route(screen: "inner", params: [:])) // a sheet's navigator has no forward: pushes inside
        #expect(sheet.path.map(\.screen) == ["inner"] && pushed == 0)
        sheet.dismiss()
        #expect(root.sheet == nil)
    }

    @Test func slotForwardsPresent() {
        let nav = Navigator()
        var pushed: [WishboneRoute] = []
        nav.forward = { pushed.append($0) }
        nav.forwardsPresent = true
        nav.present(Route(screen: "raffle.rules", params: [:]))
        #expect(nav.sheet == nil && pushed.first?.transition == "present")
    }
}
