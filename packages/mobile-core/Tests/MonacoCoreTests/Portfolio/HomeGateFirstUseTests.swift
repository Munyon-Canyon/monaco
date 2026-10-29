import MonacoAPI
import XCTest

@testable import MonacoCore

@MainActor
final class HomeGateFirstUseTests: XCTestCase {
    private func summary(cash: Int64, cabals: [Components.Schemas.PortfolioCabal]) -> PortfolioSummary {
        var portfolio = Components.Schemas.MyPortfolio.sampleEmpty
        portfolio.cashMicros = cash
        portfolio.netWorthMicros = cash
        portfolio.cabals = cabals
        return PortfolioSummary(portfolio)
    }

    private var unfundedCabal: Components.Schemas.PortfolioCabal {
        Components.Schemas.MyPortfolio.sampleWithUnfunded.cabals[3]
    }

    private var fundedCabal: Components.Schemas.PortfolioCabal {
        Components.Schemas.MyPortfolio.sample.cabals[0]
    }

    func testNoMembershipIsFirstUse() async throws {
        let none: PortfolioSummary = summary(cash: 500_000_000, cabals: [])

        XCTAssertTrue(HomeFirstUse.applies(to: none))
    }

    func testAnUnloadedPortfolioIsNotFirstUse() async throws {
        XCTAssertFalse(HomeFirstUse.applies(to: nil))
    }

    func testAnInvitationAloneIsStillFirstUseAndTheInviteRowShows() async throws {
        let none: PortfolioSummary = summary(cash: 0, cabals: [])
        let invite = HomeInviteRow(
            Components.Schemas.CabalInvite(
                requestId: "01890a5d-ac96-774b-bcce-b302099a8100",
                cabal: .init(
                    id: "01890a5d-ac96-774b-bcce-b302099a8101", name: "Quant Kids", pictureUrl: nil, memberCount: 5),
                invitedBy: .init(
                    userId: "01890a5d-ac96-774b-bcce-b302099a8102", handle: nil, displayName: "Sam", photoUrl: nil),
                expiresAt: Date(timeIntervalSince1970: 1_791_590_400)))

        XCTAssertTrue(HomeFirstUse.applies(to: none))
        guard case .cabals(let content) = HomeCabalList.state(portfolio: none, invites: [invite]) else {
            return XCTFail("the invited row should show")
        }
        XCTAssertEqual(content.invites.map(\.name), ["Quant Kids"])
        XCTAssertTrue(content.cabals.isEmpty)
    }

    func testAJoinedUnfundedCabalIsNotFirstUse() async throws {
        let joined: PortfolioSummary = summary(cash: 500_000_000, cabals: [unfundedCabal])

        XCTAssertFalse(HomeFirstUse.applies(to: joined))
    }

    func testOneFundedCabalIsNotFirstUse() async throws {
        let funded: PortfolioSummary = summary(cash: 0, cabals: [fundedCabal])

        XCTAssertFalse(HomeFirstUse.applies(to: funded))
    }

    func testFirstUseSwapsTheVoteChartAndListForTheEmptyStateAndEveryone() async throws {
        let none: PortfolioSummary = summary(cash: 500_000_000, cabals: [])
        let shown: [HomeBlock] = HomeBlock.allCases.filter { HomeFirstUse.shows($0, summary: none) }

        XCTAssertEqual(shown, [.firstUse, .everyoneFeed])
    }

    func testAMemberKeepsTheVoteChartAndList() async throws {
        let funded: PortfolioSummary = summary(cash: 0, cabals: [fundedCabal])
        let shown: [HomeBlock] = HomeBlock.allCases.filter { HomeFirstUse.shows($0, summary: funded) }

        XCTAssertEqual(shown, [.vote, .chart, .cabalsList, .cabalsFeed])
    }

    func testCashOnlyFigureReadsAllCash() async throws {
        let none: PortfolioSummary = summary(cash: 500_000_000, cabals: [])
        let figure: HomeFigureCopy.Figure = HomeFigureCopy.netWorth(micros: none.cash)

        XCTAssertEqual(figure.whole + figure.cents, "$500.00")
        XCTAssertEqual(HomeFigureCopy.allCash, "All cash")
    }
}
