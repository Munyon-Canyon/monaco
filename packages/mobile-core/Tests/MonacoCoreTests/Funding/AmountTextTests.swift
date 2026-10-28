import Foundation
import XCTest

@testable import MonacoCore

final class AmountTextTests: XCTestCase {
    func testKeysApplyToTheText() {
        let cases: [(AmountKey, String, String)] = [
            (.digit(5), "", "5"),
            (.digit(2), "5", "52"),
            (.dot, "", "0."),
            (.dot, "12", "12."),
            (.dot, "1.2", "1.2"),
            (.digit(9), "1.25", "1.25"),
            (.digit(5), "1.2", "1.25"),
            (.delete, "52", "5"),
            (.delete, "5", ""),
            (.delete, "", ""),
            (.delete, "0.", "0"),
            (.digit(5), "0", "5"),
            (.digit(0), "0", "0"),
            (.digit(1), "123456789012", "123456789012"),
        ]
        for (key, text, expected) in cases {
            XCTAssertEqual(AmountText.applying(key, to: text), expected, "\(key) on \"\(text)\"")
        }
    }

    func testSanitizeKeepsDigitsOnePointAndTwoDecimals() {
        let cases = [
            ("50", "50"), ("050", "50"), ("0", "0"), (".", "0."), ("12.345", "12.34"),
            ("1.2.3", "1.23"), ("12,5", "12.5"), ("$1a2", "12"), ("0.05", "0.05"),
        ]
        for (raw, expected) in cases {
            XCTAssertEqual(AmountText.sanitize(raw), expected, raw)
        }
    }

    func testPastedGroupingKeepsItsValue() {
        let cases = [
            ("1,250.50", "1250.50"), ("$1,250.50", "1250.50"), ("1,250", "1250"),
            ("1,250,000", "1250000"), ("1.250,50", "1250.50"),
        ]
        for (raw, expected) in cases {
            XCTAssertEqual(AmountText.sanitize(raw), expected, raw)
        }
    }

    func testATypedCommaIsADecimalPoint() {
        let cases = [("12,", "12."), ("12,5", "12.5"), ("12,50", "12.50")]
        for (raw, expected) in cases {
            XCTAssertEqual(AmountText.sanitize(raw), expected, raw)
        }
    }

    func testStrayCommasReadLikeStrayDots() {
        let cases = [("1,2,5", "1.25"), ("1,250,5", "1.25"), ("12,,5", "12.5"), ("1234,567", "1234.56")]
        for (raw, expected) in cases {
            XCTAssertEqual(AmountText.sanitize(raw), expected, raw)
        }
    }

    func testDisplayGroupsTheIntegerPartAndKeepsTypedDecimals() {
        XCTAssertEqual(AmountText.display(""), "$0")
        XCTAssertEqual(AmountText.display("1250"), "$1,250")
        XCTAssertEqual(AmountText.display("1250."), "$1,250.")
        XCTAssertEqual(AmountText.display("1250.5"), "$1,250.5")
    }

    func testPlainAndRounding() throws {
        XCTAssertEqual(AmountText.plain(Decimal(25)), "25")
        XCTAssertEqual(AmountText.plain(try XCTUnwrap(Decimal(string: "12.50"))), "12.5")
        XCTAssertEqual(
            AmountText.roundDownToCents(try XCTUnwrap(Decimal(string: "548.209"))),
            try XCTUnwrap(Decimal(string: "548.2")))
    }

    func testMicrosRoundsToTheNearestMicro() {
        XCTAssertEqual(AmountText.micros("12.34"), 12_340_000)
        XCTAssertEqual(AmountText.micros("0.10"), 100_000)
        XCTAssertEqual(AmountText.micros("1250.50"), 1_250_500_000)
        XCTAssertNil(AmountText.micros(""))
        XCTAssertNil(AmountText.micros("abc"))
    }

    func testDollarsReadsMicros() throws {
        XCTAssertEqual(AmountText.dollars(micros: 12_340_000), try XCTUnwrap(Decimal(string: "12.34")))
    }

    func testPresetRowsStackOnlyAtAccessibilitySizes() {
        XCTAssertEqual(AmountText.presetRows(4, stacked: false), [[0, 1, 2, 3]])
        XCTAssertEqual(AmountText.presetRows(4, stacked: true), [[0, 1], [2, 3]])
        XCTAssertEqual(AmountText.presetRows(0, stacked: true), [])
    }

    func testAddingAddsEachChipToTheAmount() {
        let cases: [(String, [String])] = [
            ("", ["1", "10", "20"]),
            ("0", ["1", "10", "20"]),
            ("12.5", ["13.5", "22.5", "32.5"]),
            ("999.99", ["1000.99", "1009.99", "1019.99"]),
        ]
        for (text, expected) in cases {
            let added = [Decimal(1), Decimal(10), Decimal(20)].map { AmountText.adding($0, to: text) }
            XCTAssertEqual(added, expected, "adding to \"\(text)\"")
        }
    }

    func testAddingKeepsTwoDecimals() {
        XCTAssertEqual(AmountText.adding(1, to: "0.05"), "1.05")
        XCTAssertEqual(AmountText.adding(10, to: "12.34"), "22.34")
        XCTAssertEqual(AmountText.micros(AmountText.adding(20, to: "999.99")), 1_019_990_000)
    }
}
