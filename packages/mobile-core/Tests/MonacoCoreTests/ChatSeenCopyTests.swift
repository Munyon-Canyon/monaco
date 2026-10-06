import MonacoCore
import XCTest

final class ChatSeenCopyTests: XCTestCase {
    func testLabelNamesTheCountAndHidesAtZero() {
        XCTAssertNil(ChatSeenCopy.label(count: 0))
        XCTAssertEqual(ChatSeenCopy.label(count: 1), "Seen by 1")
        XCTAssertEqual(ChatSeenCopy.label(count: 3), "Seen by 3")
    }
}
