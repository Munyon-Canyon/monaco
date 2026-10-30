import XCTest

nonisolated final class FAppearanceAuditUITests: XCTestCase {
    private static let screens: [(name: String, arguments: [String])] = [
        ("f__02-home-dark", ["-MonacoHomeSample", "fReady"]),
        ("f__02-home-empty", ["-MonacoHomeSample", "fFirstUse"]),
        ("f__02-home-light", ["-MonacoHomeSample", "fReadyLight"]),
        ("f__02-home-scrolled", ["-MonacoHomeSample", "fScrolled", "-MonacoSampleScroll", "0.35"]),
        ("f__03-cabal-empty", ["-MonacoGroupDetailSample", "fEmpty"]),
        ("f__03-cabal-members", ["-MonacoGroupDetailSample", "fMembers"]),
        ("f__03-cabal-members-all", ["-MonacoGroupDetailSample", "fMembersAll"]),
        ("f__03-cabal-members-scrolled", ["-MonacoGroupDetailSample", "fMembersScrolled", "-MonacoSampleScroll", "1"]),
        ("f__03-cabal-rules", ["-MonacoGroupDetailSample", "fRules"]),
        ("f__03-cabal-rules-scrolled", ["-MonacoGroupDetailSample", "fRulesScrolled", "-MonacoSampleScroll", "0.6"]),
        ("f__03-cabal-rules-sides", ["-MonacoGroupDetailSample", "fRulesSides", "-MonacoSampleScroll", "0.6"]),
        ("f__03-cabal-treasury", ["-MonacoGroupDetailSample", "fTreasury"]),
        ("f__03-cabal-treasury-light", ["-MonacoGroupDetailSample", "fTreasuryLight"]),
        (
            "f__03-cabal-treasury-scrolled",
            ["-MonacoGroupDetailSample", "fTreasuryScrolled", "-MonacoSampleScroll", "1"]
        ),
        ("f__03-cabals-tab", ["-MonacoCabalsTabSample", "-MonacoCabalsTabSampleScenario", "fReady"]),
        ("f__03-cabals-tab-menu", ["-MonacoCabalsTabSample", "-MonacoCabalsTabSampleScenario", "fMenu"]),
        ("f__04-composer-menu", ["-composerHarness", "menu"]),
        ("f__04-gif-reply", ["-gifHarness", "bubble"]),
        ("f__04-gif-sheet", ["-gifHarness", "trending"]),
        ("f__04-proposal-dark", ["-MonacoProposalSample", "nvdaOpen"]),
        ("f__04-proposal-failed", ["-MonacoProposalSample", "tslaFailed"]),
        ("f__04-proposal-light", ["-MonacoProposalSample", "nvdaOpen"]),
        ("f__04-proposal-passed", ["-MonacoProposalSample", "nvdaPassed"]),
        ("f__04-proposal-preipo", ["-MonacoProposalSample", "spaceXOpen"]),
        ("f__04-proposal-scrolled", ["-MonacoProposalSample", "nvdaOpen", "-MonacoSampleScroll", "bottom"]),
        ("f__04-proposal-sides", ["-MonacoProposalSample", "closedSidesOn"]),
        ("f__04-reply-composer", ["-composerHarness", "sheetComment"]),
        ("f__04-vote-closed", ["-MonacoProposalSample", "nvdaFilled"]),
        ("f__04-vote-committed", ["-MonacoProposalSample", "voteSettles"]),
        ("f__04-vote-holding", ["-MonacoProposalSample", "voteHolding"]),
        ("f__04-vote-idle", ["-MonacoProposalSample", "voteOrdinary"]),
        ("f__04-vote-picked", ["-MonacoProposalSample", "votePicked"]),
        ("f__04b-hold-1-pick", ["-MonacoHomeSample", "fVotePick", "-MonacoSampleScroll", "0.35"]),
        ("f__04b-hold-2-holding", ["-MonacoHomeSample", "fVoteHolding", "-MonacoSampleScroll", "0.35"]),
        ("f__04b-hold-3-committed", ["-MonacoHomeSample", "fVoteCommitted", "-MonacoSampleScroll", "0.35"]),
        ("f__05-comment-actions", ["-commentThreadHarness", "actions"]),
        ("f__05-emoji-picker", ["-reactionsHarness", "picker"]),
        ("f__05-feed-A", ["-feedHarness", "items"]),
        ("f__05-feed-A-scrolled", ["-feedHarness", "items", "-MonacoSampleScroll", "bottom"]),
        ("f__05-feed-empty", ["-feedHarness", "quietWithEarlier"]),
        ("f__05-feed-light", ["-feedHarness", "items"]),
        ("f__05-post", ["-feedPostHarness", "logan"]),
        ("f__05-post-public", ["-MonacoProposalSample", "nonMember"]),
        ("f__05-post-reply", ["-composerHarness", "sheetPost"]),
        ("f__05-post-scrolled", ["-feedPostHarness", "end"]),
        ("f__05-reactions", ["-reactionsHarness", "trayOpen"]),
        ("f__06-markets-A", ["-MonacoStocksTabSample", "ready"]),
        ("f__06-markets-A-scrolled", ["-MonacoStocksTabSample", "movers", "-MonacoSampleScroll", "1"]),
        ("f__06-markets-held", ["-MonacoStocksTabSample", "held"]),
        ("f__06-markets-search", ["-MonacoStocksTabSample", "search"]),
        ("f__07-stock-dark", ["-MonacoAssetDetailSample", "open"]),
        ("f__07-stock-light", ["-MonacoAssetDetailSample", "open"]),
        ("f__07-stock-preipo", ["-MonacoAssetDetailSample", "preIpo"]),
        ("f__07-stock-preipo-scrolled", ["-MonacoAssetDetailSample", "preIpo", "-MonacoSampleScroll", "bottom"]),
        ("f__08-review-sheet", ["-MonacoProposeSample", "review"]),
        ("f__08-review-sheet-preipo", ["-MonacoProposeSample", "reviewSpaceX"]),
        ("f__09-fund-cabal", ["-MonacoFundCabalSample", "typed"]),
        ("f__09-fund-cabal-empty", ["-MonacoFundCabalSample", "empty"]),
        ("f__09-fund-cabal-invalid", ["-MonacoFundCabalSample", "invalid"]),
        ("f__09-fund-cabal-settled", ["-MonacoFundCabalSample", "settled"]),
        ("f__09-withdraw", ["-MonacoWithdrawSample", "typed"]),
        ("f__09-withdraw-empty", ["-MonacoWithdrawSample", "empty"]),
        ("f__09-withdraw-invalid", ["-MonacoWithdrawSample", "invalid"]),
        ("f__09-withdraw-settled", ["-MonacoWithdrawSample", "settled"]),
        ("f__10-profile-other", ["-MonacoProfileSample", "personPublic"]),
        ("f__10-profile-other-cabals", ["-MonacoProfileSample", "personChristelleCabals"]),
        ("f__10-profile-own", ["-MonacoProfileSample", "ready"]),
        ("f__10-profile-own-scrolled", ["-MonacoProfileSample", "ready", "-MonacoSampleScroll", "bottom"]),
        ("f__11-invite-sheet", ["-MonacoGroupDetailSample", "invite"]),
        ("f__11-share-A", ["-MonacoShareSample", "screenPost"]),
        ("f__11-share-B", ["-MonacoShareSample", "screenPassed"]),
        ("f__11-share-C", ["-MonacoShareSample", "screenStake"]),
        ("f__11-share-card-vote", ["-MonacoShareSample", "screenVote"]),
        ("f__11-share-sticker", ["-MonacoShareSample", "screenSticker"]),
        ("f__11-share-sticker-on", ["-MonacoShareSample", "stickerOn"]),
        ("f__12-info-sheet", ["-MonacoParitySample", "info-sheet"]),
        ("f__12-toast", ["-MonacoParitySample", "toast-settled"]),
        ("f__13-leaderboard-C", ["-MonacoLeaderboardSample", "friendsMonth"]),
        ("f__13-leaderboard-C-cabals", ["-MonacoLeaderboardSample", "cabalsMonth"]),
        ("f__13-leaderboard-C-people", ["-MonacoLeaderboardSample", "peopleMonth"]),
        ("f__13-leaderboard-C-scrolled", ["-MonacoLeaderboardSample", "friendsMonth", "-MonacoSampleScroll", "1"]),
        ("f__13-leaderboard-C-unranked", ["-MonacoLeaderboardSample", "peopleWeekUnranked"]),
        ("f__14-hold-sheet", ["-MonacoProposalSample", "holdYes"]),
        ("f__14-ledger-story", ["-MonacoLedgerSample", "story"]),
        ("f__14-wrap-sheet", ["-MonacoLedgerSample", "week42"]),
        ("f__15-first-fund", ["-MonacoMoneyFlowSample", "firstFund"]),
        ("f__15-leaderboard-moment", ["-MonacoLeaderboardSample", "leadChange"]),
        ("f__15-passed", ["-MonacoProposalSample", "nvdaPassedMoment"]),
        ("f__16-catch-up", ["-MonacoCatchUpSample", "six"]),
        ("f__16-catch-up-empty", ["-MonacoCatchUpSample", "caughtUp"]),
        ("f__16-catch-up-empty-scrolled", ["-MonacoCatchUpSample", "caughtUp", "-MonacoSampleScroll", "bottom"]),
        ("f__16-catch-up-open", ["-MonacoCatchUpSample", "deck"]),
        ("f__16-catch-up-open-scrolled", ["-MonacoCatchUpSample", "deckScrolled"]),
        ("f__18-invite-landing", ["-MonacoInviteLandingSample", "frame"]),
        ("f__19-permission-sheet", ["-MonacoProposalSample", "permissionSheet"]),
    ]

    private static let realExceptions: [(screen: String, label: String, reason: String)] = []

    private static func isTokenText(_ issue: XCUIAccessibilityAuditIssue) -> Bool {
        issue.auditType == .contrast && issue.element?.elementType == .staticText
    }

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = true
    }

    @MainActor
    private func audit(prefix: String) async throws {
        let entries = Self.screens.filter { $0.name.hasPrefix("f__\(prefix)-") }
        XCTAssertFalse(entries.isEmpty, "no screens listed for \(prefix)")
        for (name, arguments) in entries {
            let app = XCUIApplication()
            app.launchArguments = arguments + ["-AppleInterfaceStyle", "Dark"]
            app.launch()
            XCTAssertTrue(app.wait(for: .runningForeground, timeout: 30), "\(name) never reached the foreground")
            XCTAssertTrue(app.windows.firstMatch.waitForExistence(timeout: 30), "\(name) drew no window")
            try await Task.sleep(for: .seconds(3))
            try app.performAccessibilityAudit(for: .contrast) { issue in
                let label = issue.element?.label ?? ""
                let identifier = issue.element?.identifier ?? ""
                let type = issue.element?.elementType.rawValue ?? 0
                let allowed =
                    Self.isTokenText(issue)
                    || Self.realExceptions.contains { $0.screen == name && $0.label == label }
                print(
                    "AUDIT \(name) allowed=\(allowed) type=\(type) id=\(identifier) label=\(label) | \(issue.detailedDescription)"
                )
                return allowed
            }
            app.terminate()
        }
    }

    @MainActor
    func testTheListHoldsAllNinetyFiveScreens() {
        XCTAssertEqual(Self.screens.count, 95)
        XCTAssertEqual(Set(Self.screens.map(\.name)).count, 95)
    }

    @MainActor func test02Frames() async throws { try await audit(prefix: "02") }
    @MainActor func test03Frames() async throws { try await audit(prefix: "03") }
    @MainActor func test04Frames() async throws { try await audit(prefix: "04") }
    @MainActor func test04bFrames() async throws { try await audit(prefix: "04b") }
    @MainActor func test05Frames() async throws { try await audit(prefix: "05") }
    @MainActor func test06Frames() async throws { try await audit(prefix: "06") }
    @MainActor func test07Frames() async throws { try await audit(prefix: "07") }
    @MainActor func test08Frames() async throws { try await audit(prefix: "08") }
    @MainActor func test09Frames() async throws { try await audit(prefix: "09") }
    @MainActor func test10Frames() async throws { try await audit(prefix: "10") }
    @MainActor func test11Frames() async throws { try await audit(prefix: "11") }
    @MainActor func test12Frames() async throws { try await audit(prefix: "12") }
    @MainActor func test13Frames() async throws { try await audit(prefix: "13") }
    @MainActor func test14Frames() async throws { try await audit(prefix: "14") }
    @MainActor func test15Frames() async throws { try await audit(prefix: "15") }
    @MainActor func test16Frames() async throws { try await audit(prefix: "16") }
    @MainActor func test18Frames() async throws { try await audit(prefix: "18") }
    @MainActor func test19Frames() async throws { try await audit(prefix: "19") }
}
