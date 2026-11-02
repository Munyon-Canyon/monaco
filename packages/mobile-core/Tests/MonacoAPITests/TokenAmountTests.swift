import XCTest

@testable import MonacoAPI

final class TokenAmountTests: XCTestCase {
    func testPastInt64ParsesExactly() {
        XCTAssertEqual(TokenAmount.parse("25000000000000000000"), Decimal(string: "25000000000000000000"))
        XCTAssertEqual(TokenAmount.text(TokenAmount.parse("25000000000000000000") ?? 0), "25000000000000000000")
        XCTAssertEqual(TokenAmount.text(TokenAmount.parse("9223372036854775808") ?? 0), "9223372036854775808")
    }

    func testStringFieldWinsOverTheIntegerField() {
        XCTAssertEqual(
            TokenAmount.decimal("25000000000000000000", fallback: nil), Decimal(string: "25000000000000000000"))
        XCTAssertEqual(TokenAmount.decimal("150000000", fallback: 7), 150_000_000)
    }

    func testAbsentStringFallsBackToTheIntegerField() {
        XCTAssertEqual(TokenAmount.decimal(nil, fallback: 150_000_000), 150_000_000)
        XCTAssertNil(TokenAmount.decimal(nil, fallback: nil))
    }

    func testMalformedStringFallsBackToTheIntegerField() {
        XCTAssertEqual(TokenAmount.decimal("1.5", fallback: 3), 3)
        XCTAssertEqual(TokenAmount.decimal("-4", fallback: 3), 3)
        XCTAssertEqual(TokenAmount.decimal("", fallback: 3), 3)
        XCTAssertNil(TokenAmount.decimal("abc", fallback: nil))
    }

    func testInt64OnlyWhenTheAmountFits() {
        XCTAssertEqual(TokenAmount.int64(9_223_372_036_854_775_807), Int64.max)
        XCTAssertNil(TokenAmount.int64(TokenAmount.parse("9223372036854775808") ?? 0))
        XCTAssertEqual(TokenAmount.int64(0), 0)
    }
}
