import XCTest

@testable import MonacoCore

/// What the chip under the hero price says in each session the calendar can produce.
///
/// The product argument lives in this copy — the stock stops, the token does not —
/// so it is asserted rather than left to whoever edits the view next.
final class MarketSessionCopyTests: XCTestCase {
    private let easternUS = Locale(identifier: "en_US@hours=h12")
    private let newYork = TimeZone(identifier: "America/New_York")!

    private func chip(_ market: MarketStatus?, now: Date? = nil) -> MarketSessionChipCopy? {
        MarketSessionCopy.chip(for: market, locale: easternUS, timeZone: newYork, now: now)
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

    func testAHalfDayNamesTheEarlyBell() throws {
        let copy = try XCTUnwrap(chip(Sessions.sessionEarlyClose))
        let detail = try XCTUnwrap(copy.detail)

        XCTAssertEqual(copy.title, "Market open")
        XCTAssertTrue(detail.hasPrefix("Closes early at "), detail)
        XCTAssertTrue(detail.contains("1:00"), detail)
    }

    func testAHalfDayWithNoBellTimeStillSaysItClosesEarly() throws {
        let market = MarketStatus(
            session: .open, isOpen: true, afterHours: false, nextSession: .afterHours, earlyClose: true)

        XCTAssertEqual(try XCTUnwrap(chip(market)).detail, "Closes early today")
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

    func testThePreMarketChipTurnsToOpenAtTheBellWithoutAFreshPayload() throws {
        let bell = try XCTUnwrap(Sessions.sessionPreMarket.nextTransition)

        let before = try XCTUnwrap(chip(Sessions.sessionPreMarket, now: bell.addingTimeInterval(-1)))
        let at = try XCTUnwrap(chip(Sessions.sessionPreMarket, now: bell))
        let after = try XCTUnwrap(chip(Sessions.sessionPreMarket, now: bell.addingTimeInterval(600)))

        XCTAssertEqual(before.title, "Pre-market")
        XCTAssertTrue(try XCTUnwrap(before.detail).hasPrefix("Opens "))
        XCTAssertEqual(at.title, "Market open")
        XCTAssertTrue(at.isLive)
        XCTAssertEqual(after.title, "Market open")
        XCTAssertNil(after.detail)
    }

    func testTheOpenChipTurnsToAfterHoursAtTheBell() throws {
        let bell = try XCTUnwrap(Sessions.sessionOpen.nextTransition)

        let copy = try XCTUnwrap(chip(Sessions.sessionOpen, now: bell))

        XCTAssertEqual(copy.title, "After hours")
        XCTAssertEqual(copy.detail, "Trading 24/7 on Solana")
        XCTAssertFalse(copy.isLive)
    }

    func testAnEarlyCloseOpenChipRollsToAfterHoursAndDropsTheEarlyCloseLine() throws {
        let bell = try XCTUnwrap(Sessions.sessionEarlyClose.nextTransition)

        let copy = try XCTUnwrap(chip(Sessions.sessionEarlyClose, now: bell))

        XCTAssertEqual(copy.title, "After hours")
        XCTAssertEqual(copy.detail, "Trading 24/7 on Solana")
    }

    func testTheClosedChipTurnsToPreMarketAndForgetsTheHoliday() throws {
        let bell = try XCTUnwrap(Sessions.sessionHoliday.nextTransition)

        let copy = try XCTUnwrap(chip(Sessions.sessionHoliday, now: bell))

        XCTAssertEqual(copy.title, "Pre-market")
        XCTAssertEqual(copy.detail, "Trading 24/7 on Solana")
    }

    func testAChipReadBeforeItsBellIsUnchangedByTheClock() throws {
        let reported = chip(Sessions.sessionOpen)

        XCTAssertEqual(chip(Sessions.sessionOpen, now: Sessions.tradingTuesday), reported)
    }

    func testTheNextChangeIsTheServersTransitionWhileItIsStillAhead() throws {
        let bell = try XCTUnwrap(Sessions.sessionPreMarket.nextTransition)

        XCTAssertEqual(
            MarketSessionCopy.nextChange(for: Sessions.sessionPreMarket, after: bell.addingTimeInterval(-60)), bell)
        XCTAssertNil(MarketSessionCopy.nextChange(for: Sessions.sessionPreMarket, after: bell))
        XCTAssertNil(MarketSessionCopy.nextChange(for: nil, after: bell))
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
