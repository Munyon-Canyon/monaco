import XCTest

enum V3AuditScreens {
    static let allowed: [(screen: String, identifier: String, type: String, reason: String)] = [
        (
            "cabals__cabal__ready", "cabal-slice-cash", "desc",
            "figure reads as typed; the audit's text heuristic flags a currency string, and the label wording is a follow-up in accessibility.md"
        ),
        (
            "cabals__cabal__ready", "cabal-slice-value", "desc",
            "figure reads as typed; the audit's text heuristic flags a currency string, and the label wording is a follow-up in accessibility.md"
        ),
        (
            "cabals__invite__share", "cabal-invite-link", "desc",
            "the invite URL reads as typed; the heuristic flags a link string"
        ),
        (
            "home__home__loss", "home-portfolio-total", "desc",
            "figure reads as typed; the audit's text heuristic flags a currency string, and the label wording is a follow-up in accessibility.md"
        ),
        (
            "markets__instrument__ready", "asset-detail-price", "desc",
            "figure reads as typed; the audit's text heuristic flags a currency string, and the label wording is a follow-up in accessibility.md"
        ),
        (
            "money__add-money__ready", "deposit-screen-balance-value", "desc",
            "figure reads as typed; the audit's text heuristic flags a currency string, and the label wording is a follow-up in accessibility.md"
        ),
        (
            "money__transaction__completed", "account-txn-receipt-amount", "desc",
            "figure reads as typed; the audit's text heuristic flags a currency string, and the label wording is a follow-up in accessibility.md"
        ),
        (
            "proposals__proposal__open", "", "desc",
            "figure reads as typed; the audit's text heuristic flags a currency string, and the label wording is a follow-up in accessibility.md"
        ),
        (
            "cabals__chat__ready", "chat-day-s5", "hit",
            "the day separator is a static label, not a control; the audit reads it as a hit region"
        ),
        (
            "cabals__holding__ready", "", "detect",
            "an unidentified text run the audit cannot map to an element; tracked in accessibility.md"
        ),
        (
            "home__home__loss", "", "detect",
            "an unidentified text run the audit cannot map to an element; tracked in accessibility.md"
        ),
        (
            "markets__instrument__ready", "", "detect",
            "an unidentified text run the audit cannot map to an element; tracked in accessibility.md"
        ),
        (
            "profile__person__public", "", "detect",
            "an unidentified text run the audit cannot map to an element; tracked in accessibility.md"
        ),
        (
            "proposals__proposal__open", "", "detect",
            "an unidentified text run the audit cannot map to an element; tracked in accessibility.md"
        ),
        (
            "proposals__votes__open", "", "detect",
            "an unidentified text run the audit cannot map to an element; tracked in accessibility.md"
        ),
    ]

    static let all: [(id: String, arguments: String)] = [
        ("access__signin__phone", "-MonacoWelcomeSample signInPhone"),
        ("access__verify__ready", "-MonacoWelcomeSample code"),
        ("home__home__loss", "-MonacoHomeSample loss"),
        ("home__balances__ready", "-balancesHarness ready"),
        ("home__notifications__unread", "-activityFeedHarness unread"),
        ("money__add-money__ready", "-MonacoMoneyFlowSample addMoney"),
        ("money__card-funding__handoff", "-MonacoMoneyFlowSample cardFunding"),
        ("money__cash-out__ready", "-cashOutHarness slice"),
        ("money__deposit__address", "-MonacoMoneyFlowSample depositAddress"),
        ("money__fund-cabal__ready", "-MonacoMoneyFlowSample fundCabal"),
        ("money__network__ready", "-MonacoMoneyFlowSample networks"),
        ("money__withdraw__ready", "-MonacoMoneyFlowSample withdraw"),
        ("money__activity__ready", "-accountActivityHarness ready"),
        ("money__group-cash__ready", "-groupCashHarness ready"),
        ("money__transaction__completed", "-receiptHarness completed"),
        ("proposals__propose__ready", "-MonacoProposeSample chooser"),
        ("proposals__review-proposal__ready", "-MonacoProposeSample review"),
        ("proposals__select-stock__buy", "-MonacoProposeSample buyList"),
        ("proposals__trade-amount__ready", "-MonacoProposeSample amount"),
        ("proposals__proposal__open", "-MonacoProposalSample open"),
        ("proposals__proposals__open", "-MonacoProposalSample allOpen"),
        ("proposals__votes__open", "-MonacoProposalSample votesOpen"),
        ("cabals__cabals__ready", "-MonacoCabalsTabSample -MonacoCabalsTabSampleScenario ready"),
        ("cabals__create-cabal__ready", "-MonacoCreateCabalSample ready"),
        ("cabals__discover__results", "-MonacoDiscoverSample results"),
        ("cabals__cabal__ready", "-MonacoGroupDetailSample ready"),
        ("cabals__cabal-info__member", "-MonacoGroupDetailSample info"),
        ("cabals__invite__share", "-MonacoGroupDetailSample invite"),
        ("cabals__members__members", "-MonacoGroupDetailSample members"),
        ("cabals__public-cabal__preview", "-MonacoGroupDetailSample publicPreview"),
        ("cabals__chat__ready", "-MonacoChatSampleQA -MonacoChatSampleProposal"),
        ("cabals__holding__ready", "-MonacoCabalHoldingSample ready"),
        ("markets__instrument__ready", "-MonacoAssetDetailSample open"),
        ("markets__markets__ready", "-MonacoStocksTabSample ready"),
        ("markets__watchlist__ready", "-MonacoWatchlistSample ready"),
        ("feed__feed__all", "-feedHarness items"),
        ("feed__post__ready", "-feedPostHarness ready"),
        ("explore__leaderboard__cabals", "-MonacoLeaderboardSample cabals"),
        ("profile__edit-profile__ready", "-MonacoProfileSample editProfileReady"),
        ("profile__people__results", "-friendsHarness results"),
        ("profile__person__public", "-MonacoProfileSample personPublic"),
        ("profile__profile__ready", "-MonacoProfileSample ready"),
        ("profile__settings__ready", "-settingsHarness on"),
    ]
}

nonisolated final class V3AccessibilityAuditUITests: XCTestCase {

    private static let minimumTarget: CGFloat = 44
    private static let maxSwipes = 8

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = true
    }

    @MainActor
    func testEveryAllowedFindingHasAReason() async {
        for entry in V3AuditScreens.allowed {
            XCTAssertFalse(entry.reason.isEmpty, "\(entry.screen) \(entry.identifier) \(entry.type) has no reason")
        }
    }

    @MainActor
    func testTheListHoldsTheFortyThreeDefaults() async {
        XCTAssertEqual(V3AuditScreens.all.count, 43)
        XCTAssertEqual(Set(V3AuditScreens.all.map(\.id)).count, 43)
    }

    @MainActor
    func testReduceMotionScreensDraw() async throws {
        for id in [
            "home__home__loss", "cabals__cabal__ready", "proposals__review-proposal__ready", "money__fund-cabal__ready",
        ] {
            guard let screen = V3AuditScreens.all.first(where: { $0.id == id }) else {
                return XCTFail("no launch line for \(id)")
            }
            let app = XCUIApplication()
            app.launchArguments =
                screen.arguments.split(separator: " ").map(String.init)
                + ["-MonacoSampleAct", "-UIAccessibilityReduceMotionEnabled", "YES"]
            app.launch()
            let first = app.descendants(matching: .any).matching(NSPredicate(format: "label != ''")).firstMatch
            XCTAssertTrue(first.waitForExistence(timeout: 30), "\(id) never drew with Reduce Motion on")
            app.terminate()
        }
    }

    @MainActor
    private func audit(_ id: String) async throws {
        guard let screen = V3AuditScreens.all.first(where: { $0.id == id }) else {
            return XCTFail("no launch line for \(id)")
        }
        let app = XCUIApplication()
        app.launchArguments = screen.arguments.split(separator: " ").map(String.init) + ["-MonacoSampleAct"]
        app.launch()
        defer { app.terminate() }

        let first = app.descendants(matching: .any).matching(NSPredicate(format: "label != ''")).firstMatch
        XCTAssertTrue(first.waitForExistence(timeout: 30), "\(id) never drew")
        try await Task.sleep(nanoseconds: 1_500_000_000)

        var seen = Set<String>()
        var last = ""
        for pass in 0...Self.maxSwipes {
            let signature = report(app, id: id, pass: pass, seen: &seen)
            try runAudit(app, id: id)
            if signature == last { break }
            last = signature
            app.swipeUp()
            try await Task.sleep(nanoseconds: 700_000_000)
        }
    }

    @MainActor
    private func runAudit(_ app: XCUIApplication, id: String) throws {
        try app.performAccessibilityAudit(
            for: [.hitRegion, .sufficientElementDescription, .elementDetection, .trait]
        ) { issue in
            let identifier = issue.element?.identifier ?? ""
            let type = Self.name(issue.auditType)
            print("audit|\(id)|\(identifier)|\(issue.element?.label ?? "")|\(type)|\(issue.compactDescription)")
            return V3AuditScreens.allowed.contains {
                $0.screen == id && $0.identifier == identifier && $0.type == type && !$0.reason.isEmpty
            }
        }
    }

    private static func name(_ type: XCUIAccessibilityAuditType) -> String {
        switch type {
        case .hitRegion: "hit"
        case .sufficientElementDescription: "desc"
        case .elementDetection: "detect"
        case .trait: "trait"
        default: "other"
        }
    }

    @MainActor
    private func report(_ app: XCUIApplication, id: String, pass: Int, seen: inout Set<String>) -> String {
        guard let snapshot = try? app.snapshot() else { return "" }
        let interactive: Set<XCUIElement.ElementType> = [
            .button, .link, .switch, .toggle, .textField, .secureTextField, .searchField, .slider,
            .segmentedControl, .stepper, .menuItem, .popUpButton, .tab,
        ]
        var labels: [String] = []
        func walk(_ node: XCUIElementSnapshot) {
            let frame = node.frame
            let kind = node.elementType.rawValue
            let key = "\(kind)|\(node.identifier)|\(node.label)|\(Int(frame.minX))|\(Int(frame.minY))"
            if !node.label.isEmpty || interactive.contains(node.elementType) {
                labels.append(node.label)
                if seen.insert(key).inserted {
                    let selected = node.isSelected ? "selected" : ""
                    print(
                        "a11y|\(id)|\(pass)|type \(kind)|\(node.identifier)|\(node.label)|\(node.value ?? "")|\(selected)|\(Int(frame.width))x\(Int(frame.height))"
                    )
                    let small = frame.width < Self.minimumTarget || frame.height < Self.minimumTarget
                    if interactive.contains(node.elementType), small, frame.width > 0 {
                        print(
                            "small|\(id)|type \(kind)|\(node.identifier)|\(node.label)|\(Int(frame.width))x\(Int(frame.height))"
                        )
                    }
                }
            }
            for child in node.children { walk(child) }
        }
        walk(snapshot)
        return labels.joined(separator: "\n")
    }
}

extension V3AccessibilityAuditUITests {
    @MainActor
    func testAccessSigninPhone() async throws {
        try await audit("access__signin__phone")
    }

    @MainActor
    func testAccessVerifyReady() async throws {
        try await audit("access__verify__ready")
    }

    @MainActor
    func testHomeHomeLoss() async throws {
        try await audit("home__home__loss")
    }

    @MainActor
    func testHomeBalancesReady() async throws {
        try await audit("home__balances__ready")
    }

    @MainActor
    func testHomeNotificationsUnread() async throws {
        try await audit("home__notifications__unread")
    }

    @MainActor
    func testMoneyAddMoneyReady() async throws {
        try await audit("money__add-money__ready")
    }

    @MainActor
    func testMoneyCardFundingHandoff() async throws {
        try await audit("money__card-funding__handoff")
    }

    @MainActor
    func testMoneyCashOutReady() async throws {
        try await audit("money__cash-out__ready")
    }

    @MainActor
    func testMoneyDepositAddress() async throws {
        try await audit("money__deposit__address")
    }

    @MainActor
    func testMoneyFundCabalReady() async throws {
        try await audit("money__fund-cabal__ready")
    }

    @MainActor
    func testMoneyNetworkReady() async throws {
        try await audit("money__network__ready")
    }

    @MainActor
    func testMoneyWithdrawReady() async throws {
        try await audit("money__withdraw__ready")
    }

    @MainActor
    func testMoneyActivityReady() async throws {
        try await audit("money__activity__ready")
    }

    @MainActor
    func testMoneyGroupCashReady() async throws {
        try await audit("money__group-cash__ready")
    }

    @MainActor
    func testMoneyTransactionCompleted() async throws {
        try await audit("money__transaction__completed")
    }

    @MainActor
    func testProposalsProposeReady() async throws {
        try await audit("proposals__propose__ready")
    }

    @MainActor
    func testProposalsReviewProposalReady() async throws {
        try await audit("proposals__review-proposal__ready")
    }

    @MainActor
    func testProposalsSelectStockBuy() async throws {
        try await audit("proposals__select-stock__buy")
    }

    @MainActor
    func testProposalsTradeAmountReady() async throws {
        try await audit("proposals__trade-amount__ready")
    }

    @MainActor
    func testProposalsProposalOpen() async throws {
        try await audit("proposals__proposal__open")
    }

    @MainActor
    func testProposalsProposalsOpen() async throws {
        try await audit("proposals__proposals__open")
    }

    @MainActor
    func testProposalsVotesOpen() async throws {
        try await audit("proposals__votes__open")
    }

    @MainActor
    func testCabalsCabalsReady() async throws {
        try await audit("cabals__cabals__ready")
    }

    @MainActor
    func testCabalsCreateCabalReady() async throws {
        try await audit("cabals__create-cabal__ready")
    }

    @MainActor
    func testCabalsDiscoverResults() async throws {
        try await audit("cabals__discover__results")
    }

    @MainActor
    func testCabalsCabalReady() async throws {
        try await audit("cabals__cabal__ready")
    }

    @MainActor
    func testCabalsCabalInfoMember() async throws {
        try await audit("cabals__cabal-info__member")
    }

    @MainActor
    func testCabalsInviteShare() async throws {
        try await audit("cabals__invite__share")
    }

    @MainActor
    func testCabalsMembersMembers() async throws {
        try await audit("cabals__members__members")
    }

    @MainActor
    func testCabalsPublicCabalPreview() async throws {
        try await audit("cabals__public-cabal__preview")
    }

    @MainActor
    func testCabalsChatReady() async throws {
        try await audit("cabals__chat__ready")
    }

    @MainActor
    func testCabalsHoldingReady() async throws {
        try await audit("cabals__holding__ready")
    }

    @MainActor
    func testMarketsInstrumentReady() async throws {
        try await audit("markets__instrument__ready")
    }

    @MainActor
    func testMarketsMarketsReady() async throws {
        try await audit("markets__markets__ready")
    }

    @MainActor
    func testMarketsWatchlistReady() async throws {
        try await audit("markets__watchlist__ready")
    }

    @MainActor
    func testFeedFeedAll() async throws {
        try await audit("feed__feed__all")
    }

    @MainActor
    func testFeedPostReady() async throws {
        try await audit("feed__post__ready")
    }

    @MainActor
    func testExploreLeaderboardCabals() async throws {
        try await audit("explore__leaderboard__cabals")
    }

    @MainActor
    func testProfileEditProfileReady() async throws {
        try await audit("profile__edit-profile__ready")
    }

    @MainActor
    func testProfilePeopleResults() async throws {
        try await audit("profile__people__results")
    }

    @MainActor
    func testProfilePersonPublic() async throws {
        try await audit("profile__person__public")
    }

    @MainActor
    func testProfileProfileReady() async throws {
        try await audit("profile__profile__ready")
    }

    @MainActor
    func testProfileSettingsReady() async throws {
        try await audit("profile__settings__ready")
    }
}
