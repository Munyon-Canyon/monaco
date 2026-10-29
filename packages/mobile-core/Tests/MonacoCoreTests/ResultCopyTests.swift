import XCTest

@testable import MonacoCore

final class ResultCopyTests: XCTestCase {
    func testCalledItNamesTheProposer() {
        XCTAssertEqual(ResultCopy.calledIt("Christelle"), "Christelle called it")
    }
}
