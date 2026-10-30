import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct ProfileIdentityTests {
    private func summary(_ portfolio: Components.Schemas.MyPortfolio) -> PortfolioSummary {
        PortfolioSummary(portfolio)
    }

    @Test func theFullScreenErrorNeedsAFailedReadAndNoCardToKeep() {
        #expect(!ProfileScreen.showsLoadFailure(model: nil))
    }

    @Test func theCashRowShowsTheAvailableFigure() throws {
        let balance = try AccountBalance(Components.Schemas.Balance.sample)
        #expect(ProfileCashSection.cash(.loaded(balance)) == .amount(balance.availableMicros))
        #expect(ProfileCashSection.cash(.idle) == .loading)
        #expect(ProfileCashSection.cash(.loading) == .loading)
        #expect(ProfileCashSection.cash(.failed(.signedOut)) == .unavailable)
    }

    @Test func theCashRowSpeaksEachFigure() {
        #expect(ProfileCashRow.spoken(.amount(500_000_000)) == "500 dollars")
        #expect(ProfileCashRow.spoken(.loading) == "loading")
        #expect(ProfileCashRow.spoken(.unavailable) == "unavailable")
    }

    @Test func theCashRowHidesTheApyAtLaunch() {
        #expect(ProfileCopy.cashLabel == "Cash \u{00B7} USDC")
        #expect(!LaunchFeatures.apy)
        #expect(ProfileCopy.apyLabel == "3.5% APY")
        #expect(ProfileCopy.apyInfo == CabalYield.fundNote)
    }

    @Test func theRowsFollowTheDesignOrder() {
        let names = ProfileScreen.sections.map { String(describing: $0) }
        #expect(!names.contains("ProfileStatsSlot"))
        #expect(names.first == "ProfileCardSlot")
        #expect(names == ["ProfileCardSlot", "ProfileStandingSlot", "ProfileCashSlot", "ProfileTabsSlot"])
    }
}
