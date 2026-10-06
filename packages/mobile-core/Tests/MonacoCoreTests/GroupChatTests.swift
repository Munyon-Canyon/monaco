import Foundation
import XCTest

@testable import MonacoCore

final class GroupChatDatesTests: XCTestCase {
    func testCreatedAtDate_plainSecondsAndGarbage() {
        XCTAssertEqual(GroupChatDates.parse("2026-09-18T15:04:05Z")?.timeIntervalSince1970, 1_789_743_845)
        XCTAssertEqual(
            GroupChatDates.parse("2026-09-18T15:04:05.123456Z")?.timeIntervalSince1970 ?? 0, 1_789_743_845.123,
            accuracy: 0.001)
        XCTAssertNil(GroupChatDates.parse("yesterday"))
    }
}

final class GroupChatDraftTests: XCTestCase {
    func testValidate_trimsWhitespace() {
        XCTAssertEqual(try GroupChatDraft.validate("  hi cabal \n").get(), "hi cabal")
    }

    func testValidate_blank_isEmpty() {
        XCTAssertEqual(GroupChatDraft.validate(" \n\t "), .failure(.empty))
    }

    func testValidate_countsCharactersNotBytes() {
        XCTAssertNoThrow(try GroupChatDraft.validate(String(repeating: "é", count: 2000)).get())
        XCTAssertEqual(
            GroupChatDraft.validate(String(repeating: "a", count: 2001)),
            .failure(.tooLong(count: 2001))
        )
    }
}
