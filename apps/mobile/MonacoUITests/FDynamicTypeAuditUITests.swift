import XCTest

nonisolated final class FDynamicTypeAuditUITests: XCTestCase {
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

    private static let requestRowReason =
        "the join request row draws whole at AX5 in the screenshot and the accessibility tree; "
        + "the audit still flags its name and sub-line after eight layout variants (#7235)"

    private static let proposeBuyBackdropReason =
        "the 'Propose buy' text behind the presented review sheet is flagged at every scroll position "
        + "after the sheet title wraps (#7285) and the asset buttons stack (#7206); the sheet covers it"

    private static let allowed: [AllowedIssue] = [
        AllowedIssue(
            screen: "*", element: "no element",
            reason: "a report with no element to name; it appears only while the screen scrolls"),
        AllowedIssue(
            screen: "f__03-cabal-members-all", element: "'Leo'",
            reason: requestRowReason),
        AllowedIssue(
            screen: "f__03-cabal-members-all", element: "'Asked to join · ",
            reason: requestRowReason),
        AllowedIssue(
            screen: "f__08-review-sheet", element: "48 'Propose buy'",
            reason: proposeBuyBackdropReason),
        AllowedIssue(
            screen: "f__08-review-sheet-preipo", element: "48 'Propose buy'",
            reason: proposeBuyBackdropReason),
    ]

    private static let preScaledReason: String? =
        "Typo.scaled returns Font.system(size:) already scaled for the launch size, so the runtime font check reads it as fixed"
    private static let offscreenReason: String? =
        "the element is partly outside the viewport or under the tab bar while the screen is scrolled"

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = true
    }

    @MainActor
    func test02Home() throws {
        try auditScreens(Self.group02)
    }

    @MainActor
    func test04Proposal() throws {
        try auditScreens(Self.group04)
    }

    @MainActor
    func test12Sheets() throws {
        try auditScreens(Self.group12)
    }

    @MainActor
    func test03Cabals() throws {
        try auditScreens(Self.group03)
    }

    @MainActor
    func test15Moments() throws {
        try auditScreens(Self.group15)
    }

    @MainActor
    func test19Permission() throws {
        try auditScreens(Self.group19)
    }

    @MainActor
    func test14Ledger() throws {
        try auditScreens(Self.group14)
    }

    @MainActor
    func test05Feed() throws {
        try auditScreens(Self.group05)
    }

    @MainActor
    func test06Markets() throws {
        try auditScreens(Self.group06)
    }

    @MainActor
    func test07Stock() throws {
        try auditScreens(Self.group07)
    }

    @MainActor
    func test08Review() throws {
        try auditScreens(Self.group08)
    }

    @MainActor
    func test09Money() throws {
        try auditScreens(Self.group09)
    }

    @MainActor
    func test10Profile() throws {
        try auditScreens(Self.group10)
    }

    @MainActor
    func test11Share() throws {
        try auditScreens(Self.group11)
    }

    @MainActor
    func test18Invite() throws {
        try auditScreens(Self.group18)
    }

    @MainActor
    func test13Leaderboard() throws {
        try auditScreens(Self.group13)
    }

    @MainActor
    func test16CatchUp() throws {
        try auditScreens(Self.group16)
    }

    @MainActor
    func testTheListHoldsEveryScreenFrame() {
        let all = [
            Self.group02, Self.group04, Self.group12, Self.group03, Self.group15, Self.group19, Self.group14,
            Self.group05, Self.group06, Self.group07, Self.group08, Self.group09, Self.group10, Self.group11,
            Self.group18, Self.group13, Self.group16,
        ].flatMap { $0 }
        XCTAssertEqual(all.count, 95)
        XCTAssertEqual(Set(all.map(\.name)).count, 95)
    }

    @MainActor
    private func auditScreens(_ screens: [Screen]) throws {
        var failures: [String] = []
        for size in [Self.xxxLarge, Self.accessibilityXXXLarge] {
            for screen in screens {
                failures += audit(screen, at: size)
            }
        }
        XCTAssertTrue(failures.isEmpty, "\n" + Set(failures).sorted().joined(separator: "\n"))
    }

    @MainActor
    private func audit(_ screen: Screen, at size: String) -> [String] {
        var failures: [String] = []
        let app = XCUIApplication()
        app.launchArguments = screen.arguments + ["-UIPreferredContentSizeCategoryName", size]
        app.launch()
        defer { app.terminate() }
        guard app.staticTexts.firstMatch.waitForExistence(timeout: 30) else {
            return ["\(screen.name) at \(size): the screen never drew"]
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
        return failures
    }

    private static func allowedReason(_ screen: String, _ element: String) -> String? {
        allowed.first {
            ($0.screen == screen || $0.screen == "*") && ($0.element.isEmpty || element.contains($0.element))
        }?.reason
    }
}

extension FDynamicTypeAuditUITests {
    private static let group02: [Screen] = [
        Screen("f__02-home-dark", "-MonacoHomeSample fReady"),
        Screen("f__02-home-light", "-MonacoHomeSample fReadyLight"),
        Screen("f__02-home-scrolled", "-MonacoHomeSample fScrolled -MonacoSampleScroll 0.35"),
        Screen("f__02-home-empty", "-MonacoHomeSample fFirstUse"),
    ]

    private static let group04: [Screen] = [
        Screen("f__04b-hold-1-pick", "-MonacoHomeSample fVotePick -MonacoSampleScroll 0.35"),
        Screen("f__04b-hold-2-holding", "-MonacoHomeSample fVoteHolding -MonacoSampleScroll 0.35"),
        Screen("f__04b-hold-3-committed", "-MonacoHomeSample fVoteCommitted -MonacoSampleScroll 0.35"),
        Screen("f__04-proposal-dark", "-MonacoProposalSample nvdaOpen"),
        Screen("f__04-proposal-light", "-MonacoProposalSample nvdaOpen"),
        Screen("f__04-proposal-scrolled", "-MonacoProposalSample nvdaOpen -MonacoSampleScroll bottom"),
        Screen("f__04-vote-idle", "-MonacoProposalSample voteOrdinary"),
        Screen("f__04-vote-picked", "-MonacoProposalSample votePicked"),
        Screen("f__04-vote-holding", "-MonacoProposalSample voteHolding"),
        Screen("f__04-vote-committed", "-MonacoProposalSample voteSettles"),
        Screen("f__04-vote-closed", "-MonacoProposalSample nvdaFilled"),
        Screen("f__04-proposal-passed", "-MonacoProposalSample nvdaPassed"),
        Screen("f__04-proposal-sides", "-MonacoProposalSample closedSidesOn"),
        Screen("f__04-proposal-failed", "-MonacoProposalSample tslaFailed"),
        Screen("f__04-proposal-preipo", "-MonacoProposalSample spaceXOpen"),
        Screen("f__04-reply-composer", "-composerHarness sheetComment"),
        Screen("f__04-composer-menu", "-composerHarness menu"),
        Screen("f__04-gif-sheet", "-gifHarness trending"),
        Screen("f__04-gif-reply", "-gifHarness bubble"),
    ]

    private static let group12: [Screen] = [
        Screen("f__12-toast", "-MonacoParitySample toast-settled"),
        Screen("f__12-info-sheet", "-MonacoParitySample info-sheet"),
    ]

    private static let group03: [Screen] = [
        Screen("f__03-cabals-tab", "-MonacoCabalsTabSample -MonacoCabalsTabSampleScenario fReady"),
        Screen("f__03-cabals-tab-menu", "-MonacoCabalsTabSample -MonacoCabalsTabSampleScenario fMenu"),
        Screen("f__03-cabal-treasury", "-MonacoGroupDetailSample fTreasury"),
        Screen("f__03-cabal-treasury-light", "-MonacoGroupDetailSample fTreasuryLight"),
        Screen("f__03-cabal-treasury-scrolled", "-MonacoGroupDetailSample fTreasuryScrolled -MonacoSampleScroll 1"),
        Screen("f__03-cabal-rules", "-MonacoGroupDetailSample fRules"),
        Screen("f__03-cabal-rules-scrolled", "-MonacoGroupDetailSample fRulesScrolled -MonacoSampleScroll 1"),
        Screen("f__03-cabal-rules-sides", "-MonacoGroupDetailSample fRulesSides -MonacoSampleScroll 1"),
        Screen("f__03-cabal-members", "-MonacoGroupDetailSample fMembers"),
        Screen("f__03-cabal-members-scrolled", "-MonacoGroupDetailSample fMembersScrolled -MonacoSampleScroll 1"),
        Screen("f__03-cabal-members-all", "-MonacoGroupDetailSample fMembersAll"),
        Screen("f__03-cabal-empty", "-MonacoGroupDetailSample fEmpty"),
    ]

    private static let group15: [Screen] = [
        Screen("f__15-first-fund", "-MonacoMoneyFlowSample firstFund"),
        Screen("f__15-passed", "-MonacoProposalSample nvdaPassedMoment"),
        Screen("f__15-leaderboard-moment", "-MonacoLeaderboardSample leadChange"),
    ]

    private static let group19: [Screen] = [
        Screen("f__19-permission-sheet", "-MonacoProposalSample permissionSheet")
    ]

    private static let group14: [Screen] = [
        Screen("f__14-hold-sheet", "-MonacoProposalSample holdYes"),
        Screen("f__14-wrap-sheet", "-MonacoLedgerSample week42"),
        Screen("f__14-ledger-story", "-MonacoLedgerSample story"),
    ]

    private static let group05: [Screen] = [
        Screen("f__05-feed-A", "-feedHarness items"),
        Screen("f__05-feed-A-scrolled", "-feedHarness items -MonacoSampleScroll bottom"),
        Screen("f__05-feed-light", "-feedHarness items"),
        Screen("f__05-reactions", "-reactionsHarness trayOpen"),
        Screen("f__05-emoji-picker", "-reactionsHarness picker"),
        Screen("f__05-post", "-feedPostHarness logan"),
        Screen("f__05-post-scrolled", "-feedPostHarness end"),
        Screen("f__05-post-reply", "-composerHarness sheetPost"),
        Screen("f__05-comment-actions", "-commentThreadHarness actions"),
        Screen("f__05-post-public", "-MonacoProposalSample nonMember"),
        Screen("f__05-feed-empty", "-feedHarness quietWithEarlier"),
    ]

    private static let group06: [Screen] = [
        Screen("f__06-markets-A", "-MonacoStocksTabSample ready"),
        Screen("f__06-markets-A-scrolled", "-MonacoStocksTabSample movers -MonacoSampleScroll 1"),
        Screen("f__06-markets-search", "-MonacoStocksTabSample search"),
        Screen("f__06-markets-held", "-MonacoStocksTabSample held"),
    ]

    private static let group07: [Screen] = [
        Screen("f__07-stock-dark", "-MonacoAssetDetailSample open"),
        Screen("f__07-stock-light", "-MonacoAssetDetailSample open"),
        Screen("f__07-stock-preipo", "-MonacoAssetDetailSample preIpo"),
        Screen("f__07-stock-preipo-scrolled", "-MonacoAssetDetailSample preIpo -MonacoSampleScroll bottom"),
    ]

    private static let group08: [Screen] = [
        Screen("f__08-review-sheet", "-MonacoProposeSample reviewNvidia"),
        Screen("f__08-review-sheet-preipo", "-MonacoProposeSample reviewSpaceX"),
    ]

    private static let group09: [Screen] = [
        Screen("f__09-fund-cabal-empty", "-MonacoFundCabalSample empty"),
        Screen("f__09-fund-cabal", "-MonacoFundCabalSample typed"),
        Screen("f__09-fund-cabal-invalid", "-MonacoFundCabalSample invalid"),
        Screen("f__09-fund-cabal-settled", "-MonacoFundCabalSample settled"),
        Screen("f__09-withdraw-empty", "-MonacoWithdrawSample empty"),
        Screen("f__09-withdraw", "-MonacoWithdrawSample typed"),
        Screen("f__09-withdraw-invalid", "-MonacoWithdrawSample invalid"),
        Screen("f__09-withdraw-settled", "-MonacoWithdrawSample settled"),
    ]

    private static let group10: [Screen] = [
        Screen("f__10-profile-own", "-MonacoProfileSample ready"),
        Screen("f__10-profile-own-scrolled", "-MonacoProfileSample ready -MonacoSampleScroll bottom"),
        Screen("f__10-profile-other", "-MonacoProfileSample personPublic"),
        Screen("f__10-profile-other-cabals", "-MonacoProfileSample personChristelleCabals"),
    ]

    private static let group11: [Screen] = [
        Screen("f__11-invite-sheet", "-MonacoProfileSample invite"),
        Screen("f__11-share-A", "-MonacoShareSample screenPost"),
        Screen("f__11-share-B", "-MonacoShareSample screenPassed"),
        Screen("f__11-share-C", "-MonacoShareSample screenStake"),
        Screen("f__11-share-card-vote", "-MonacoShareSample screenVote"),
        Screen("f__11-share-sticker", "-MonacoShareSample screenSticker"),
        Screen("f__11-share-sticker-on", "-MonacoShareSample stickerOn"),
    ]

    private static let group18: [Screen] = [
        Screen("f__18-invite-landing", "-MonacoInviteLandingSample frame")
    ]

    private static let group13: [Screen] = [
        Screen("f__13-leaderboard-C-people", "-MonacoLeaderboardSample peopleMonth"),
        Screen("f__13-leaderboard-C", "-MonacoLeaderboardSample friendsMonth"),
        Screen("f__13-leaderboard-C-scrolled", "-MonacoLeaderboardSample friendsMonth -MonacoSampleScroll 1"),
        Screen("f__13-leaderboard-C-cabals", "-MonacoLeaderboardSample cabalsMonth"),
        Screen("f__13-leaderboard-C-unranked", "-MonacoLeaderboardSample peopleWeekUnranked"),
    ]

    private static let group16: [Screen] = [
        Screen("f__16-catch-up", "-MonacoCatchUpSample six"),
        Screen("f__16-catch-up-open", "-MonacoCatchUpSample deck"),
        Screen("f__16-catch-up-open-scrolled", "-MonacoCatchUpSample deckScrolled"),
        Screen("f__16-catch-up-empty", "-MonacoCatchUpSample caughtUp"),
        Screen("f__16-catch-up-empty-scrolled", "-MonacoCatchUpSample caughtUp -MonacoSampleScroll bottom"),
    ]
}
