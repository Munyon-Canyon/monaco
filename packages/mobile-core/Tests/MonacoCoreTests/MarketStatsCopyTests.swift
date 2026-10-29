import MonacoAPI
import XCTest

@testable import MonacoCore

final class MarketStatsCopyTests: XCTestCase {
    private let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        return calendar
    }()
    private let now = Date(timeIntervalSince1970: 1_792_662_780)

    private func facts(
        networks: [Components.Schemas.Network] = [.solana, .base, .robinhood], continuous: Bool = true,
        source: PriceSource? = .jupiter, asOf: Date? = Date(timeIntervalSince1970: 1_792_662_780 - 720),
        seconds: Int = 120
    ) -> MarketStatsFacts {
        MarketStatsFacts(
            listingNetworks: networks, tradesContinuously: continuous, priceSource: source, priceAsOf: asOf,
            pollInterval: .seconds(seconds))
    }

    func testTheMessageNamesOneTwoAndThreeChains() {
        XCTAssertEqual(
            MarketStatsCopy.message(facts(networks: [.solana])),
            "Prices are for the tokenized stock on Solana, not the exchange listing. It trades 24/7.")
        XCTAssertEqual(
            MarketStatsCopy.message(facts(networks: [.solana, .base])),
            "Prices are for the tokenized stock on Solana and Base, not the exchange listing. It trades 24/7.")
        XCTAssertEqual(
            MarketStatsCopy.message(facts()),
            "Prices are for the tokenized stock on Solana, Base and Robinhood Chain, not the exchange listing. It trades 24/7."
        )
    }

    func testASessionAssetSaysItsExchangeKeepsMarketHours() {
        XCTAssertEqual(
            MarketStatsCopy.message(facts(networks: [.solana], continuous: false)),
            "Prices are for the tokenized stock on Solana, not the exchange listing. It trades 24/7; the stock's own exchange keeps market hours."
        )
    }

    func testTheSourceNamesTheRealProvider() {
        let rows = MarketStatsCopy.rows(facts(), now: now, calendar: utc)
        XCTAssertEqual(rows[0], .init(label: "Source", value: "Jupiter"))
        XCTAssertEqual(MarketStatsCopy.rows(facts(source: .coingecko), now: now, calendar: utc)[0].value, "CoinGecko")
        XCTAssertEqual(MarketStatsCopy.rows(facts(source: nil), now: now, calendar: utc)[0].value, "—")
    }

    func testUpdatedReadsTheTimeTodayAndTheDateOtherDays() {
        let rows = MarketStatsCopy.rows(facts(), now: now, calendar: utc)
        XCTAssertEqual(rows[1], .init(label: "Updated", value: "9:41 AM"))
        let earlier = facts(asOf: now.addingTimeInterval(-86_400 * 3))
        XCTAssertEqual(MarketStatsCopy.rows(earlier, now: now, calendar: utc)[1].value, "Oct 19")
        XCTAssertEqual(MarketStatsCopy.rows(facts(asOf: nil), now: now, calendar: utc)[1].value, "Not priced yet")
    }

    func testCheckedReadsTheCadence() {
        func checked(_ seconds: Int) -> String {
            MarketStatsCopy.rows(facts(seconds: seconds), now: now, calendar: utc)[2].value
        }
        XCTAssertEqual(MarketStatsCopy.rows(facts(), now: now, calendar: utc)[2].label, "Checked")
        XCTAssertEqual(checked(120), "Every 2 min")
        XCTAssertEqual(checked(30), "Every 30 s")
        XCTAssertEqual(checked(60), "Every min")
    }

    func testNoStringSaysUpTo() {
        let sheet = MarketStatsCopy.sheet(facts(), now: now, calendar: utc)
        let text = ([sheet.message] + sheet.rows.map(\.value) + MarketStatsCopy.auditedStrings).joined(separator: " ")
        XCTAssertFalse(text.lowercased().contains("up to"))
    }

    func testTheEstimateSheetReusesTheLedgerRows() throws {
        let preIpo = try XCTUnwrap(PreIpoDetail(.spaceX))
        let sheet = MarketStatsCopy.estimateSheet(preIpo)
        XCTAssertEqual(sheet.title, "Secondary estimate")
        XCTAssertEqual(sheet.message, MarketStatsCopy.estimateMessage)
        XCTAssertEqual(sheet.rows.count, 2)
        XCTAssertEqual(sheet.rows.map(\.label), try XCTUnwrap(preIpo.profile).ledger.map(\.label))
        XCTAssertTrue(sheet.rows[0].label.hasPrefix("Last round"))
        XCTAssertTrue(sheet.rows[1].label.hasPrefix("Secondary estimate"))
    }

    func testTheFactsComeFromTheAssetDetail() {
        let facts = MarketStatsFacts(.nvda)
        XCTAssertEqual(facts.priceSource, .jupiter)
        XCTAssertEqual(facts.pollInterval, .seconds(120))
        XCTAssertEqual(facts.priceAsOf, Date(timeIntervalSince1970: 1_772_596_200))
        XCTAssertFalse(MarketStatsFacts(.spaceX).tradesContinuously == false)
    }
}
