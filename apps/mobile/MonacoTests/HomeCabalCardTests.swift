import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct HomeCabalCardTests {
    private func row(value: Int64, ownershipBps: Int32, members: Int32) -> HomeCabalRow {
        let ref = Components.Schemas.CabalRef.sampleACCapital
        let stake = Components.Schemas.PortfolioCabal(
            cabal: ref, valueMicros: value, shareUnits: value, netContributedMicros: value, pnlMicros: 0,
            returnBps: value == 0 ? nil : 0, sliceBps: 0, ownershipBps: ownershipBps, role: .member, canVote: true,
            potValueMicros: 0, memberCount: members, openVoteCount: 0)
        let portfolio = Components.Schemas.MyPortfolio(
            cashMicros: 0, netWorthMicros: value, totalValueMicros: value, pnlMicros: 0, returnBps: nil,
            computedAt: nil, pricesAsOf: nil, cabals: [stake])
        guard case .cabals(let content) = HomeCabalList.state(portfolio: PortfolioSummary(portfolio), invites: [])
        else { return fallback }
        return content.cabals[0]
    }

    private var fallback: HomeCabalRow {
        row(value: 0, ownershipBps: 0, members: 0)
    }

    @Test func aFundedRowReadsItsVotesThenYourShare() {
        let funded = row(value: 7_000_000_000, ownershipBps: 3_500, members: 8)

        #expect(funded.isFunded)
        #expect(funded.subtitle(votesText: "1 vote open") == "1 vote open · 35% yours")
        #expect(funded.spoken(votesText: "1 vote open") == "1 vote open, 35% yours")
    }

    @Test func anUnfundedRowReadsMembersAndZeroPercentWithNoReturn() {
        let unfunded = row(value: 0, ownershipBps: 0, members: 4)

        #expect(!unfunded.isFunded)
        #expect(unfunded.returnBps == nil)
        #expect(unfunded.subtitle(votesText: nil) == "4 members · 0% yours")
    }

    @Test func theCrestBadgeCountsTheOpenVotesAndHidesAtZero() {
        #expect(CrestMetrics.badgeText(1) == "1")
        #expect(CrestMetrics.badgeText(0) == nil)
    }
}
