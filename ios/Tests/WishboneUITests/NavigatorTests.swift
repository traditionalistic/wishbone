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
}
