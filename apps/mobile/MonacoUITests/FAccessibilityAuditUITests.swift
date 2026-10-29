import XCTest

enum FAuditScreens {
    static let allowed: [(screen: String, identifier: String, type: String, reason: String)] = []

    static let all: [(id: String, arguments: String)] = [
        ("f__02-home-dark", "-MonacoHomeSample fReady"),
        ("f__02-home-light", "-MonacoHomeSample fReadyLight"),
        ("f__02-home-scrolled", "-MonacoHomeSample fScrolled -MonacoSampleScroll 0.35"),
        ("f__02-home-empty", "-MonacoHomeSample fFirstUse"),
        ("f__04b-hold-1-pick", "-MonacoHomeSample fVotePick -MonacoSampleScroll 0.35"),
        ("f__04b-hold-2-holding", "-MonacoHomeSample fVoteHolding -MonacoSampleScroll 0.35"),
        ("f__04b-hold-3-committed", "-MonacoHomeSample fVoteCommitted -MonacoSampleScroll 0.35"),
        ("f__12-toast", "-MonacoParitySample toast-settled"),
        ("f__03-cabals-tab", "-MonacoCabalsTabSample -MonacoCabalsTabSampleScenario fReady"),
        ("f__03-cabals-tab-menu", "-MonacoCabalsTabSample -MonacoCabalsTabSampleScenario fMenu"),
        ("f__03-cabal-treasury", "-MonacoGroupDetailSample fTreasury"),
        ("f__03-cabal-treasury-light", "-MonacoGroupDetailSample fTreasuryLight"),
        ("f__03-cabal-treasury-scrolled", "-MonacoGroupDetailSample fTreasuryScrolled -MonacoSampleScroll 1"),
        ("f__03-cabal-rules", "-MonacoGroupDetailSample fRules"),
        ("f__03-cabal-rules-scrolled", "-MonacoGroupDetailSample fRulesScrolled -MonacoSampleScroll 0.6"),
        ("f__03-cabal-rules-sides", "-MonacoGroupDetailSample fRulesSides -MonacoSampleScroll 0.6"),
        ("f__03-cabal-members", "-MonacoGroupDetailSample fMembers"),
        ("f__03-cabal-members-scrolled", "-MonacoGroupDetailSample fMembersScrolled -MonacoSampleScroll 1"),
        ("f__03-cabal-members-all", "-MonacoGroupDetailSample fMembersAll"),
        ("f__03-cabal-empty", "-MonacoGroupDetailSample fEmpty"),
        ("f__15-first-fund", "-MonacoMoneyFlowSample firstFund"),
        ("f__04-proposal-dark", "-MonacoProposalSample nvdaOpen"),
        ("f__04-proposal-light", "-MonacoProposalSample nvdaOpen"),
        ("f__04-proposal-scrolled", "-MonacoProposalSample nvdaOpen -MonacoSampleScroll bottom"),
        ("f__04-vote-idle", "-MonacoProposalSample voteOrdinary"),
        ("f__04-vote-picked", "-MonacoProposalSample votePicked"),
        ("f__04-vote-holding", "-MonacoProposalSample voteHolding"),
        ("f__04-vote-committed", "-MonacoProposalSample voteSettles"),
        ("f__19-permission-sheet", "-MonacoProposalSample permissionSheet"),
        ("f__04-vote-closed", "-MonacoProposalSample nvdaFilled"),
        ("f__04-proposal-passed", "-MonacoProposalSample nvdaPassed"),
        ("f__15-passed", "-MonacoProposalSample nvdaPassedMoment"),
        ("f__04-proposal-sides", "-MonacoProposalSample closedSidesOn"),
        ("f__04-proposal-failed", "-MonacoProposalSample tslaFailed"),
        ("f__04-proposal-preipo", "-MonacoProposalSample spaceXOpen"),
        ("f__04-reply-composer", "-composerHarness sheetComment"),
        ("f__04-composer-menu", "-composerHarness menu"),
        ("f__04-gif-sheet", "-gifHarness trending"),
        ("f__04-gif-reply", "-gifHarness bubble"),
        ("f__14-hold-sheet", "-MonacoProposalSample holdYes"),
        ("f__05-feed-A", "-feedHarness items"),
        ("f__05-feed-A-scrolled", "-feedHarness items -MonacoSampleScroll bottom"),
        ("f__05-feed-light", "-feedHarness items"),
        ("f__05-reactions", "-reactionsHarness trayOpen"),
        ("f__05-emoji-picker", "-reactionsHarness picker"),
        ("f__05-post", "-feedPostHarness logan"),
        ("f__05-post-scrolled", "-feedPostHarness end"),
        ("f__05-post-reply", "-composerHarness sheetPost"),
        ("f__05-comment-actions", "-commentThreadHarness actions"),
        ("f__05-post-public", "-MonacoProposalSample nonMember"),
        ("f__05-feed-empty", "-feedHarness quietWithEarlier"),
        ("f__06-markets-A", "-MonacoStocksTabSample ready"),
        ("f__06-markets-A-scrolled", "-MonacoStocksTabSample movers -MonacoSampleScroll 1"),
        ("f__06-markets-search", "-MonacoStocksTabSample search"),
        ("f__06-markets-held", "-MonacoStocksTabSample held"),
        ("f__07-stock-dark", "-MonacoAssetDetailSample open"),
        ("f__07-stock-light", "-MonacoAssetDetailSample open"),
        ("f__07-stock-preipo", "-MonacoAssetDetailSample preIpo"),
        ("f__07-stock-preipo-scrolled", "-MonacoAssetDetailSample preIpo -MonacoSampleScroll bottom"),
        ("f__08-review-sheet", "-MonacoProposeSample review"),
        ("f__08-review-sheet-preipo", "-MonacoProposeSample reviewSpaceX"),
        ("f__12-info-sheet", "-MonacoParitySample info-sheet"),
        ("f__09-fund-cabal-empty", "-MonacoFundCabalSample empty"),
        ("f__09-fund-cabal", "-MonacoFundCabalSample typed"),
        ("f__09-fund-cabal-invalid", "-MonacoFundCabalSample invalid"),
        ("f__09-fund-cabal-settled", "-MonacoFundCabalSample settled"),
        ("f__09-withdraw-empty", "-MonacoWithdrawSample empty"),
        ("f__09-withdraw", "-MonacoWithdrawSample typed"),
        ("f__09-withdraw-invalid", "-MonacoWithdrawSample invalid"),
        ("f__09-withdraw-settled", "-MonacoWithdrawSample settled"),
        ("f__10-profile-own", "-MonacoProfileSample ready"),
        ("f__10-profile-own-scrolled", "-MonacoProfileSample ready -MonacoSampleScroll bottom"),
        ("f__10-profile-other", "-MonacoProfileSample personPublic"),
        ("f__10-profile-other-cabals", "-MonacoProfileSample personChristelleCabals"),
        ("f__11-invite-sheet", "-MonacoGroupDetailSample invite"),
        ("f__18-invite-landing", "-MonacoInviteLandingSample frame"),
        ("f__11-share-A", "-MonacoShareSample screenPost"),
        ("f__11-share-B", "-MonacoShareSample screenPassed"),
        ("f__11-share-C", "-MonacoShareSample screenStake"),
        ("f__11-share-card-vote", "-MonacoShareSample screenVote"),
        ("f__11-share-sticker", "-MonacoShareSample screenSticker"),
        ("f__11-share-sticker-on", "-MonacoShareSample stickerOn"),
        ("f__13-leaderboard-C-people", "-MonacoLeaderboardSample peopleMonth"),
        ("f__13-leaderboard-C", "-MonacoLeaderboardSample friendsMonth"),
        ("f__13-leaderboard-C-scrolled", "-MonacoLeaderboardSample friendsMonth -MonacoSampleScroll 1"),
        ("f__13-leaderboard-C-cabals", "-MonacoLeaderboardSample cabalsMonth"),
        ("f__13-leaderboard-C-unranked", "-MonacoLeaderboardSample peopleWeekUnranked"),
        ("f__15-leaderboard-moment", "-MonacoLeaderboardSample leadChange"),
        ("f__14-wrap-sheet", "-MonacoLedgerSample week42"),
        ("f__14-ledger-story", "-MonacoLedgerSample story"),
        ("f__16-catch-up", "-MonacoCatchUpSample six"),
        ("f__16-catch-up-open", "-MonacoCatchUpSample deck"),
        ("f__16-catch-up-open-scrolled", "-MonacoCatchUpSample deckScrolled"),
        ("f__16-catch-up-empty", "-MonacoCatchUpSample caughtUp"),
        ("f__16-catch-up-empty-scrolled", "-MonacoCatchUpSample caughtUp -MonacoSampleScroll bottom"),
    ]
}
nonisolated final class FAccessibilityAuditUITests: XCTestCase {

    private static let minimumTarget: CGFloat = 44
    private static let maxSwipes = 8

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = true
    }

    @MainActor
    func testEveryAllowedFindingHasAReason() async {
        for entry in FAuditScreens.allowed {
            XCTAssertFalse(entry.reason.isEmpty, "\(entry.screen) \(entry.identifier) \(entry.type) has no reason")
        }
    }

    @MainActor
    func testTheListHoldsTheNinetyFiveScreens() async {
        XCTAssertEqual(FAuditScreens.all.count, 95)
        XCTAssertEqual(Set(FAuditScreens.all.map(\.id)).count, 95)
    }

    @MainActor
    func testReduceMotionScreensDraw() async throws {
        for id in [
            "f__02-home-dark", "f__04-vote-holding", "f__09-fund-cabal-settled", "f__12-toast",
            "f__15-leaderboard-moment",
        ] {
            let app = try launch(id, extra: ["-UIAccessibilityReduceMotionEnabled", "YES"])
            let first = app.descendants(matching: .any).matching(NSPredicate(format: "label != ''")).firstMatch
            XCTAssertTrue(first.waitForExistence(timeout: 30), "\(id) never drew with Reduce Motion on")
            app.terminate()
        }
    }

    @MainActor
    func testHoldCommitsOnPressAndHold() async throws {
        for id in ["f__04-vote-idle", "f__04b-hold-1-pick", "f__14-hold-sheet"] {
            let app = try launch(id)
            defer { app.terminate() }
            let yes = voteButton(app, "yes")
            XCTAssertTrue(yes.waitForExistence(timeout: 30), "\(id) has no Yes button")
            yes.tap()
            yes.press(forDuration: 0.8)
            XCTAssertTrue(committed(app).waitForExistence(timeout: 5), "\(id) did not commit after a hold")
        }
    }

    @MainActor
    func testShortPressDoesNotCommit() async throws {
        for id in ["f__04-vote-idle", "f__04b-hold-1-pick", "f__14-hold-sheet"] {
            let app = try launch(id)
            defer { app.terminate() }
            let yes = voteButton(app, "yes")
            XCTAssertTrue(yes.waitForExistence(timeout: 30), "\(id) has no Yes button")
            yes.press(forDuration: 0.2)
            XCTAssertFalse(committed(app).waitForExistence(timeout: 1.5), "\(id) committed on a short press")
        }
    }

    @MainActor
    func testVoteButtonsAreLabelled() async throws {
        for id in ["f__04-vote-idle", "f__04b-hold-1-pick", "f__14-hold-sheet"] {
            let app = try launch(id)
            defer { app.terminate() }
            let yes = voteButton(app, "yes")
            let no = voteButton(app, "no")
            XCTAssertTrue(yes.waitForExistence(timeout: 30), "\(id) has no Yes button")
            XCTAssertTrue(yes.label.hasPrefix("Yes, "), "\(id) Yes reads \(yes.label)")
            XCTAssertTrue(no.label.hasPrefix("No, "), "\(id) No reads \(no.label)")
            if id == "f__04-vote-idle" {
                XCTAssertFalse(yes.isSelected, "\(id) Yes is selected before the pick")
                yes.tap()
            }
            XCTAssertTrue(yes.isSelected, "\(id) Yes is not selected after the pick")
            XCTAssertFalse(no.isSelected, "\(id) No is selected after Yes was picked")
            XCTAssertTrue(yes.label.hasPrefix("Yes, "), "\(id) Yes reads \(yes.label) after the pick")
        }
    }

    @MainActor
    func testUndoToastIsReachable() async throws {
        let settled = try launch("f__12-toast")
        XCTAssertTrue(
            settled.descendants(matching: .any)["monaco-toast-banner"].waitForExistence(timeout: 30),
            "the settling vote's toast never showed"
        )
        XCTAssertFalse(settled.buttons["monaco-toast-action"].exists, "the settling vote's toast has an Undo")
        settled.terminate()

        let app = try launch("f__04-vote-idle")
        defer { app.terminate() }
        let yes = voteButton(app, "yes")
        XCTAssertTrue(yes.waitForExistence(timeout: 30), "no Yes button")
        yes.tap()
        yes.press(forDuration: 0.8)
        let undo = app.buttons["monaco-toast-action"]
        XCTAssertTrue(undo.waitForExistence(timeout: 5), "an ordinary vote's toast has no Undo")
        XCTAssertEqual(undo.label, "Undo")
        XCTAssertTrue(undo.isHittable, "Undo is not hittable")
        XCTAssertGreaterThanOrEqual(
            undo.frame.width.rounded(), Self.minimumTarget, "Undo is \(undo.frame.width) pt wide")
        XCTAssertGreaterThanOrEqual(
            undo.frame.height.rounded(), Self.minimumTarget, "Undo is \(undo.frame.height) pt tall")
    }

    @MainActor
    private func launch(_ id: String, extra: [String] = []) throws -> XCUIApplication {
        guard let screen = FAuditScreens.all.first(where: { $0.id == id }) else {
            throw XCTSkip("no launch line for \(id)")
        }
        let app = XCUIApplication()
        app.launchArguments = screen.arguments.split(separator: " ").map(String.init) + ["-MonacoSampleAct"] + extra
        app.launch()
        return app
    }

    @MainActor
    private func voteButton(_ app: XCUIApplication, _ side: String) -> XCUIElement {
        app.buttons.matching(NSPredicate(format: "identifier CONTAINS %@", "vote-\(side)")).firstMatch
    }

    @MainActor
    private func committed(_ app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier == 'proposal-voted' OR label BEGINSWITH 'Voted '")
        ).firstMatch
    }

    @MainActor
    private func audit(group: String) async throws {
        for screen in FAuditScreens.all where screen.id.hasPrefix("f__\(group)") {
            try await audit(screen.id)
        }
    }

    @MainActor
    private func audit(_ id: String) async throws {
        let app = try launch(id)
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
            return FAuditScreens.allowed.contains {
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
                        "a11y|\(id)|\(pass)|type \(kind)|\(node.identifier)|\(node.label)|\(node.value ?? "")|\(selected)|\(Int(frame.width.rounded()))x\(Int(frame.height.rounded()))"
                    )
                    let small =
                        frame.width.rounded() < Self.minimumTarget || frame.height.rounded() < Self.minimumTarget
                    if interactive.contains(node.elementType), small, frame.width > 0 {
                        print(
                            "small|\(id)|type \(kind)|\(node.identifier)|\(node.label)|\(Int(frame.width.rounded()))x\(Int(frame.height.rounded()))"
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

extension FAccessibilityAuditUITests {
    @MainActor
    func testGroup02() async throws {
        try await audit(group: "02")
    }

    @MainActor
    func testGroup04() async throws {
        try await audit(group: "04")
    }

    @MainActor
    func testGroup12() async throws {
        try await audit(group: "12")
    }

    @MainActor
    func testGroup03() async throws {
        try await audit(group: "03")
    }

    @MainActor
    func testGroup15() async throws {
        try await audit(group: "15")
    }

    @MainActor
    func testGroup19() async throws {
        try await audit(group: "19")
    }

    @MainActor
    func testGroup14() async throws {
        try await audit(group: "14")
    }

    @MainActor
    func testGroup05() async throws {
        try await audit(group: "05")
    }

    @MainActor
    func testGroup06() async throws {
        try await audit(group: "06")
    }

    @MainActor
    func testGroup07() async throws {
        try await audit(group: "07")
    }

    @MainActor
    func testGroup08() async throws {
        try await audit(group: "08")
    }

    @MainActor
    func testGroup09() async throws {
        try await audit(group: "09")
    }

    @MainActor
    func testGroup10() async throws {
        try await audit(group: "10")
    }

    @MainActor
    func testGroup11() async throws {
        try await audit(group: "11")
    }

    @MainActor
    func testGroup18() async throws {
        try await audit(group: "18")
    }

    @MainActor
    func testGroup13() async throws {
        try await audit(group: "13")
    }

    @MainActor
    func testGroup16() async throws {
        try await audit(group: "16")
    }
}
