import Foundation
import MonacoAPI
import MonacoCore
import Testing

@testable import Monaco

@MainActor
struct HomeCabalCardTests {
    private func memberships(_ ids: [String]) -> [Components.Schemas.MyCabal] {
        ids.enumerated().map { index, id in
            Components.Schemas.MyCabal(
                id: id, name: "Cabal \(index)", pictureUrl: nil, role: "member", canVote: true, memberCount: 8,
                joinedAt: Date(timeIntervalSince1970: 1_790_000_000 + TimeInterval(index)), pendingRequestCount: 0,
                unreadCount: 0)
        }
    }

    private func items(_ portfolio: Components.Schemas.MyPortfolio, _ ids: [String]) -> [HomeCabalItem] {
        HomeCabalList.items(portfolio: PortfolioSummary(portfolio), memberships: memberships(ids))
    }

    @Test func aFundedCardKeepsItsReturnAndShare() {
        let alpha = Components.Schemas.CabalRef.sampleAlpha.id
        let item = items(.sample, [alpha])[0]
        let model = CabalCoverCardModel(item: item, context: nil, votesNeededText: "1 vote needed")

        #expect(model.pnl != nil)
        #expect(model.pnlUnavailableReason == nil)
        #expect(model.shareText == "$600.00")
        #expect(model.votesNeededText == "1 vote needed")
        #expect(model.memberCountText == "8 members")
    }

    @Test func anUnfundedCardSaysNotFundedWithAZeroShare() {
        let item = items(.sampleEmpty, ["01890a5d-ac96-774b-bcce-b302099a805a"])[0]
        let model = CabalCoverCardModel(item: item, context: nil, votesNeededText: "1 vote needed")

        #expect(model.pnl == nil)
        #expect(model.pnlUnavailableReason == "Not funded")
        #expect(model.shareText == "$0.00")
        #expect(model.ownershipText == "0.00%")
        #expect(model.votesNeededText == nil)
    }

    @Test func aCashedOutCardIsUnfundedToo() throws {
        let beta = Components.Schemas.CabalRef.sampleBeta.id
        let item = try #require(items(.sampleLoss, [beta]).first { $0.id == beta })
        let model = CabalCoverCardModel(item: item, context: nil, votesNeededText: nil)

        #expect(model.pnl == nil)
        #expect(model.pnlUnavailableReason == "Not funded")
    }
}
