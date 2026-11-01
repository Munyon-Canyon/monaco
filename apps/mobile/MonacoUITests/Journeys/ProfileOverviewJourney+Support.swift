import XCTest

extension ProfileOverviewJourney {
    static let signOutEverywhereTitle = "Sign out of all devices?"
    static let signOutEverywhereMessage =
        "This ends every session on your account, including a lost phone. "
        + "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."

    static func row(_ app: XCUIApplication, _ cabal: Cabal) -> XCUIElement {
        app.element("profile-cabal-row-\(cabal.id)")
    }

    static func reachCabal(_ app: XCUIApplication, _ target: XCUIElement) {
        let screen = app.windows.firstMatch.frame
        var attempts = 0
        while attempts < 40 {
            let inside = target.exists && screen.contains(target.frame.insetBy(dx: 4, dy: 4))
            if inside && app.isAboveTabBar(target) { break }
            app.swipeUp()
            attempts += 1
        }
    }

    static func tapCabalsTab(_ app: XCUIApplication) {
        let tab = app.element("profile-tab-cabals")
        var swipes = 0
        while !(tab.exists && tab.isHittable && app.isAboveTabBar(tab)) && swipes < 8 {
            app.swipeUp()
            swipes += 1
        }
        tab.tap()
    }

    static func positionsFigure(_ app: XCUIApplication) -> String {
        let texts = app.element("profile-positions").staticTexts.allElementsBoundByIndex
        return texts.map(\.label).first { $0.hasPrefix("$") } ?? ""
    }

    static func cabalsCount(_ app: XCUIApplication) -> Int? {
        let label = app.element("profile-stat-cabals").label
        return label.split(whereSeparator: { !$0.isNumber }).first.flatMap { Int($0) }
    }
}
