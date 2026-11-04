import XCTest

@testable import MonacoCore

final class HomeVariantTests: XCTestCase {
    func testNoCabalsAndNoCashIsEmpty() {
        let variant = HomeVariant(summary: PortfolioSummary(.sampleEmpty), cashMicros: 0)

        XCTAssertEqual(variant, .empty)
        XCTAssertFalse(variant.showsMoneyStrip)
        XCTAssertFalse(variant.showsProposals)
    }

    func testCashWithNoCabalsIsCashOnly() {
        let variant = HomeVariant(summary: PortfolioSummary(.sampleEmpty), cashMicros: 500_000_000)

        XCTAssertEqual(variant, .cashOnly)
        XCTAssertTrue(variant.showsMoneyStrip)
        XCTAssertFalse(variant.showsProposals)
    }

    func testCabalValueIsInvested() {
        let variant = HomeVariant(summary: PortfolioSummary(.sample), cashMicros: 0)

        XCTAssertEqual(variant, .invested)
        XCTAssertTrue(variant.showsMoneyStrip)
        XCTAssertTrue(variant.showsProposals)
    }

    func testCashOnlyWithAnOpenVoteShowsProposals() {
        let variant = HomeVariant(summary: PortfolioSummary(.sampleEmpty), cashMicros: 500_000_000)

        XCTAssertEqual(variant, .cashOnly)
        XCTAssertTrue(variant.showsProposals(openVotes: 1))
    }

    func testEmptyWithAnOpenVoteShowsProposals() {
        let variant = HomeVariant(summary: PortfolioSummary(.sampleEmpty), cashMicros: 0)

        XCTAssertEqual(variant, .empty)
        XCTAssertTrue(variant.showsProposals(openVotes: 1))
    }

    func testEmptyAndCashOnlyWithoutOpenVotesHideProposals() {
        let empty = HomeVariant(summary: PortfolioSummary(.sampleEmpty), cashMicros: 0)
        let cashOnly = HomeVariant(summary: PortfolioSummary(.sampleEmpty), cashMicros: 500_000_000)

        XCTAssertFalse(empty.showsProposals(openVotes: 0))
        XCTAssertFalse(cashOnly.showsProposals(openVotes: 0))
    }

    func testInvestedShowsProposalsWithoutOpenVotes() {
        let variant = HomeVariant(summary: PortfolioSummary(.sample), cashMicros: 0)

        XCTAssertTrue(variant.showsProposals(openVotes: 0))
    }
}
