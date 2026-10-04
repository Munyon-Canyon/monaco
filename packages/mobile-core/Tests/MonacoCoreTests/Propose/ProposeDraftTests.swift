import MonacoCore
import XCTest

final class ProposeDraftTests: XCTestCase {
    private let pot = ProposePot(id: "cabal", name: "Weekend", totalMicros: 1_000_000)

    func testBuyRequiresASymbolAndNonzeroAmountWithinThePot() {
        XCTAssertThrowsError(try ProposalDraft.buy(symbol: " ", usdcMicros: 1, thesis: "").validate(in: pot)) {
            XCTAssertEqual($0 as? ProposalDraft.ValidationError, .symbolRequired)
        }
        XCTAssertThrowsError(try ProposalDraft.buy(symbol: "AAPLx", usdcMicros: 0, thesis: "").validate(in: pot)) {
            XCTAssertEqual($0 as? ProposalDraft.ValidationError, .amountRequired)
        }
        XCTAssertThrowsError(
            try ProposalDraft.buy(symbol: "AAPLx", usdcMicros: 1_000_001, thesis: "").validate(in: pot)
        ) {
            XCTAssertEqual($0 as? ProposalDraft.ValidationError, .amountExceedsPot)
        }
        XCTAssertNoThrow(try ProposalDraft.buy(symbol: "AAPLx", usdcMicros: 1_000_000, thesis: "").validate(in: pot))
    }

    func testSellRequiresASymbolAndNonzeroTokenAmount() {
        XCTAssertThrowsError(try ProposalDraft.sell(symbol: "", tokenAmount: 1, thesis: "").validate(in: pot))
        XCTAssertThrowsError(try ProposalDraft.sell(symbol: "AAPLx", tokenAmount: 0, thesis: "").validate(in: pot))
        XCTAssertNoThrow(try ProposalDraft.sell(symbol: "AAPLx", tokenAmount: 1, thesis: "").validate(in: pot))
    }

    func testReasonCounterAndLimit() {
        XCTAssertFalse(ProposeReasonRules.showsCounter(for: String(repeating: "x", count: 179)))
        XCTAssertTrue(ProposeReasonRules.showsCounter(for: String(repeating: "x", count: 180)))
        XCTAssertEqual(ProposeReasonRules.limited(String(repeating: "x", count: 281)).count, 280)
    }
}
