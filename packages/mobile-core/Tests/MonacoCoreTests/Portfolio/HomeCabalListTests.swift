import XCTest

@testable import MonacoAPI
@testable import MonacoCore

final class HomeCabalListTests: XCTestCase {
    private let alpha = Components.Schemas.CabalRef.sampleAlpha.id
    private let beta = Components.Schemas.CabalRef.sampleBeta.id
    private let gamma = "01890a5d-ac96-774b-bcce-b302099a805a"
    private let delta = "01890a5d-ac96-774b-bcce-b302099a805b"

    private func cabal(_ id: String, name: String, joined: TimeInterval) -> Components.Schemas.MyCabal {
        Components.Schemas.MyCabal(
            id: id, name: name, pictureUrl: nil, role: "member", canVote: true, memberCount: 4,
            joinedAt: Date(timeIntervalSince1970: joined), pendingRequestCount: 0, unreadCount: 0)
    }

    private func state(_ portfolio: Components.Schemas.MyPortfolio, _ memberships: [Components.Schemas.MyCabal]?)
        -> HomeCabalsState
    {
        HomeCabalList.state(portfolio: PortfolioSummary(portfolio), memberships: memberships)
    }

    private func items(_ state: HomeCabalsState) -> [HomeCabalItem] {
        guard case .cabals(let items) = state else {
            XCTFail("expected cabals, got \(state)")
            return []
        }
        return items
    }

    func testFundedCabalsKeepTheirPortfolioOrderAndFigures() {
        let shown = items(
            state(.sample, [cabal(beta, name: "Sunday Investors", joined: 2), cabal(alpha, name: "Alpha", joined: 1)]))

        XCTAssertEqual(shown.map(\.id), [alpha, beta])
        XCTAssertTrue(shown.allSatisfy(\.isFunded))
        XCTAssertEqual(shown.first?.standing?.valueMicros, 600_000_000)
        XCTAssertEqual(shown.first?.memberCount, 4)
    }

    func testAnUnfundedMemberCabalFollowsTheFundedOnes() {
        let shown = items(
            state(.sampleLoss, [cabal(gamma, name: "Gamma", joined: 5), cabal(alpha, name: "Alpha", joined: 1)]))

        XCTAssertEqual(shown.map(\.id), [alpha, gamma])
        XCTAssertEqual(shown.map(\.isFunded), [true, false])
        XCTAssertNil(shown.last?.standing)
        XCTAssertEqual(shown.last?.name, "Gamma")
    }

    func testAMemberWhoFundedNoneStillSeesEveryCabalNewestJoinFirst() {
        let shown = items(
            state(
                .sampleEmpty,
                [
                    cabal(gamma, name: "Gamma", joined: 10), cabal(delta, name: "Delta", joined: 30),
                    cabal(alpha, name: "Alpha", joined: 20),
                ]))

        XCTAssertEqual(shown.map(\.id), [delta, alpha, gamma])
        XCTAssertTrue(shown.allSatisfy { !$0.isFunded })
    }

    func testAFullyCashedOutMemberCabalIsUnfundedAndKeepsItsRow() {
        let shown = items(
            state(
                .sampleLoss, [cabal(alpha, name: "Alpha", joined: 1), cabal(beta, name: "Sunday Investors", joined: 2)])
        )

        XCTAssertEqual(shown.map(\.id), [alpha, beta])
        XCTAssertEqual(shown.map(\.isFunded), [true, false])
        XCTAssertEqual(shown.last?.standing?.valueMicros, 0)
    }

    func testACashedOutCabalYouLeftIsDropped() {
        let shown = items(state(.sampleLoss, [cabal(alpha, name: "Alpha", joined: 1)]))

        XCTAssertEqual(shown.map(\.id), [alpha])
    }

    func testNoMembershipsAndNoPortfolioIsTheEmptyState() {
        XCTAssertEqual(state(.sampleEmpty, []), .empty)
    }

    func testAFailedMembershipsReadFallsBackToThePortfolioRows() {
        let shown = items(state(.sample, nil))

        XCTAssertEqual(shown.map(\.id), [alpha, beta])
        XCTAssertNil(shown.first?.memberCount)
    }

    func testAFailedMembershipsReadWithNoPortfolioRowsNeverShowsTheEmptyState() {
        XCTAssertEqual(state(.sampleEmpty, nil), .loading)
    }
}
