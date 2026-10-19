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

    func testTheMemberCountReadsAsWords() {
        XCTAssertEqual(HomeCabalContext(memberCount: 8, ownershipBps: nil).memberCountText, "8 members")
        XCTAssertEqual(HomeCabalContext(memberCount: 1, ownershipBps: nil).memberCountText, "1 member")
        XCTAssertEqual(HomeCabalContext(memberCount: nil, ownershipBps: nil).memberCountText, "Cabal")
    }

    func testOwnershipShowsTwoDecimals() {
        XCTAssertEqual(HomeCabalContext(memberCount: 3, ownershipBps: 3_500).ownershipText, "35.00%")
        XCTAssertEqual(HomeCabalContext(memberCount: 3, ownershipBps: 0).ownershipText, "0.00%")
        XCTAssertNil(HomeCabalContext(memberCount: 3, ownershipBps: nil).ownershipText)
    }
}
