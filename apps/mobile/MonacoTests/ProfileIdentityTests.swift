import MonacoAPI
import MonacoCore
import SwiftUI
import Testing

@testable import Monaco

@MainActor
struct ProfileIdentityTests {
    private func summary(_ portfolio: Components.Schemas.MyPortfolio) -> PortfolioSummary {
        PortfolioSummary(portfolio)
    }

    private func profile() -> SessionProfile {
        ProfileSampleHarness.sampleProfile(
            userID: "u", displayName: " Alex ", photoURL: nil,
            createdAt: ISO8601DateFormatter().date(from: "2026-10-01T14:30:00Z"))
    }

    @Test func aNewProfileDrawsZerosAsDesigned() {
        let stats = summary(.sampleEmpty).stats
        #expect(IdentityPerformance(stats: stats) == .empty)
        #expect(IdentityPerformance.empty == .figure(dollars: "+$0.00", percent: "0.00%"))
    }

    @Test func aLossKeepsItsSignAndTwoDecimals() {
        let stats = summary(.sampleLoss).stats
        #expect(IdentityPerformance(stats: stats) == .figure(dollars: "−$1.00", percent: "−0.11%"))
    }

    @Test func theCardNamesTheMemberAndTheAllTimePeriod() {
        let model = ProfileHeader.model(for: profile(), portfolio: .loaded(summary(.sample)), bio: nil)
        #expect(model.displayName == "Alex")
        #expect(model.periodText == "All time")
        #expect(model.positionsValue == "$1,000.00")
        #expect(model.joinedText?.hasPrefix("Joined ") == true)
        #expect(model.bio == nil)
    }

    @Test func theCardShowsLoadingAndFailureWithoutInventingFigures() {
        let loading = ProfileHeader.model(for: profile(), portfolio: .loading, bio: nil)
        #expect(loading.performance == .loading)
        let failed = ProfileHeader.model(for: profile(), portfolio: .failed(.signedOut), bio: "Hi")
        #expect(failed.performance == .failed)
        #expect(failed.bio == "Hi")
    }

    @Test func theFullScreenErrorNeedsNoProfileAndNoLoadInFlight() {
        #expect(ProfileScreen.showsLoadFailure(profile: nil, isLoading: false))
        #expect(!ProfileScreen.showsLoadFailure(profile: nil, isLoading: true))
        #expect(!ProfileScreen.showsLoadFailure(profile: profile(), isLoading: false))
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

    @Test func theCoverCardTakesTheRowFigures() throws {
        let row = try #require(summary(.sample).rows.first)
        let model = CabalCoverCardModel(profileRow: row)
        #expect(model.shareText == "$600.00")
        #expect(model.ownershipText == row.slice)
        #expect(model.pnl?.direction == .profit)
        #expect(model.groupReturn == nil)
        let unfunded = try #require(summary(.sampleLoss).rows.last)
        #expect(CabalCoverCardModel(profileRow: unfunded).pnl == nil)
    }

    @Test func theRowsFollowTheDesignOrder() {
        let names = ProfileScreen.sections.map { String(describing: $0) }
        #expect(!names.contains("ProfileStatsSlot"))
        #expect(names.first == "ProfileHeaderSlot")
        #expect(names.last == "ProfileLinksSlot")
    }
}
