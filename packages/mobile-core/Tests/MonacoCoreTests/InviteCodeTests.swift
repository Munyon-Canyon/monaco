import MonacoCore
import XCTest

final class InviteCodeTests: XCTestCase {
    func testValidCodeIsKept() {
        XCTAssertEqual(InviteCode("ABCD2345XY")?.value, "ABCD2345XY")
    }

    func testLowercaseIsUppercased() {
        XCTAssertEqual(InviteCode("abcd2345xy")?.value, "ABCD2345XY")
    }

    func testSurroundingWhitespaceIsTrimmed() {
        XCTAssertEqual(InviteCode("  ABCD2345XY\n")?.value, "ABCD2345XY")
    }

    func testOReadsAsZero() {
        XCTAssertEqual(InviteCode("O0o0O0o0O0")?.value, "0000000000")
    }

    func testIAndLReadAsOne() {
        XCTAssertEqual(InviteCode("IiLl1IiLl1")?.value, "1111111111")
    }

    func testTooShortIsRejected() {
        XCTAssertNil(InviteCode("ABCD2345X"))
    }

    func testTooLongIsRejected() {
        XCTAssertNil(InviteCode("ABCD2345XYZ"))
    }

    func testLetterOutsideTheAlphabetIsRejected() {
        XCTAssertNil(InviteCode("ABCD2345XU"))
    }

    func testInnerSpaceIsRejected() {
        XCTAssertNil(InviteCode("ABCD 2345X"))
    }

    func testCabalIDIsRejected() {
        XCTAssertNil(InviteCode("01890a5d-ac96-774b-bcce-b302099a8058"))
    }

    func testEmptyIsRejected() {
        XCTAssertNil(InviteCode(""))
    }

    func testTheShareTextCarriesTheCode() {
        XCTAssertEqual(CabalCopy.inviteShareText(code: "ABCD2345XY"), "Join my cabal on Monaco with code ABCD2345XY")
    }
}
