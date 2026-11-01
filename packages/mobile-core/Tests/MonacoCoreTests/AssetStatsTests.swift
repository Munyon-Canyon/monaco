import XCTest

@testable import MonacoCore

final class AssetStatsTests: XCTestCase {
    private let nvidia = AssetStats(
        marketCapMicros: "5380000000000000000", volume24hMicros: 9_400_000_000_000, openMicros: 211_170_000,
        yearLowMicros: 86_620_000, yearHighMicros: 224_900_000, heldByCabals: 1)

    func testSixCellsInTwoRowsOfThreeAsTheFrameDrawsThem() {
        XCTAssertEqual(
            nvidia.topRow.map { "\($0.label) \($0.value)" },
            ["Market cap $5.38T", "Volume · 24h $9.4M", "Open $211.17"])
        XCTAssertEqual(
            nvidia.bottomRow.map { "\($0.label) \($0.value)" },
            ["52-week low $86.62", "52-week high $224.90", "Held by 1 cabal"])
    }

    func testANullCellIsADashThatReadsNotAvailableAndNoCellIsHidden() {
        let empty = AssetStats(
            marketCapMicros: nil, volume24hMicros: nil, openMicros: nil, yearLowMicros: nil, yearHighMicros: nil,
            heldByCabals: 0)

        XCTAssertEqual(empty.cells.count, 6)
        XCTAssertEqual(empty.cells.map(\.value), Array(repeating: "—", count: 6))
        XCTAssertEqual(empty.cells.map(\.spokenValue), Array(repeating: "not available", count: 6))
        XCTAssertEqual(empty.cells.map(\.label), nvidia.cells.map(\.label))
    }

    func testHeldByCountsCabals() {
        func held(_ count: Int?) -> String {
            AssetStats(
                marketCapMicros: nil, volume24hMicros: nil, openMicros: nil, yearLowMicros: nil, yearHighMicros: nil,
                heldByCabals: count
            ).bottomRow[2].value
        }
        XCTAssertEqual(held(2), "2 cabals")
        XCTAssertEqual(held(12), "12 cabals")
        XCTAssertEqual(held(nil), "—")
    }

    func testMarketCapAbbreviatesToTwoDecimalsAndDropsTrailingZeros() {
        XCTAssertEqual(AssetStats.abbreviated("5380000000000000000"), "$5.38T")
        XCTAssertEqual(AssetStats.abbreviated("5000000000000000000"), "$5T")
        XCTAssertEqual(AssetStats.abbreviated("350000000000000000"), "$350B")
        XCTAssertEqual(AssetStats.abbreviated("12340000000000"), "$12.34M")
        XCTAssertEqual(AssetStats.abbreviated("9990000000000000000000"), "$9990T")
        XCTAssertNil(AssetStats.abbreviated("n/a"))
    }

    func testTodayMoveTruncatesToTheCentSoItNeverOverstatesTheMove() throws {
        let move = try XCTUnwrap(AssetTodayMove(priceMicros: 220_000_000, basisPoints: 418))

        XCTAssertEqual(move.changeMicros, 8_820_000)
        XCTAssertEqual(move.line, "+$8.82 · 4.18% today")
        XCTAssertFalse(move.isLoss)
    }

    func testTodayMoveForALossHasNoPercentSign() throws {
        let move = try XCTUnwrap(AssetTodayMove(priceMicros: 180_000_000, basisPoints: -80))

        XCTAssertEqual(move.line, "−$1.45 · 0.80% today")
        XCTAssertTrue(move.isLoss)
    }

    func testTodayMoveNeedsAPriceAndAChange() {
        XCTAssertNil(AssetTodayMove(priceMicros: nil, basisPoints: 100))
        XCTAssertNil(AssetTodayMove(priceMicros: 100_000_000, basisPoints: nil))
        XCTAssertNil(AssetTodayMove(priceMicros: 100_000_000, basisPoints: -10_000))
    }

    func testAfterHoursMoveReadsAsItsOwnSignedLine() throws {
        let gain = try XCTUnwrap(AssetAfterHoursMove(basisPoints: 20))
        let loss = try XCTUnwrap(AssetAfterHoursMove(basisPoints: -50))

        XCTAssertEqual(gain.line, "After hours +0.2%")
        XCTAssertFalse(gain.isLoss)
        XCTAssertEqual(gain.speech, "after hours up 0.2 percent")
        XCTAssertEqual(loss.line, "After hours −0.5%")
        XCTAssertTrue(loss.isLoss)
        XCTAssertEqual(loss.speech, "after hours down 0.5 percent")
    }

    func testAfterHoursMoveNeedsAChange() {
        XCTAssertNil(AssetAfterHoursMove(basisPoints: nil))
        XCTAssertNil(AssetAfterHoursMove(basisPoints: -10_000))
    }

    func testTheHeaderReadsForVoiceOver() throws {
        let move = try XCTUnwrap(AssetTodayMove(priceMicros: 220_000_000, basisPoints: 418))

        XCTAssertEqual(move.speech(priceMicros: 220_000_000), "220 dollars, up 8 dollars 82, 4.18 percent today")
    }

    func testPositionReturnIsPnlOverCostBasis() {
        let position = AssetCabalPosition(
            cabalID: "c", cabalName: "N", pictureURL: nil, canVote: true, units: "1.0000",
            kind: .stock, valueMicros: 120_000_000, pnlMicros: 20_000_000, costBasisMicros: 100_000_000)
        XCTAssertEqual(position.returnBasisPoints, 2000)
    }

    func testPositionLabelFollowsTheAssetKind() {
        func label(_ kind: AssetKind) -> String {
            AssetCabalPosition(
                cabalID: "c", cabalName: "N", pictureURL: nil, canVote: true, units: "2.5000", kind: kind,
                valueMicros: 0, pnlMicros: 0, costBasisMicros: 0
            ).sharesLabel
        }
        XCTAssertEqual(label(.stock), "2.5 shares")
        XCTAssertEqual(label(.preIpo), "2.5 tokens")
    }
}

extension AssetStatsTests {
}
