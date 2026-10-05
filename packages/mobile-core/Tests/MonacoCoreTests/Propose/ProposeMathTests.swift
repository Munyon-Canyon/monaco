import MonacoCore
import XCTest

final class ProposeMathTests: XCTestCase {
    func testSellTokenAmountFloorsAndCapsAtTheHolding() {
        XCTAssertEqual(ProposeMath.tokenAmount(units: 101, dollars: 25, holdingValue: 100), 25)
        XCTAssertEqual(ProposeMath.tokenAmount(units: 101, dollars: 50, holdingValue: 100), 50)
        XCTAssertEqual(ProposeMath.tokenAmount(units: 101, dollars: 100, holdingValue: 100), 101)
    }

    func testQuantityLabelReflectsAssetKind() {
        XCTAssertEqual(ProposeMath.quantityLabel(kind: .stock), "shares")
        XCTAssertEqual(ProposeMath.quantityLabel(kind: .preIpo), "tokens")
    }
}
