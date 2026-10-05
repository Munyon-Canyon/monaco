import XCTest

@testable import MonacoCore

/// What the chip under the hero price says in each session the calendar can produce.
///
/// The product argument lives in this copy — the stock stops, the token does not —
/// so it is asserted rather than left to whoever edits the view next.
final class MarketSessionCopyTests: XCTestCase {
    private let easternUS = Locale(identifier: "en_US@hours=h12")
    private let newYork = TimeZone(identifier: "America/New_York")!

    private func chip(_ market: MarketStatus?) -> MarketSessionChipCopy? {
        MarketSessionCopy.chip(for: market, locale: easternUS, timeZone: newYork)
    }

    func testOpenSaysNothingAboutSolanaBecauseThereIsNothingToExplain() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionOpen))

        XCTAssertEqual(copy.title, "Market open")
        XCTAssertTrue(copy.isLive)
        XCTAssertFalse(copy.spoken.contains("Solana"))
    }

    func testOpenNamesTheBell() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionOpen))
        let detail = try XCTUnwrap(copy.detail)

        // The sample's next transition is six hours after 10:00 ET. The space before
        // the meridiem is ICU's, narrow and non-breaking, so the assertion is on the
        // parts rather than on a literal nobody can see.
        XCTAssertTrue(detail.hasPrefix("Closes "), detail)
        XCTAssertTrue(detail.contains("4:00"), detail)
        XCTAssertTrue(detail.uppercased().contains("PM"), detail)
    }

    func testAHalfDaySaysSoInsteadOfNamingAnHour() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionEarlyClose))

        XCTAssertEqual(copy.title, "Market open")
        XCTAssertEqual(copy.detail, "Closes early today")
    }

    func testAfterHoursIsTheStateTheWholeChipExistsFor() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionAfterHours))

        XCTAssertEqual(copy.title, "After hours")
        XCTAssertEqual(copy.detail, "Trading 24/7 on Solana")
        XCTAssertFalse(copy.isLive)
    }

    func testPreMarketCountsDownToTheOpenWhenTheServerSaidWhen() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionPreMarket))
        let detail = try XCTUnwrap(copy.detail)

        XCTAssertEqual(copy.title, "Pre-market")
        XCTAssertTrue(detail.hasPrefix("Opens "), detail)
        XCTAssertTrue(detail.contains("9:30"), detail)
        XCTAssertFalse(copy.isLive)
    }

    /// A transition already behind the instant the payload was priced at is a stale
    /// payload. "Opens half an hour ago" is worse than saying the token is trading.
    func testATransitionInThePastIsNotACountdown() throws {
        let market = MarketStatus(
            session: .preMarket,
            isOpen: false,
            afterHours: true,
            nextSession: .open,
            nextTransition: Date(timeIntervalSince1970: 1_790_078_400),
            asOf: Date(timeIntervalSince1970: 1_790_085_600)
        )
        let copy = try XCTUnwrap(chip(market))

        XCTAssertEqual(copy.detail, "Trading 24/7 on Solana")
    }

    func testAHolidayIsNamed() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionHoliday))

        XCTAssertEqual(copy.title, "Closed for Thanksgiving Day")
        XCTAssertEqual(copy.detail, "Trading 24/7 on Solana")
    }

    func testAnOrdinaryOvernightCloseIsJustClosed() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionClosedOvernight))

        XCTAssertEqual(copy.title, "Market closed")
        XCTAssertFalse(copy.isLive)
    }

    /// An older backend sends no market block at all, and a session name added later
    /// decodes to `.unknown`. Both draw no chip rather than a guess.
    func testNoChipWithoutASession() {
        XCTAssertNil(chip(nil))
        XCTAssertNil(chip(MarketStatus(session: .unknown, isOpen: false, afterHours: true)))
    }

    func testSpokenFormJoinsBothHalves() {
        let copy = MarketSessionChipCopy(title: "After hours", detail: "Trading 24/7 on Solana", isLive: false)

        XCTAssertEqual(copy.spoken, "After hours, Trading 24/7 on Solana")
        XCTAssertEqual(MarketSessionChipCopy(title: "Market open", detail: nil, isLive: true).spoken, "Market open")
    }

    func testA24HourLocaleGetsA24HourBell() throws {
        let copy = try XCTUnwrap(
            MarketSessionCopy.chip(
                for: Sessions.sessionOpen,
                locale: Locale(identifier: "en_GB"),
                timeZone: TimeZone(identifier: "Europe/London")!
            )
        )
        let detail = try XCTUnwrap(copy.detail)

        XCTAssertTrue(detail.contains("21:00"), detail)
        XCTAssertFalse(detail.uppercased().contains("PM"), detail)
    }
}

private enum Sessions {
    static let tradingTuesday = Date(timeIntervalSince1970: 1_790_085_600)

    static let sessionOpen = MarketStatus(
        session: .open,
        isOpen: true,
        afterHours: false,
        nextSession: .afterHours,
        nextTransition: tradingTuesday.addingTimeInterval(6 * 3600),
        asOf: tradingTuesday
    )

    static let sessionPreMarket = MarketStatus(
        session: .preMarket,
        isOpen: false,
        afterHours: true,
        nextSession: .open,
        nextTransition: tradingTuesday.addingTimeInterval(-30 * 60),
        asOf: tradingTuesday.addingTimeInterval(-2 * 3600)
    )

    static let sessionAfterHours = MarketStatus(
        session: .afterHours,
        isOpen: false,
        afterHours: true,
        nextSession: .closed,
        nextTransition: tradingTuesday.addingTimeInterval(10 * 3600),
        asOf: tradingTuesday.addingTimeInterval(7 * 3600)
    )

    static let sessionClosedOvernight = MarketStatus(
        session: .closed,
        isOpen: false,
        afterHours: true,
        nextSession: .preMarket,
        nextTransition: tradingTuesday.addingTimeInterval(22 * 3600),
        asOf: tradingTuesday.addingTimeInterval(12 * 3600)
    )

    static let sessionHoliday = MarketStatus(
        session: .closed,
        isOpen: false,
        afterHours: true,
        nextSession: .preMarket,
        nextTransition: Date(timeIntervalSince1970: 1_795_770_000),
        asOf: Date(timeIntervalSince1970: 1_795_708_800),
        holiday: "Thanksgiving Day"
    )

    static let sessionEarlyClose = MarketStatus(
        session: .open,
        isOpen: true,
        afterHours: false,
        nextSession: .afterHours,
        nextTransition: Date(timeIntervalSince1970: 1_795_802_400),
        asOf: Date(timeIntervalSince1970: 1_795_780_800),
        earlyClose: true
    )
}
