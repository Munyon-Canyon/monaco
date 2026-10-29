import MonacoAPI
import XCTest

@testable import MonacoCore

final class HomeGateFirstUseTests: XCTestCase {
    private func summary(_ cabals: [Components.Schemas.PortfolioCabal], cashMicros: Int64 = 500_000_000)
        -> PortfolioSummary
    {
        var portfolio = Components.Schemas.MyPortfolio.sampleF
        portfolio.cashMicros = cashMicros
        portfolio.cabals = cabals
        return PortfolioSummary(portfolio)
    }

    private func stake(value: Int64) -> Components.Schemas.PortfolioCabal {
        Components.Schemas.PortfolioCabal(
            cabal: Components.Schemas.CabalRef(id: "0f1d6a52-0001-7000-8000-000000000001", name: "AC", pictureUrl: nil),
            valueMicros: value, shareUnits: value, netContributedMicros: value, pnlMicros: 0, returnBps: nil,
            sliceBps: 0, ownershipBps: 0, role: .member, canVote: true, potValueMicros: value, memberCount: 3,
            openVoteCount: 0)
    }

    func testNoMembershipsIsFirstUseEvenWithCash() {
        XCTAssertTrue(summary([]).isFirstUse)
    }

    func testAnInvitationAloneIsStillFirstUse() {
        XCTAssertTrue(PortfolioSummary(.sampleEmpty).isFirstUse)
    }

    func testAJoinedUnfundedCabalIsNotFirstUse() {
        XCTAssertFalse(summary([stake(value: 0)]).isFirstUse)
    }

    func testOneFundedCabalIsNotFirstUse() {
        XCTAssertFalse(summary([stake(value: 7_000_000_000)]).isFirstUse)
    }
}
