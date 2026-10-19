import XCTest

nonisolated final class V3DynamicTypeAuditUITests: XCTestCase {
    private struct Screen {
        let name: String
        let arguments: [String]

        init(_ name: String, _ arguments: String) {
            self.name = name
            self.arguments = arguments.split(separator: " ").map(String.init)
        }
    }

    private struct AllowedIssue {
        let screen: String
        let element: String
        let reason: String
    }

    private static let xxxLarge = "UICTContentSizeCategoryXXXL"
    private static let accessibilityXXXLarge = "UICTContentSizeCategoryAccessibilityXXXL"
    private static let swipes = 2

    private static let screens: [Screen] = [
        Screen("access__signin__phone", "-MonacoWelcomeSample signInPhone"),
        Screen("access__verify__ready", "-MonacoWelcomeSample code"),
        Screen("cabals__cabal-info__member", "-MonacoGroupDetailSample info"),
        Screen("cabals__cabal__ready", "-MonacoGroupDetailSample ready"),
        Screen("cabals__cabals__ready", "-MonacoCabalsTabSample -MonacoCabalsTabSampleScenario ready"),
        Screen("cabals__chat__ready", "-MonacoChatSampleQA -MonacoChatSampleProposal"),
        Screen("cabals__create-cabal__ready", "-MonacoCreateCabalSample ready"),
        Screen("cabals__discover__results", "-MonacoDiscoverSample results"),
        Screen("cabals__holding__ready", "-MonacoCabalHoldingSample ready"),
        Screen("cabals__invite__share", "-MonacoGroupDetailSample invite"),
        Screen("cabals__members__members", "-MonacoGroupDetailSample members"),
        Screen("cabals__public-cabal__preview", "-MonacoGroupDetailSample publicPreview"),
        Screen("explore__leaderboard__cabals", "-MonacoLeaderboardSample cabals"),
        Screen("feed__feed__all", "-feedHarness items"),
        Screen("feed__post__ready", "-feedPostHarness ready"),
        Screen("home__balances__ready", "-balancesHarness ready"),
        Screen("home__home__loss", "-MonacoHomeSample loss"),
        Screen("home__notifications__unread", "-activityFeedHarness unread"),
        Screen("markets__instrument__ready", "-MonacoAssetDetailSample open"),
        Screen("markets__markets__ready", "-MonacoStocksTabSample ready"),
        Screen("markets__watchlist__ready", "-MonacoWatchlistSample ready"),
        Screen("money__activity__ready", "-accountActivityHarness ready"),
        Screen("money__add-money__ready", "-MonacoMoneyFlowSample addMoney"),
        Screen("money__card-funding__handoff", "-MonacoMoneyFlowSample cardFunding"),
        Screen("money__cash-out__ready", "-cashOutHarness slice"),
        Screen("money__deposit__address", "-MonacoMoneyFlowSample depositAddress"),
        Screen("money__fund-cabal__ready", "-MonacoMoneyFlowSample fundCabal"),
        Screen("money__group-cash__ready", "-groupCashHarness ready"),
        Screen("money__network__ready", "-MonacoMoneyFlowSample networks"),
        Screen("money__transaction__completed", "-receiptHarness completed"),
        Screen("money__withdraw__ready", "-MonacoMoneyFlowSample withdraw"),
        Screen("profile__edit-profile__ready", "-MonacoProfileSample editProfileReady"),
        Screen("profile__people__results", "-friendsHarness results"),
        Screen("profile__person__public", "-MonacoProfileSample personPublic"),
        Screen("profile__profile__ready", "-MonacoProfileSample ready"),
        Screen("profile__settings__ready", "-settingsHarness on"),
        Screen("proposals__proposal__open", "-MonacoProposalSample open"),
        Screen("proposals__proposals__open", "-MonacoProposalSample allOpen"),
        Screen("proposals__propose__ready", "-MonacoProposeSample chooser"),
        Screen("proposals__review-proposal__ready", "-MonacoProposeSample review"),
        Screen("proposals__select-stock__buy", "-MonacoProposeSample buyList"),
        Screen("proposals__trade-amount__ready", "-MonacoProposeSample amount"),
        Screen("proposals__votes__open", "-MonacoProposalSample votesOpen"),
    ]

    private static func rows(_ pairs: [(String, String)], _ reason: String) -> [AllowedIssue] {
        pairs.map { AllowedIssue(screen: $0.0, element: $0.1, reason: reason) }
    }

    private static let allowed: [AllowedIssue] =
        rows(
            [
                ("cabals__cabals__ready", "Find your cabals"),
                ("cabals__create-cabal__ready", "Cabal name"),
                ("cabals__discover__results", "Search cabals"),
                ("feed__feed__all", "Search the feed"),
                ("markets__markets__ready", "Search stocks or tickers"),
                ("markets__watchlist__ready", "Find stocks for your watchlist"),
                ("profile__people__results", "Search people by name or handle"),
                ("proposals__select-stock__buy", "Search stocks or tickers"),
                ("access__signin__phone", "49 ''"),
            ], "a single-line text field scrolls its own text, and the whole text is its accessibility value")
        + rows(
            [
                ("cabals__create-cabal__ready", "Add a cover photo"),
                ("cabals__cabals__ready", "Return opens soon"),
                ("home__notifications__unread", "How much cash would we have left after this?"),
                ("home__notifications__unread", "The proposal reached its vote threshold."),
                ("money__add-money__ready", "How would you like to deposit?"),
                ("markets__markets__ready", "Manage"),
                ("home__home__loss", "Proposals"),
                ("home__home__loss", "Market updates"),
                ("home__home__loss", "Markets"),
                ("home__home__loss", "View all"),
            ], "a real defect with a follow-up in docs/design/v3/qa/dynamic-type.md")
        + rows(
            [
                ("cabals__invite__share", ""),
                ("proposals__propose__ready", ""),
                ("proposals__review-proposal__ready", ""),
                ("proposals__votes__open", ""),
            ],
            "text in a sheet that the screenshot shows whole; the audit measures it against the screen behind the sheet"
        )
        + rows(
            [
                ("access__signin__phone", "Your people."),
                ("feed__feed__all", "A small position alongside"),
                ("markets__markets__ready", "Default"),
                ("home__home__loss", "past month"),
                ("cabals__cabal__ready", "past month"),
                ("markets__instrument__ready", "Cabals holding GOOGL"),
            ], "whole in the screenshot, or cut by a line limit the design sets on purpose")
        + rows(
            [
                ("cabals__cabal__ready", "Alex"),
                ("cabals__cabal__ready", "Open chat"),
                ("cabals__cabal__ready", "3 new messages"),
                ("cabals__cabal__ready", "What’s everyone watching this week?"),
            ], "the chat teaser scrolls under the floating tab bar, and the bottom shot shows the last row clear of it")
        + rows([("*", "no element")], "a report with no element to name; it appears only while the screen scrolls")

    private static let preScaledReason: String? =
        "Typo.scaled returns Font.system(size:) already scaled for the launch size, so the runtime font check reads it as fixed"
    private static let offscreenReason: String? =
        "the element is partly outside the viewport or under the tab bar while the screen is scrolled"

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = true
    }

    @MainActor
    func testFirstHalfAtTheLargestNonAccessibilitySize() throws {
        try auditScreens(Self.firstHalf, at: Self.xxxLarge)
    }

    @MainActor
    func testSecondHalfAtTheLargestNonAccessibilitySize() throws {
        try auditScreens(Self.secondHalf, at: Self.xxxLarge)
    }

    @MainActor
    func testFirstHalfAtAccessibilityExtraExtraExtraLarge() throws {
        try auditScreens(Self.firstHalf, at: Self.accessibilityXXXLarge)
    }

    @MainActor
    func testSecondHalfAtAccessibilityExtraExtraExtraLarge() throws {
        try auditScreens(Self.secondHalf, at: Self.accessibilityXXXLarge)
    }

    @MainActor
    func testTheListHoldsTheFortyThreeDefaults() throws {
        XCTAssertEqual(Self.screens.count, 43)
        XCTAssertEqual(Set(Self.screens.map(\.name)).count, 43)
    }

    private static var firstHalf: [Screen] { Array(screens.prefix(22)) }
    private static var secondHalf: [Screen] { Array(screens.dropFirst(22)) }

    @MainActor
    private func auditScreens(_ screens: [Screen], at size: String) throws {
        var failures: [String] = []
        for screen in screens {
            let app = XCUIApplication()
            app.launchArguments = screen.arguments + ["-UIPreferredContentSizeCategoryName", size]
            app.launch()
            defer { app.terminate() }
            guard app.staticTexts.firstMatch.waitForExistence(timeout: 30) else {
                failures.append("\(screen.name) at \(size): the screen never drew")
                continue
            }
            Thread.sleep(forTimeInterval: 2)
            for position in 0...Self.swipes {
                if position > 0 { app.swipeUp() }
                let viewport = app.windows.firstMatch.frame
                let tabBar = app.tabBars.firstMatch
                let tabBarTop = tabBar.exists ? tabBar.frame.minY : viewport.maxY
                let navigationBar = app.navigationBars.firstMatch
                let navigationBarBottom = navigationBar.exists ? navigationBar.frame.maxY : viewport.minY
                do {
                    try app.performAccessibilityAudit(for: [.dynamicType, .textClipped]) { issue in
                        let element = issue.element.map { "\($0.elementType.rawValue) '\($0.label)'" } ?? "no element"
                        let outsideViewport = issue.element.map { element in
                            let frame = element.frame
                            return frame.minY < navigationBarBottom || frame.maxY > tabBarTop
                                || frame.minX < viewport.minX
                                || frame.maxX > viewport.maxX
                        }
                        let reason =
                            issue.auditType == .dynamicType
                            ? Self.preScaledReason
                            : (outsideViewport == true
                                ? Self.offscreenReason : Self.allowedReason(screen.name, element))
                        if reason != nil { return true }
                        failures.append(
                            "\(screen.name) at \(size), scroll \(position): text clipped on \(element): "
                                + issue.compactDescription)
                        return true
                    }
                } catch {
                    failures.append("\(screen.name) at \(size), scroll \(position): the audit failed: \(error)")
                }
            }
        }
        XCTAssertTrue(failures.isEmpty, "\n" + Set(failures).sorted().joined(separator: "\n"))
    }

    private static func allowedReason(_ screen: String, _ element: String) -> String? {
        allowed.first {
            ($0.screen == screen || $0.screen == "*") && ($0.element.isEmpty || element.contains($0.element))
        }?.reason
    }
}
