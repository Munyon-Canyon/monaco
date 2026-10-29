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

    @Test func netWorthWaitsForBothCashAndThePortfolio() {
        let loaded = LoadState.loaded(summary(.sample))
        let withCash = ProfileMoneyStripSection.netWorth(cash: .amount(500_000_000), portfolio: loaded)
        #expect(withCash == .amount(1_500_000_000))
        #expect(ProfileMoneyStripSection.netWorth(cash: .loading, portfolio: loaded) == .loading)
        #expect(ProfileMoneyStripSection.netWorth(cash: .amount(1), portfolio: .loading) == .loading)
        #expect(ProfileMoneyStripSection.netWorth(cash: .unavailable, portfolio: loaded) == .unavailable)
        #expect(ProfileMoneyStripSection.netWorth(cash: .amount(1), portfolio: .failed(.signedOut)) == .unavailable)
    }

    @Test func theStripSpeaksEachFigure() {
        #expect(ProfileMoneyStrip.spoken(.amount(500_000_000)) == "$500.00")
        #expect(ProfileMoneyStrip.spoken(.loading) == "loading")
        #expect(ProfileMoneyStrip.spoken(.unavailable) == "unavailable")
    }

    @Test func theRowsFollowTheDesignOrder() {
        let names = ProfileScreen.sections.map { String(describing: $0) }
        #expect(!names.contains("ProfileStatsSlot"))
        #expect(names.first == "ProfileCardSlot")
        #expect(names.last == "ProfileLinksSlot")
    }
}
