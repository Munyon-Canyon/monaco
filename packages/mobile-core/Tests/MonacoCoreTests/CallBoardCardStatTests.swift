import MonacoRanking
import XCTest

final class CallBoardCardStatTests: XCTestCase {
    func testCardStatDropsTheRightSuffix() {
        XCTAssertEqual(CallBoardCopy.cardStat(rate: "78%", right: 7, resolved: 9), "78% · 7 of 9")
    }
}
