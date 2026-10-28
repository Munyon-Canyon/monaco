import MonacoCore
import XCTest

final class FundCabalCopyTests: XCTestCase {
    func testPercentsAreWholeNumbersOrOneDecimalUnderTenPercent() {
        XCTAssertEqual(FundCabalCopy.percent(bps: 3500), "35%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 3658), "37%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 850), "8.5%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 800), "8%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 5), "0.1%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 999), "10%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 1000), "10%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 0), "0%")
        XCTAssertEqual(FundCabalCopy.percent(bps: 10_000), "100%")
    }

    func testDollarsDropCentsOnlyWhenWhole() {
        XCTAssertEqual(FundCabalCopy.amount(micros: 250_000_000), "$250")
        XCTAssertEqual(FundCabalCopy.amount(micros: 1_250_500_000), "$1,250.50")
        XCTAssertEqual(FundCabalCopy.amount(micros: 1_000_000), "$1")
    }

    func testTheShareLineShowsTheArrowOnlyWithAnAfter() {
        XCTAssertEqual(FundCabalCopy.shareLine(now: 3500), "From your cash · your share 35%")
        XCTAssertEqual(FundCabalCopy.shareLine(now: 3500, after: 3600), "From your cash · your share 35% → 36%")
        XCTAssertEqual(FundCabalCopy.shareLine(now: 850, after: 1210), "From your cash · your share 8.5% → 12%")
    }

    func testTheLimitSlotAndMinimumLines() {
        XCTAssertEqual(
            FundCabalCopy.limitLine(availableMicros: 500_000_000, overMicros: 150_000_000),
            "$500.00 available · $150.00 over")
        XCTAssertEqual(FundCabalCopy.slotHint(availableMicros: 500_000_000), "Up to $500.00")
        XCTAssertEqual(FundCabalCopy.slotHint(availableMicros: 1_250_500_000), "Up to $1,250.50")
        XCTAssertEqual(FundCabalCopy.belowMinimum(minMicros: 1_000_000), "Minimum $1")
        XCTAssertEqual(FundCabalCopy.enterAnAmount, "Enter an amount")
    }

    func testPausedButtonAndFundingLines() {
        XCTAssertEqual(
            FundCabalCopy.paused(cabalName: "AC Capital"), "AC Capital is paused. Funding opens when it resumes.")
        XCTAssertEqual(FundCabalCopy.button(micros: 250_000_000), "Fund $250")
        XCTAssertEqual(FundCabalCopy.button(micros: 1_250_500_000), "Fund $1,250.50")
        XCTAssertEqual(FundCabalCopy.funding, "Funding…")
    }

    func testTheSettledLineNamesTheShareOnlyWhenTheTransferReportedOne() {
        XCTAssertEqual(FundCabalCopy.settledLine(now: 3600), "Added just now · your share is now 36%")
        XCTAssertEqual(FundCabalCopy.settledLine(now: nil), "Added just now")
        XCTAssertEqual(FundCabalCopy.check(micros: 250_000_000, cabalName: "AC Capital"), "$250 to AC Capital")
    }
}
