import XCTest

nonisolated final class V3AppearanceAuditUITests: XCTestCase {
    private static let screens: [(name: String, arguments: [String])] = [
        ("login-phone", ["-MonacoWelcomeSample", "signInPhone"]),
        ("login-code", ["-MonacoWelcomeSample", "code"]),
        ("home-v3-loss", ["-MonacoHomeSample", "loss"]),
        ("balances-ready", ["-balancesHarness", "ready"]),
        ("activity-feed-unread", ["-activityFeedHarness", "unread"]),
        ("money-add", ["-MonacoMoneyFlowSample", "addMoney"]),
        ("money-card-handoff", ["-MonacoMoneyFlowSample", "cardFunding"]),
        ("cash-out-slice", ["-cashOutHarness", "slice"]),
        ("money-deposit-address", ["-MonacoMoneyFlowSample", "depositAddress"]),
        ("money-fund", ["-MonacoMoneyFlowSample", "fundCabal"]),
        ("money-networks", ["-MonacoMoneyFlowSample", "networks"]),
        ("money-withdraw", ["-MonacoMoneyFlowSample", "withdraw"]),
        ("account-activity-ready", ["-accountActivityHarness", "ready"]),
        ("group-cash-ready", ["-groupCashHarness", "ready"]),
        ("receipt-completed", ["-receiptHarness", "completed"]),
        ("propose-chooser", ["-MonacoProposeSample", "chooser"]),
        ("propose-review", ["-MonacoProposeSample", "review"]),
        ("propose-buy-list", ["-MonacoProposeSample", "buyList"]),
        ("propose-amount", ["-MonacoProposeSample", "amount"]),
        ("proposal-open", ["-MonacoProposalSample", "open"]),
        ("proposal-all-open", ["-MonacoProposalSample", "allOpen"]),
        ("proposal-votes-open", ["-MonacoProposalSample", "votesOpen"]),
        ("cabals-ready", ["-MonacoCabalsTabSample", "-MonacoCabalsTabSampleScenario", "ready"]),
        ("create-cabal-ready", ["-MonacoCreateCabalSample", "ready"]),
        ("discover-results", ["-MonacoDiscoverSample", "results"]),
        ("cabal-ready", ["-MonacoGroupDetailSample", "ready"]),
        ("cabal-info", ["-MonacoGroupDetailSample", "info"]),
        ("cabal-invite", ["-MonacoGroupDetailSample", "invite"]),
        ("cabal-members", ["-MonacoGroupDetailSample", "members"]),
        ("cabal-public-preview", ["-MonacoGroupDetailSample", "publicPreview"]),
        ("chat-thread", ["-MonacoChatSampleQA", "-MonacoChatSampleProposal"]),
        ("cabal-holding-ready", ["-MonacoCabalHoldingSample", "ready"]),
        ("asset-open", ["-MonacoAssetDetailSample", "open"]),
        ("stocks-tab-ready", ["-MonacoStocksTabSample", "ready"]),
        ("watchlist-ready", ["-MonacoWatchlistSample", "ready"]),
        ("feed-items", ["-feedHarness", "items"]),
        ("feed-post-ready", ["-feedPostHarness", "ready"]),
        ("leaderboard-cabals", ["-MonacoLeaderboardSample", "cabals"]),
        ("profile-edit-ready", ["-MonacoProfileSample", "editProfileReady"]),
        ("friends-results", ["-friendsHarness", "results"]),
        ("profile-person-public", ["-MonacoProfileSample", "personPublic"]),
        ("profile-ready", ["-MonacoProfileSample", "ready"]),
        ("settings", ["-settingsHarness"]),
    ]

    private enum Reason {
        static let inactive = "inactive control: disabled, placeholder or coming soon"
        static let chrome = "scrolled under the glass tab bar or dock, so the audit samples the blur"
        static let cover = "drawn on a cabal cover, glass sheet or tile, where the pair is held by MonacoContrastTests"
        static let token = "token pair held at AA in both schemes by MonacoContrastTests"
    }

    private static let allowedIssues: [(screen: String, label: String, reason: String)] = [
        ("login-phone", "Send code", Reason.inactive),
        ("login-code", "Continue", Reason.inactive),
        ("home-v3-loss", "Your cabals", Reason.chrome),
        ("home-v3-loss", "See all", Reason.token),
        ("home-v3-loss", "3 yes · 1 no · 4 not voted", Reason.token),
        ("cash-out-slice", "Review cash-out", Reason.inactive),
        ("cash-out-slice", "25%", Reason.inactive),
        ("cash-out-slice", "50%", Reason.inactive),
        ("cash-out-slice", "All", Reason.inactive),
        ("money-deposit-address", "3", Reason.cover),
        ("money-fund", "Enter an amount", Reason.inactive),
        ("money-fund", "$0", Reason.inactive),
        ("propose-chooser", "Cancel", Reason.chrome),
        ("propose-chooser", "Add a trading bot", Reason.inactive),
        ("propose-chooser", "Trading bots open soon.", Reason.inactive),
        ("propose-buy-list", "NEWCO", Reason.inactive),
        ("propose-buy-list", "Newco", Reason.inactive),
        ("propose-amount", "Review proposal", Reason.inactive),
        ("propose-amount", "Thesis", Reason.inactive),
        ("propose-amount", "$0", Reason.inactive),
        ("propose-amount", "$25", Reason.inactive),
        ("propose-amount", "$50", Reason.inactive),
        ("propose-amount", "$100", Reason.inactive),
        ("propose-amount", "Max", Reason.inactive),
        ("proposal-open", "Vote No", Reason.chrome),
        ("proposal-open", "Cabal gets", Reason.chrome),
        ("proposal-open", "Replies", Reason.chrome),
        ("proposal-open", "Maya", Reason.chrome),
        ("cabals-ready", "$1,300.00", Reason.cover),
        ("cabals-ready", "6.50%", Reason.cover),
        ("cabals-ready", "Your P&L · All time", Reason.cover),
        ("cabals-ready", "−$167.68", Reason.cover),
        ("cabals-ready", "−11.4%", Reason.cover),
        ("cabals-ready", "Cabal · 1M — Return opens soon", Reason.inactive),
        ("create-cabal-ready", "Passes once more than half the voters say yes.", Reason.token),
        ("cabal-ready", "Fund this cabal", Reason.chrome),
        ("cabal-ready", "Propose", Reason.chrome),
        ("cabal-ready", "Cash out", Reason.chrome),
        ("cabal-ready", "Open chat", Reason.chrome),
        ("cabal-info", "Do not send funds here. Transfers are returned.", Reason.token),
        ("cabal-invite", "Share a link with your friends.", Reason.token),
        ("cabal-invite", "Cabal links open soon", Reason.inactive),
        ("cabal-invite", "Copy link", Reason.inactive),
        ("cabal-invite", "Share your gain as an image", Reason.inactive),
        ("chat-thread", "L", Reason.cover),
        ("cabal-holding-ready", "+$117.86", Reason.token),
        ("asset-open", "178.02", Reason.cover),
        ("asset-open", "Market stats", Reason.chrome),
        ("asset-open", "Data provided by CoinGecko", Reason.chrome),
        ("stocks-tab-ready", "Popular now", Reason.chrome),
        ("stocks-tab-ready", "Most popular on Monaco", Reason.chrome),
        ("stocks-tab-ready", "NVDA", Reason.chrome),
        ("stocks-tab-ready", "NVIDIA", Reason.chrome),
        ("stocks-tab-ready", "$220.00", Reason.chrome),
        ("profile-edit-ready", "Save changes", Reason.inactive),
        ("profile-edit-ready", "LN", Reason.cover),
    ]

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = true
    }

    @MainActor
    private func audit(_ name: String) async throws {
        let arguments = try XCTUnwrap(Self.screens.first { $0.name == name }?.arguments, "\(name) is not listed")
        for appearance in ["Dark", "Light"] {
            let app = XCUIApplication()
            app.launchArguments = arguments + ["-AppleInterfaceStyle", appearance]
            app.launch()
            XCTAssertTrue(app.wait(for: .runningForeground, timeout: 30), "\(name) never reached the foreground")
            XCTAssertTrue(app.windows.firstMatch.waitForExistence(timeout: 30), "\(name) drew no window")
            try await Task.sleep(for: .seconds(3))
            try app.performAccessibilityAudit(for: .contrast) { issue in
                let label = issue.element?.label ?? ""
                let allowed = Self.allowedIssues.contains { $0.screen == name && $0.label == label }
                print("AUDIT \(name) \(appearance) allowed=\(allowed) label=\(label) | \(issue.detailedDescription)")
                return allowed
            }
            app.terminate()
        }
    }

    @MainActor
    func testTheListHoldsAllFortyThreeDefaults() {
        XCTAssertEqual(Self.screens.count, 43)
        XCTAssertEqual(Set(Self.screens.map(\.name)).count, 43)
    }

    @MainActor func test01SigninPhone() async throws { try await audit("login-phone") }
    @MainActor func test02VerifyReady() async throws { try await audit("login-code") }
    @MainActor func test03HomeLoss() async throws { try await audit("home-v3-loss") }
    @MainActor func test04BalancesReady() async throws { try await audit("balances-ready") }
    @MainActor func test05NotificationsUnread() async throws { try await audit("activity-feed-unread") }
    @MainActor func test06AddMoneyReady() async throws { try await audit("money-add") }
    @MainActor func test07CardFundingHandoff() async throws { try await audit("money-card-handoff") }
    @MainActor func test08CashOutReady() async throws { try await audit("cash-out-slice") }
    @MainActor func test09DepositAddress() async throws { try await audit("money-deposit-address") }
    @MainActor func test10FundCabalReady() async throws { try await audit("money-fund") }
    @MainActor func test11NetworkReady() async throws { try await audit("money-networks") }
    @MainActor func test12WithdrawReady() async throws { try await audit("money-withdraw") }
    @MainActor func test13ActivityReady() async throws { try await audit("account-activity-ready") }
    @MainActor func test14GroupCashReady() async throws { try await audit("group-cash-ready") }
    @MainActor func test15TransactionCompleted() async throws { try await audit("receipt-completed") }
    @MainActor func test16ProposeReady() async throws { try await audit("propose-chooser") }
    @MainActor func test17ReviewProposalReady() async throws { try await audit("propose-review") }
    @MainActor func test18SelectStockBuy() async throws { try await audit("propose-buy-list") }
    @MainActor func test19TradeAmountReady() async throws { try await audit("propose-amount") }
    @MainActor func test20ProposalOpen() async throws { try await audit("proposal-open") }
    @MainActor func test21ProposalsOpen() async throws { try await audit("proposal-all-open") }
    @MainActor func test22VotesOpen() async throws { try await audit("proposal-votes-open") }
    @MainActor func test23CabalsReady() async throws { try await audit("cabals-ready") }
    @MainActor func test24CreateCabalReady() async throws { try await audit("create-cabal-ready") }
    @MainActor func test25DiscoverResults() async throws { try await audit("discover-results") }
    @MainActor func test26CabalReady() async throws { try await audit("cabal-ready") }
    @MainActor func test27CabalInfoMember() async throws { try await audit("cabal-info") }
    @MainActor func test28InviteShare() async throws { try await audit("cabal-invite") }
    @MainActor func test29MembersMembers() async throws { try await audit("cabal-members") }
    @MainActor func test30PublicCabalPreview() async throws { try await audit("cabal-public-preview") }
    @MainActor func test31ChatReady() async throws { try await audit("chat-thread") }
    @MainActor func test32HoldingReady() async throws { try await audit("cabal-holding-ready") }
    @MainActor func test33InstrumentReady() async throws { try await audit("asset-open") }
    @MainActor func test34MarketsReady() async throws { try await audit("stocks-tab-ready") }
    @MainActor func test35WatchlistReady() async throws { try await audit("watchlist-ready") }
    @MainActor func test36FeedAll() async throws { try await audit("feed-items") }
    @MainActor func test37PostReady() async throws { try await audit("feed-post-ready") }
    @MainActor func test38LeaderboardCabals() async throws { try await audit("leaderboard-cabals") }
    @MainActor func test39EditProfileReady() async throws { try await audit("profile-edit-ready") }
    @MainActor func test40PeopleResults() async throws { try await audit("friends-results") }
    @MainActor func test41PersonPublic() async throws { try await audit("profile-person-public") }
    @MainActor func test42ProfileReady() async throws { try await audit("profile-ready") }
    @MainActor func test43SettingsReady() async throws { try await audit("settings") }
}
