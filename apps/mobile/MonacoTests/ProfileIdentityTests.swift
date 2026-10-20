import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

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
        let model = ProfileHeader.model(for: profile(), portfolio: .loaded(summary(.sample)))
        #expect(model.displayName == "Alex")
        #expect(model.periodText == "All time")
        #expect(model.positionsValue == "$1,000.00")
        #expect(model.joinedText?.hasPrefix("Joined ") == true)
        #expect(model.bio == nil)
    }

    @Test func theCardShowsLoadingAndFailureWithoutInventingFigures() {
        let loading = ProfileHeader.model(for: profile(), portfolio: .loading)
        #expect(loading.performance == .loading)
        var withBio = profile()
        withBio.bio = "Hi"
        let failed = ProfileHeader.model(for: withBio, portfolio: .failed(.signedOut))
        #expect(failed.performance == .failed)
        #expect(failed.bio == "Hi")
    }

    @Test func theOwnCardShowsTheRealBioAndTheOneMonthFigure() {
        var member = profile()
        member.bio = "Building in public.\nNYC"
        let model = ProfileHeader.model(
            for: member, portfolio: .loaded(summary(.sample)),
            oneMonth: .loaded(.figure(pnlMicros: -417_680_000, returnBps: -348)))
        #expect(model.bio == "Building in public.\nNYC")
        #expect(model.periodText == "1M")
        #expect(model.performance == .figure(dollars: "−$417.68", percent: "−3.48%"))
        #expect(model.positionsValue == "$1,000.00")
        #expect(model.positionsPhase == .loaded)
    }

    @Test func theOwnCardKeepsPositionsWhenOnlyTheOneMonthFigureFails() {
        let model = ProfileHeader.model(for: profile(), portfolio: .loaded(summary(.sample)), oneMonth: .failed)
        #expect(model.performance == .failed)
        #expect(model.positionsPhase == .loaded)
        let waiting = ProfileHeader.model(for: profile(), portfolio: .loaded(summary(.sample)), oneMonth: .loading)
        #expect(waiting.performance == .loading)
        #expect(waiting.positionsValue == "$1,000.00")
    }

    @Test func aPublicReturnDrawsAFigureThePrivateStateOrNoReturn() {
        #expect(
            IdentityPerformance(.figure(pnlMicros: 984_000_000, returnBps: 820))
                == .figure(dollars: "+$984.00", percent: "+8.20%"))
        #expect(IdentityPerformance(.hidden) == .hidden)
        #expect(IdentityPerformance(.unavailable) == .notRanked)
    }

    private func cardHeight(_ performance: IdentityPerformance, positions: String?) -> CGFloat {
        let model = IdentityCardModel(
            displayName: "Alex", positionsValue: positions, performance: performance, periodText: "1M")
        let card = IdentityCard(model: model) {
            Color.clear.frame(width: 64, height: 64)
        } share: {
            EmptyView()
        }
        return UIHostingController(rootView: card)
            .sizeThatFits(in: CGSize(width: 358, height: CGFloat.greatestFiniteMagnitude)).height
    }

    @Test func theOneMonthDollarsAndPercentShareALine() {
        let figure = IdentityPerformance.figure(dollars: "+$1,234.56", percent: "+12.30%")
        for positions in [nil, "$12,345.67"] {
            #expect(cardHeight(figure, positions: positions) == cardHeight(.hidden, positions: positions))
        }
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
